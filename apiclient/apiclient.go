package apiclient

import (
	"context"
	"fmt"
	"time"

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

func (c *ApiClient) SetTimeout(timeout time.Duration) *ApiClient {
	c.httpClient.SetTimeout(timeout)
	return c
}

// Do makes an arbitrary API request using JSON content type
func (c *ApiClient) Do(ctx context.Context, method string, path string, requestBody interface{}, responseBody interface{}) (int, error) {
	// Set content type to JSON for this request
	c.httpClient.SetContentType("application/json")
	defer c.httpClient.SetContentType(rest.ContentTypeMsgPack) // Reset to default

	switch method {
	case "GET":
		return c.httpClient.Get(ctx, path, responseBody)
	case "POST":
		return c.httpClient.Post(ctx, path, requestBody, responseBody, 200)
	case "PUT":
		return c.httpClient.Put(ctx, path, requestBody, responseBody, 200)
	case "DELETE":
		return c.httpClient.Delete(ctx, path, nil, nil, 200)
	default:
		return 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}
