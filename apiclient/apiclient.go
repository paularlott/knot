package apiclient

import (
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/viper"
)

type ApiClient struct {
	httpClient *rest.RESTClient
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) *ApiClient {
	return &ApiClient{
		httpClient: rest.NewClient(baseURL, token, insecureSkipVerify),
	}
}

func NewRemoteSession(token string) *ApiClient {
	baseURL := viper.GetString("server.core_server")

	client := &ApiClient{
		httpClient: rest.NewClient(baseURL, token, viper.GetBool("tls_skip_verify")),
	}

	client.httpClient.AppendUserAgent("Remote (" + viper.GetString("server.location") + ")")
	client.httpClient.SetTokenKey("X-Knot-Remote-Session")
	client.httpClient.SetTokenValue("%s")

	return client
}

func (c *ApiClient) SetToken(token string) {
	c.httpClient.SetAuthToken(token)
}
