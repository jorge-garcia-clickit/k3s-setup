// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/haproxytech/config-parser/v3/params"
	"github.com/haproxytech/config-parser/v3/types"
	"github.com/haproxytech/models/v2"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/haproxytech/kubernetes-ingress/controller/annotations"
	"github.com/haproxytech/kubernetes-ingress/controller/haproxy"
	"github.com/haproxytech/kubernetes-ingress/controller/haproxy/api"
	ingressRoute "github.com/haproxytech/kubernetes-ingress/controller/ingress"
	"github.com/haproxytech/kubernetes-ingress/controller/store"
	"github.com/haproxytech/kubernetes-ingress/controller/utils"
)

//nolint:golint,stylecheck
const (
	CONTROLLER_CLASS = "haproxy.org/ingress-controller"
	// Configmaps
	Main        = "main"
	TCPServices = "tcpservices"
	Errorfiles  = "errorfiles"
	// sections
	FrontendHTTP      = "http"
	FrontendHTTPS     = "https"
	FrontendSSL       = "ssl"
	SSLDefaultBaceknd = "ssl"
	// Status
	ADDED    = store.ADDED
	DELETED  = store.DELETED
	ERROR    = store.ERROR
	EMPTY    = store.EMPTY
	MODIFIED = store.MODIFIED
	// MapFiles
	SNI         = "sni"
	HOST        = "host"
	PATH_EXACT  = "path-exact"
	PATH_PREFIX = "path-prefix"
)

var (
	HAProxyBinary        string
	HAProxyCFG           string
	HAProxyCfgDir        string
	HAProxyCertDir       string
	HAProxyFtCertDir     string
	HAProxyBdCertDir     string
	HAProxyCaCertDir     string
	HAProxyStateDir      string
	HAProxyMapDir        string
	HAProxyErrFileDir    string
	HAProxyRuntimeSocket string
	HAProxyPIDFile       string
	TransactionDir       string
)

var logger = utils.GetLogger()

// HAProxyController is ingress controller
type HAProxyController struct {
	k8s            *K8s
	Store          store.K8s
	PublishService *store.Service
	IngressClass   string
	ready          bool
	cfg            Configuration
	osArgs         utils.OSArgs
	Client         api.HAProxyClient
	eventChan      chan SyncDataEvent
	serverlessPods map[string]int
	UpdateHandlers []UpdateHandler
}

// Wrapping a Native-Client transaction and commit it.
// Returning an error to let panic or log it upon the scenario.
func (c *HAProxyController) clientAPIClosure(fn func() error) (err error) {
	if err = c.Client.APIStartTransaction(); err != nil {
		return err
	}
	defer func() {
		c.Client.APIDisposeTransaction()
	}()
	if err = fn(); err != nil {
		return err
	}
	if err = c.Client.APICommitTransaction(); err != nil {
		return err
	}
	return nil
}

// Start initializes and runs HAProxyController
func (c *HAProxyController) Start(osArgs utils.OSArgs) {
	c.osArgs = osArgs
	logger.SetLevel(osArgs.LogLevel.LogLevel)
	c.haproxyInitialize()
	c.initHandlers()

	logger.Panic(c.clientAPIClosure(func() error {
		logger.Error(c.handleBinds())
		if osArgs.PprofEnabled {
			logger.Error(c.handlePprof())
		}
		return nil
	}))

	parts := strings.Split(osArgs.PublishService, "/")
	if len(parts) == 2 {
		c.PublishService = &store.Service{
			Namespace: parts[0],
			Name:      parts[1],
			Status:    EMPTY,
			Addresses: []string{},
		}
	}

	var k8s *K8s
	var err error

	if osArgs.External {
		kubeconfig := filepath.Join(utils.HomeDir(), ".kube", "config")
		if osArgs.KubeConfig != "" {
			kubeconfig = osArgs.KubeConfig
		}
		k8s, err = GetRemoteKubernetesClient(kubeconfig)
	} else {
		k8s, err = GetKubernetesClient()
	}
	if err != nil {
		logger.Panic(err)
	}
	c.k8s = k8s

	x := k8s.API.Discovery()
	if k8sVersion, err := x.ServerVersion(); err != nil {
		logger.Panicf("Unable to get Kubernetes version: %v\n", err)
	} else {
		logger.Printf("Running on Kubernetes version: %s %s", k8sVersion.String(), k8sVersion.Platform)
	}

	c.serverlessPods = map[string]int{}
	c.eventChan = make(chan SyncDataEvent, watch.DefaultChanSize*6)
	go c.monitorChanges()
}

// Stop handles shutting down HAProxyController
func (c *HAProxyController) Stop() {
	logger.Infof("Stopping Ingress Controller")
	logger.Error(c.haproxyService("stop"))
}

// updateHAProxy syncs HAProxy configuration
func (c *HAProxyController) updateHAProxy() {
	logger.Trace("HAProxy config sync started")
	reload := false

	err := c.Client.APIStartTransaction()
	if err != nil {
		logger.Error(err)
		return
	}
	defer func() {
		c.Client.APIDisposeTransaction()
	}()

	restart, reload := annotations.HandleGlobalAnnotations(
		c.Store,
		c.Client,
		false,
		c.Store.ConfigMaps[Main].Annotations,
	)
	reload = c.handleDefaultCert() || reload
	c.handleDefaultService()

	for _, namespace := range c.Store.Namespaces {
		if !namespace.Relevant {
			continue
		}
		for _, ingress := range namespace.Ingresses {
			if ingress.Status == DELETED {
				continue
			}
			if !c.igClassIsSupported(ingress) {
				logger.Debugf("ingress '%s/%s' ignored: no matching IngressClass", ingress.Namespace, ingress.Name)
				continue
			}
			if c.PublishService != nil {
				logger.Error(c.k8s.UpdateIngressStatus(ingress, c.PublishService))
			}
			// Default Backend
			if ingress.DefaultBackend != nil {
				logger.Error(c.cfg.IngressRoutes.AddRoute(&ingressRoute.Route{
					Namespace: namespace,
					Ingress:   ingress,
					Path:      ingress.DefaultBackend,
				}))
			}
			// Ingress secrets
			logger.Tracef("ingress '%s/%s': processing secrets...", ingress.Namespace, ingress.Name)
			for _, tls := range ingress.TLS {
				if tls.Status == store.DELETED {
					continue
				}
				crt, updated, _ := c.cfg.Certificates.HandleTLSSecret(c.Store, haproxy.SecretCtx{
					DefaultNS:  ingress.Namespace,
					SecretPath: tls.SecretName.Value,
					SecretType: haproxy.FT_CERT,
				})
				if crt != "" && updated {
					reload = true
					logger.Debugf("Secret '%s' in ingress '%s/%s' was updated, reload required", tls.SecretName.Value, ingress.Namespace, ingress.Name)
				}
			}
			// Ingress annotations
			logger.Tracef("ingress '%s/%s': processing annotations...", ingress.Namespace, ingress.Name)
			if len(ingress.Rules) == 0 {
				logger.Debugf("Ingress %s/%s: no rules defined", ingress.Namespace, ingress.Name)
				continue
			}
			c.handleIngressAnnotations(ingress)
			// Ingress rules
			logger.Tracef("ingress '%s/%s': processing rules...", ingress.Namespace, ingress.Name)
			for _, rule := range ingress.Rules {
				for _, path := range rule.Paths {
					logger.Error(c.cfg.IngressRoutes.AddRoute(&ingressRoute.Route{
						Namespace:      namespace,
						Ingress:        ingress,
						Host:           rule.Host,
						Path:           path,
						HAProxyRules:   c.cfg.HAProxyRules.GetIngressRuleIDs(ingress.Namespace + "-" + ingress.Name),
						SSLPassthrough: c.sslPassthroughEnabled(ingress, path),
					}))
				}
			}
		}
	}

	for _, handler := range c.UpdateHandlers {
		r, errHandler := handler.Update(c.Store, &c.cfg, c.Client)
		logger.Error(errHandler)
		reload = reload || r
	}

	err = c.Client.APICommitTransaction()
	if err != nil {
		logger.Error("unable to Sync HAProxy configuration !!")
		logger.Error(err)
		c.clean(true)
		return
	}
	c.clean(false)
	if !c.ready {
		c.setToReady()
	}
	switch {
	case restart:
		if err = c.haproxyService("restart"); err != nil {
			logger.Error(err)
		} else {
			logger.Info("HAProxy restarted")
		}
	case reload:
		if err = c.haproxyService("reload"); err != nil {
			logger.Error(err)
		} else {
			logger.Info("HAProxy reloaded")
		}
	}

	logger.Trace("HAProxy config sync ended")
}

// setToRready exposes readiness endpoint
func (c *HAProxyController) setToReady() {
	logger.Panic(c.clientAPIClosure(func() error {
		return c.Client.FrontendBindEdit("healthz",
			models.Bind{
				Name:    "v4",
				Address: "0.0.0.0:1042",
			})
	}))

	if !c.osArgs.DisableIPV6 {
		logger.Panic(c.clientAPIClosure(func() error {
			return c.Client.FrontendBindCreate("healthz",
				models.Bind{
					Name:    "v6",
					Address: ":::1042",
					V4v6:    true,
				})
		}))
	}

	logger.Debugf("healthz frontend exposed for readiness probe")
	c.ready = true
}

// haproxyInitialize initializes HAProxy environment and its API client.
func (c *HAProxyController) haproxyInitialize() {
	var err error
	// HAProxy executable
	HAProxyBinary = "/usr/local/sbin/haproxy"
	if c.osArgs.Program != "" {
		HAProxyBinary = c.osArgs.Program
	}
	_, err = os.Stat(HAProxyBinary)
	if err != nil && !c.osArgs.Test {
		logger.Panic(err)
	}
	// Initialize files and directories
	if HAProxyCFG == "" {
		HAProxyCFG = filepath.Join(HAProxyCfgDir, "haproxy.cfg")
	}
	if _, err = os.Stat(HAProxyCFG); err != nil {
		logger.Panic(err)
	}
	if HAProxyPIDFile == "" {
		HAProxyPIDFile = "/var/run/haproxy.pid"
	}
	if HAProxyRuntimeSocket == "" {
		HAProxyRuntimeSocket = "/var/run/haproxy-runtime-api.sock"
	}
	if HAProxyCertDir == "" {
		HAProxyCertDir = filepath.Join(HAProxyCfgDir, "certs")
	}

	HAProxyFtCertDir = filepath.Join(HAProxyCertDir, "frontend")
	HAProxyBdCertDir = filepath.Join(HAProxyCertDir, "backend")
	HAProxyCaCertDir = filepath.Join(HAProxyCertDir, "ca")

	if HAProxyMapDir == "" {
		HAProxyMapDir = filepath.Join(HAProxyCfgDir, "maps")
	}
	if HAProxyErrFileDir == "" {
		HAProxyErrFileDir = filepath.Join(HAProxyCfgDir, "errors")
	}
	if HAProxyStateDir == "" {
		HAProxyStateDir = "/var/state/haproxy/"
	}
	if TransactionDir != "" {
		err = os.MkdirAll(TransactionDir, 0755)
		if err != nil {
			logger.Panic(err)
		}
	}
	for _, d := range []string{HAProxyCertDir, HAProxyFtCertDir, HAProxyBdCertDir, HAProxyCaCertDir, HAProxyMapDir, HAProxyErrFileDir, HAProxyStateDir} {
		err = os.MkdirAll(d, 0755)
		if err != nil {
			logger.Panic(err)
		}
	}
	_, err = os.Create(filepath.Join(HAProxyStateDir, "global"))
	logger.Err(err)

	// Initialize HAProxy client API
	c.Client, err = api.Init(TransactionDir, HAProxyCFG, HAProxyBinary, HAProxyRuntimeSocket)
	if err != nil {
		logger.Panic(err)
	}
	if c.osArgs.External && !c.osArgs.Test {
		logger.Panic(c.clientAPIClosure(func() error {
			var errors utils.Errors
			errors.Add(
				// Configure runtime socket
				c.Client.RuntimeSocket(nil),
				c.Client.RuntimeSocket(&types.Socket{
					Path: HAProxyRuntimeSocket,
					Params: []params.BindOption{
						&params.BindOptionDoubleWord{Name: "expose-fd", Value: "listeners"},
						&params.BindOptionValue{Name: "level", Value: "admin"},
					},
				}),
				// Configure pidfile
				c.Client.PIDFile(&types.StringC{Value: HAProxyPIDFile}),
				// Configure server-state-base
				c.Client.ServerStateBase(&types.StringC{Value: HAProxyStateDir}),
			)
			return errors.Result()
		}))
	}

	cmd := exec.Command(HAProxyBinary, "-v")
	haproxyInfo, err := cmd.Output()
	if err == nil {
		haproxyInfo := strings.Split(string(haproxyInfo), "\n")
		logger.Printf("Running with %s", haproxyInfo[0])
	} else {
		logger.Error(err)
	}

	logger.Printf("Starting HAProxy with %s", HAProxyCFG)
	logger.Panic(c.haproxyService("start"))

	hostname, err := os.Hostname()
	logger.Error(err)
	logger.Printf("Running on %s", hostname)

	c.cfg.Init()
}

// handleBind configures Frontends bind lines
func (c *HAProxyController) handleBinds() (err error) {
	var errors utils.Errors
	frontends := make(map[string]int64, 2)
	protos := make(map[string]string, 2)
	if !c.osArgs.DisableHTTP {
		frontends[FrontendHTTP] = c.osArgs.HTTPBindPort
	}
	if !c.osArgs.DisableHTTPS {
		frontends[FrontendHTTPS] = c.osArgs.HTTPSBindPort
	}
	if !c.osArgs.DisableIPV4 {
		protos["v4"] = c.osArgs.IPV4BindAddr
	}
	if !c.osArgs.DisableIPV6 {
		protos["v6"] = c.osArgs.IPV6BindAddr

		// IPv6 not disabled, so add v6 listening to stats frontend
		errors.Add(c.Client.FrontendBindCreate("stats",
			models.Bind{
				Name:    "v6",
				Address: ":::1024",
				V4v6:    false,
			}))
	}
	for ftName, ftPort := range frontends {
		for proto, addr := range protos {
			bind := models.Bind{
				Name:    proto,
				Address: addr,
				Port:    utils.PtrInt64(ftPort),
			}
			if err = c.Client.FrontendBindEdit(ftName, bind); err != nil {
				errors.Add(c.Client.FrontendBindCreate(ftName, bind))
			}
		}
	}
	return errors.Result()
}

// handlePprof enables  pprof backend
func (c *HAProxyController) handlePprof() (err error) {
	pprofBackend := "pprof"

	err = c.Client.BackendCreate(models.Backend{
		Name: pprofBackend,
		Mode: "http",
	})
	if err != nil {
		return err
	}
	err = c.Client.BackendServerCreate(pprofBackend, models.Server{
		Name:    "pprof",
		Address: "127.0.0.1:6060",
	})
	if err != nil {
		return err
	}
	logger.Debug("pprof backend created")
	logger.Error(c.cfg.IngressRoutes.AddRoute(&ingressRoute.Route{
		Path: &store.IngressPath{
			Path:           "/debug/pprof",
			ExactPathMatch: false,
		},
		BackendName:  pprofBackend,
		LocalBackend: true,
	}))
	return nil
}

// handleDefaultService configures HAProy default backend provided via cli param "default-backend-service"
func (c *HAProxyController) handleDefaultService() {
	dsvcData, _ := c.Store.GetValueFromAnnotations("default-backend-service")
	if dsvcData == nil {
		return
	}
	dsvc := strings.Split(dsvcData.Value, "/")

	if len(dsvc) != 2 {
		logger.Errorf("default service invalid data")
		return
	}
	if dsvc[0] == "" || dsvc[1] == "" {
		return
	}
	namespace, ok := c.Store.Namespaces[dsvc[0]]
	if !ok {
		logger.Errorf("default service invalid namespace " + dsvc[0])
		return
	}
	service, ok := namespace.Services[dsvc[1]]
	if !ok {
		logger.Errorf("service '" + dsvc[1] + "' does not exist")
		return
	}
	ingress := &store.Ingress{
		Namespace:   namespace.Name,
		Name:        "DefaultService",
		Annotations: store.MapStringW{},
		Rules:       map[string]*store.IngressRule{},
	}
	path := &store.IngressPath{
		SvcName:          service.Name,
		SvcPortInt:       service.Ports[0].Port,
		IsDefaultBackend: true,
	}
	logger.Error(c.cfg.IngressRoutes.AddRoute(&ingressRoute.Route{
		Namespace: namespace,
		Ingress:   ingress,
		Path:      path,
	}))
}

// handleDefaultCert configures default/fallback HAProxy certificate to use for client HTTPS requests.
func (c *HAProxyController) handleDefaultCert() (reload bool) {
	secretAnn, _ := c.Store.GetValueFromAnnotations("ssl-certificate", c.Store.ConfigMaps[Main].Annotations)
	if secretAnn == nil {
		return false
	}
	crt, updated, _ := c.cfg.Certificates.HandleTLSSecret(c.Store, haproxy.SecretCtx{
		SecretPath: secretAnn.Value,
		SecretType: haproxy.FT_CERT,
	})
	return crt != "" && updated
}

// clean controller state
func (c *HAProxyController) clean(failedSync bool) {
	c.cfg.Clean()
	if c.PublishService != nil {
		c.PublishService.Status = EMPTY
	}
	c.cfg.SSLPassthrough = false
	if failedSync {
		return
	}
	c.Store.Clean()
}

func (c *HAProxyController) sslPassthroughEnabled(ingress *store.Ingress, path *store.IngressPath) bool {
	var annSSLPassthrough *store.StringW
	var service *store.Service
	ok := false
	if path != nil {
		service, ok = c.Store.Namespaces[ingress.Namespace].Services[path.SvcName]
	}
	if ok {
		annSSLPassthrough, _ = c.Store.GetValueFromAnnotations("ssl-passthrough", service.Annotations, ingress.Annotations, c.Store.ConfigMaps[Main].Annotations)
	} else {
		annSSLPassthrough, _ = c.Store.GetValueFromAnnotations("ssl-passthrough", ingress.Annotations, c.Store.ConfigMaps[Main].Annotations)
	}
	enabled, err := utils.GetBoolValue(annSSLPassthrough.Value, "ssl-passthrough")
	if err != nil {
		logger.Errorf("ssl-passthrough annotation: %s", err)
		return false
	}
	if annSSLPassthrough.Status == DELETED {
		return false
	}
	if enabled {
		c.cfg.SSLPassthrough = true
		return true
	}
	return false
}

// igClassIsSupported verifies if the IngressClass matches the ControllerClass
// and in such case returns true otherwise false
//
// According to https://github.com/kubernetes/api/blob/master/networking/v1/types.go#L257
// ingress.class annotation should have precedence over the IngressClass mechanism implemented
// in "networking.k8s.io".
func (c *HAProxyController) igClassIsSupported(ingress *store.Ingress) bool {
	var igClassAnn string
	var igClass *store.IngressClass
	if ann, _ := c.Store.GetValueFromAnnotations("ingress.class", ingress.Annotations); ann != nil {
		igClassAnn = ann.Value
	}
	if igClassAnn == "" || igClassAnn != c.IngressClass {
		igClass = c.Store.IngressClasses[ingress.Class]
		if igClass != nil && igClass.Status != DELETED && igClass.Controller == CONTROLLER_CLASS {
			// Corresponding IngresClass was updated so Ingress resource should be re-processed
			// This is particularly important if the Ingress was skipped due to mismatching ingrssClass
			if igClass.Status != EMPTY {
				ingress.Status = MODIFIED
			}
			return true
		}
	}
	if igClassAnn == c.IngressClass {
		return true
	}
	return false
}
