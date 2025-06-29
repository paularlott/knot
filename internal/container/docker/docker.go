package docker

import "github.com/paularlott/knot/internal/config"

type DockerClient struct {
	Host       string
	DriverName string
}

func NewClient() *DockerClient {
	cfg := config.GetServerConfig()

	return &DockerClient{
		Host:       cfg.Docker.Host,
		DriverName: "docker",
	}
}
