package nomad

import (
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"
)

type NomadClient struct {
	httpClient *rest.RESTClient
}

func NewClient() (*NomadClient, error) {
	cfg := config.GetServerConfig()
	hc, err := rest.NewClient(cfg.Nomad.Host, cfg.Nomad.Token, false)
	if err != nil {
		return nil, err
	}

	client := &NomadClient{
		httpClient: hc,
	}

	client.httpClient.SetTokenKey("X-Nomad-Token")
	client.httpClient.SetTokenValue("%s")

	return client, nil
}
