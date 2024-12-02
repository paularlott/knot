package nomad

import (
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/viper"
)

type NomadClient struct {
	httpClient *rest.RESTClient
}

func NewClient() *NomadClient {
	baseURL := viper.GetString("server.nomad.addr")
	token := viper.GetString("server.nomad.token")

	client := &NomadClient{
		httpClient: rest.NewClient(baseURL, token, false),
	}

	client.httpClient.SetTokenKey("X-Nomad-Token")
	client.httpClient.SetTokenValue("%s")

	return client
}
