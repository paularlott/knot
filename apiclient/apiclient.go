package apiclient

import (
	"github.com/paularlott/knot/internal/util/rest"
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
