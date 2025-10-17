package docker

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

type DockerClient struct {
	Host   string
	Logger logger.Logger
}

func NewClient() *DockerClient {
	cfg := config.GetServerConfig()

	return &DockerClient{
		Host:   cfg.Docker.Host,
		Logger: log.WithGroup("docker"),
	}
}
