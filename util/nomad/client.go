package nomad

import (
	"github.com/paularlott/knot/util/rest"
)

type NomadClient struct {
  httpClient *rest.RESTClient
}

func NewClient(baseURL string, token string) *NomadClient {
  client := &NomadClient{
    httpClient: rest.NewClient(baseURL, token),
  }

  client.httpClient.SetTokenKey("X-Nomad-Token")
  client.httpClient.SetTokenValue("%s")

  return client
}
