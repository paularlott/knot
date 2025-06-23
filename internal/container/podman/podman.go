package podman

import (
	"github.com/paularlott/knot/internal/container/docker"

	"github.com/spf13/viper"
)

type PodmanClient struct {
	docker.DockerClient
}

func NewClient() *PodmanClient {
	c := &PodmanClient{}
	c.Host = viper.GetString("server.podman.host")
	c.DriverName = "podman"
	return c
}
