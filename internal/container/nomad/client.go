package nomad

import (
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/spf13/viper"
)

type NomadClient struct {
	httpClient *rest.RESTClient
}

func NewClient() (*NomadClient, error) {
	baseURL := viper.GetString("server.nomad.addr")
	token := viper.GetString("server.nomad.token")

	hc, err := rest.NewClient(baseURL, token, false)
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
