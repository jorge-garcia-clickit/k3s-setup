package annotations

import (
	"strconv"

	"github.com/haproxytech/config-parser/v3/types"

	"github.com/haproxytech/kubernetes-ingress/controller/haproxy/api"
	"github.com/haproxytech/kubernetes-ingress/controller/store"
)

type GlobalMaxconn struct {
	name   string
	data   *types.Int64C
	client api.HAProxyClient
}

func NewGlobalMaxconn(n string, c api.HAProxyClient) *GlobalMaxconn {
	return &GlobalMaxconn{name: n, client: c}
}

func (a *GlobalMaxconn) GetName() string {
	return a.name
}

func (a *GlobalMaxconn) Parse(input store.StringW, forceParse bool) error {
	if input.Status == store.EMPTY && !forceParse {
		return ErrEmptyStatus
	}
	if input.Status == store.DELETED {
		return nil
	}
	v, err := strconv.Atoi(input.Value)
	if err != nil {
		return err
	}
	a.data = &types.Int64C{Value: int64(v)}
	return nil
}

func (a *GlobalMaxconn) Update() error {
	if a.data == nil {
		logger.Infof("Removing default maxconn")
		return a.client.GlobalMaxconn(nil)
	}
	logger.Infof("Setting default maxconn to %d", a.data.Value)
	return a.client.GlobalMaxconn(a.data)
}
