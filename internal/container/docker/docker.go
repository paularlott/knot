package docker

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/logger"
)

type DockerClient struct {
	httpClient *rest.HTTPClient
	Logger     logger.Logger
}

// SetHTTPClient allows embedding types (e.g. PodmanClient) to override the HTTP client.
func (c *DockerClient) SetHTTPClient(hc *rest.HTTPClient) {
	c.httpClient = hc
}

func NewClient() *DockerClient {
	cfg := config.GetServerConfig()
	hc, err := rest.NewUnixSocketClient(cfg.Docker.Host)
	if err != nil {
		log.WithGroup("docker").Error("failed to create docker client", "error", err)
		return nil
	}

	return &DockerClient{
		httpClient: hc,
		Logger:     log.WithGroup("docker"),
	}
}
