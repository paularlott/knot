package nomad

import (
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/logger"
)

type NomadClient struct {
	httpClient rest.RESTClient
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

	// CSI controller operations (create/delete) run synchronously through to
	// the storage plugin and can take far longer than the REST client's 10s
	// default; abandoning them leaves the operation running in Nomad.
	client.httpClient.SetTimeout(2 * time.Minute)

	return client, nil
}
