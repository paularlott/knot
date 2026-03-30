package podman

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/docker"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/rest"
)

type PodmanClient struct {
	docker.DockerClient
}

func NewClient() *PodmanClient {
	cfg := config.GetServerConfig()

	hc, err := rest.NewUnixSocketClient(cfg.Podman.Host)
	if err != nil {
		log.WithGroup("podman").Error("failed to create podman client", "error", err)
		return nil
	}

	c := &PodmanClient{}
	c.SetHTTPClient(hc)
	c.Logger = log.WithGroup("podman")
	return c
}
