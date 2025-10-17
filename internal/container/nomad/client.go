package nomad

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/logger"
)

type NomadClient struct {
	httpClient *rest.RESTClient
	logger     logger.Logger
}

func NewClient() (*NomadClient, error) {
	cfg := config.GetServerConfig()
	hc, err := rest.NewClient(cfg.Nomad.Host, cfg.Nomad.Token, false)
	if err != nil {
		return nil, err
	}

	client := &NomadClient{
		httpClient: hc,
		logger:     log.WithGroup("nomad"),
	}

	client.httpClient.SetTokenKey("X-Nomad-Token").SetTokenFormat("%s")

	return client, nil
}
