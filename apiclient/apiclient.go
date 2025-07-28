package apiclient

import (
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

type ApiClient struct {
	httpClient *rest.RESTClient
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) (*ApiClient, error) {

	client, err := rest.NewClient(baseURL, token, insecureSkipVerify)
	if err != nil {
		return nil, err
	}

	c := &ApiClient{
		httpClient: client,
	}

	c.httpClient.SetContentType(rest.ContentTypeMsgPack)

	return c, nil
}

func (c *ApiClient) AppendUserAgent(userAgent string) *ApiClient {
	c.httpClient.AppendUserAgent(userAgent)
	return c
}

func (c *ApiClient) UseSessionCookie(useCookie bool) *ApiClient {
	c.httpClient.SetTokenKey(model.WebSessionCookie).SetTokenFormat("%s")
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
