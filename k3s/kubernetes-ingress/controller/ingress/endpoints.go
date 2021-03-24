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

package ingress

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/haproxytech/kubernetes-ingress/controller/annotations"
	"github.com/haproxytech/kubernetes-ingress/controller/store"
	"github.com/haproxytech/kubernetes-ingress/controller/utils"
	"github.com/haproxytech/models/v2"
)

// scaleHAproxySrvs adds servers to match available addresses
func (route *Route) scaleHAProxySrvs() {
	var reload bool
	var srvSlots int
	var disabled []*store.HAProxySrv
	haproxySrvs := route.endpoints.HAProxySrvs
	// "servers-increment", "server-slots" are legacy annotations
	for _, annotation := range []string{"servers-increment", "server-slots", "scale-server-slots"} {
		annServerSlots, _ := k8sStore.GetValueFromAnnotations(annotation, k8sStore.ConfigMaps[Main].Annotations)
		if annServerSlots != nil {
			if value, err := strconv.Atoi(annServerSlots.Value); err == nil {
				srvSlots = value
				break
			} else {
				logger.Error(err)
			}
		}
	}
	// Add disabled HAProxySrvs to match scale-server-slots
	reload = false
	for len(haproxySrvs) < srvSlots {
		srv := &store.HAProxySrv{
			Name:     fmt.Sprintf("SRV_%d", len(haproxySrvs)+1),
			Address:  "",
			Modified: true,
		}
		haproxySrvs = append(haproxySrvs, srv)
		disabled = append(disabled, srv)
		reload = true
	}
	if reload {
		route.reload = true
		logger.Debugf("Server slots in backend '%s' scaled to match scale-server-slots value: %d, reload required", route.BackendName, srvSlots)
	}
	// Configure remaining addresses in available HAProxySrvs
	reload = false
	for addr := range route.endpoints.AddrNew {
		if len(disabled) != 0 {
			disabled[0].Address = addr
			disabled[0].Modified = true
			disabled = disabled[1:]
		} else {
			srv := &store.HAProxySrv{
				Name:     fmt.Sprintf("SRV_%d", len(haproxySrvs)+1),
				Address:  addr,
				Modified: true,
			}
			haproxySrvs = append(haproxySrvs, srv)
			reload = true
		}
		delete(route.endpoints.AddrNew, addr)
	}
	if reload {
		route.reload = true
		logger.Debugf("Server slots in backend '%s' scaled to match available endpoints, reload required", route.BackendName)
	}
	route.endpoints.HAProxySrvs = haproxySrvs
}

// handleEndpoints lookups the IngressPath related endpoints and makes corresponding backend servers configuration in HAProxy
// If only the address changes , no need to reload just generate new config
func (route *Route) handleEndpoints() {
	err := route.getEndpoints()
	if err != nil {
		logger.Warning(err)
		return
	}
	route.endpoints.BackendName = route.BackendName
	if route.service.DNS == "" {
		route.scaleHAProxySrvs()
	}
	backendUpdated := annotations.HandleServerAnnotations(
		k8sStore,
		client,
		haproxyCerts,
		&models.Server{Namespace: route.Namespace.Name},
		false,
		route.service.Annotations,
		route.Ingress.Annotations,
		k8sStore.ConfigMaps[Main].Annotations,
	) || route.NewBackend
	route.reload = route.reload || backendUpdated
	for _, srv := range route.endpoints.HAProxySrvs {
		if srv.Modified || backendUpdated {
			route.handleHAProxSrv(srv)
		}
	}
}

// handleHAProxSrv creates/updates corresponding HAProxy backend server
func (route *Route) handleHAProxSrv(srv *store.HAProxySrv) {
	server := models.Server{
		Name:    srv.Name,
		Address: srv.Address,
		Port:    &route.endpoints.Port,
		Weight:  utils.PtrInt64(128),
	}
	// Disabled
	if server.Address == "" {
		server.Address = "127.0.0.1"
		server.Maintenance = "enabled"
	}
	// Server related annotations
	annotations.HandleServerAnnotations(
		k8sStore,
		client,
		haproxyCerts,
		&server,
		true,
		route.service.Annotations,
		route.Ingress.Annotations,
		k8sStore.ConfigMaps[Main].Annotations,
	)
	// Update server
	errAPI := client.BackendServerEdit(route.BackendName, server)
	if errAPI == nil {
		logger.Tracef("Updating server '%s/%s'", route.BackendName, server.Name)
		return
	}
	if strings.Contains(errAPI.Error(), "does not exist") {
		logger.Tracef("Creating server '%s/%s'", route.BackendName, server.Name)
		logger.Error(client.BackendServerCreate(route.BackendName, server))
	}
}

func (route *Route) handleExternalName() error {
	//TODO: currently HAProxy will only resolve server name at startup/reload
	// This needs to be improved by using HAProxy resolvers to have resolution at runtime
	logger.Tracef("Configuring service '%s', of type ExternalName", route.service.Name)
	var port int64
	for _, sp := range route.service.Ports {
		if sp.Name == route.Path.SvcPortString || sp.Port == route.Path.SvcPortInt {
			port = sp.Port
		}
	}
	if port == 0 {
		ingressPort := route.Path.SvcPortString
		if route.Path.SvcPortInt != 0 {
			ingressPort = fmt.Sprintf("%d", route.Path.SvcPortInt)
		}
		return fmt.Errorf("service '%s': service port '%s' not found", route.service.Name, ingressPort)
	}
	backend, err := client.BackendGet(route.BackendName)
	if err != nil {
		return err
	}
	backend.DefaultServer = &models.DefaultServer{InitAddr: "last,libc,none"}
	if err = client.BackendEdit(backend); err != nil {
		return err
	}
	route.endpoints = &store.PortEndpoints{
		Port: port,
		HAProxySrvs: []*store.HAProxySrv{
			{
				Name:     "SRV_1",
				Address:  route.service.DNS,
				Modified: true,
			},
		},
	}
	return nil
}

func (route *Route) getEndpoints() error {
	endpoints, ok := route.Namespace.Endpoints[route.service.Name]
	if !ok {
		if route.service.DNS != "" {
			return route.handleExternalName()
		}
		return fmt.Errorf("ingress %s/%s: No Endpoints for service '%s'", route.Namespace.Name, route.Ingress.Name, route.service.Name)
	}
	sp := route.Path.SvcPortResolved
	if sp != nil {
		for portName, endpoints := range endpoints.Ports {
			if portName == sp.Name || endpoints.Port == sp.Port {
				route.endpoints = endpoints
				return nil
			}
		}
	}
	if route.Path.SvcPortString != "" {
		return fmt.Errorf("ingress %s/%s: no matching endpoints for service '%s' and port '%s'", route.Namespace.Name, route.Ingress.Name, route.service.Name, route.Path.SvcPortString)
	}
	return fmt.Errorf("ingress %s/%s: no matching endpoints for service '%s' and port '%d'", route.Namespace.Name, route.Ingress.Name, route.service.Name, route.Path.SvcPortInt)
}
