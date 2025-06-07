package adapters

import (
	"hack-a-tone/internal/core/port"
)

type GrafanaServer struct {
	kubeCtl port.KubeController
}

func (s *GrafanaServer) Start() error {
	//TODO implement me
	panic("implement me")
}
