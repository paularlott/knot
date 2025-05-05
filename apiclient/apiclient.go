package apiclient

import (
	"github.com/paularlott/knot/util/rest"

	"github.com/spf13/viper"
)

// Enum for authorization methods
const (
	AuthToken = iota
	AuthSessionCookie
	AuthRemoteServerToken
)

type ApiClient struct {
	httpClient *rest.RESTClient
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) *ApiClient {
	c := &ApiClient{
		httpClient: rest.NewClient(baseURL, token, insecureSkipVerify),
	}

	c.httpClient.SetContentType(rest.ContentTypeMsgPack)

	return c
}

// TODO Remove this as server.origin_server isn't defined any more!
func NewRemoteToken(token string) *ApiClient {
	c := NewClient(viper.GetString("server.origin_server"), token, viper.GetBool("tls_skip_verify"))
	return c
}

func NewRemoteSession(token string) *ApiClient {
	c := NewRemoteToken(token)
	c.httpClient.UseSessionCookie(true)
	return c
}

func (c *ApiClient) AppendUserAgent(userAgent string) *ApiClient {
	c.httpClient.AppendUserAgent(userAgent)
	return c
}

func (c *ApiClient) UseSessionCookie(useCookie bool) *ApiClient {
	c.httpClient.UseSessionCookie(useCookie)
	return c
}

func (c *ApiClient) SetAuthToken(token string) *ApiClient {
	c.httpClient.SetAuthToken(token)
	return c
}

func (c *ApiClient) SetBaseUrl(baseURL string) *ApiClient {
	c.httpClient.SetBaseUrl(baseURL)
	return c
}
