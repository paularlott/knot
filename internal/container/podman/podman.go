package podman

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/docker"
)

type PodmanClient struct {
	docker.DockerClient
}

func NewClient() *PodmanClient {
	cfg := config.GetServerConfig()

	c := &PodmanClient{}
	c.Host = cfg.Podman.Host
	c.DriverName = "podman"
	return c
}
