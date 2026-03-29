package apiclient

import (
	"context"
	"fmt"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

type ApiClient struct {
	httpClient rest.RESTClient
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) (*ApiClient, error) {
	client, err := rest.NewClient(baseURL, token, insecureSkipVerify)
	if err != nil {
		return nil, err
	}

	client.SetContentType(rest.ContentTypeMsgPack)

	return &ApiClient{
		httpClient: client,
	}, nil
}

// NewMuxClient creates an ApiClient that uses MuxClient for direct API calls
func NewMuxClient(user *model.User) *ApiClient {
	client := rest.NewMuxClient(user)
	client.SetContentType(rest.ContentTypeMsgPack)

	return &ApiClient{
		httpClient: client,
	}
}

func (c *ApiClient) SetContentType(contentType string) *ApiClient {
	c.httpClient.SetContentType(contentType)
	return c
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

func (c *ApiClient) GetBaseURL() string {
	return c.httpClient.GetBaseURL()
}

func (c *ApiClient) GetAuthToken() string {
	return c.httpClient.GetAuthToken()
}

func (c *ApiClient) GetRESTClient() rest.RESTClient {
	return c.httpClient
}

// Do makes an arbitrary API request
func (c *ApiClient) Do(ctx context.Context, method string, path string, requestBody interface{}, responseBody interface{}) (int, error) {
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

// DoJSON makes an arbitrary API request using JSON content type and JSON accept header (thread-safe)
func (c *ApiClient) DoJSON(ctx context.Context, method string, path string, requestBody interface{}, responseBody interface{}) (int, error) {
	switch method {
	case "GET":
		return c.httpClient.GetJSON(ctx, path, responseBody)
	case "POST":
		return c.httpClient.PostJSON(ctx, path, requestBody, responseBody, 200)
	case "PUT":
		return c.httpClient.PutJSON(ctx, path, requestBody, responseBody, 200)
	case "DELETE":
		return c.httpClient.Delete(ctx, path, nil, nil, 200)
	default:
		return 0, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}

func (c *ApiClient) GetWebSocketURL() string {
	baseURL := c.httpClient.GetBaseURL()
	if baseURL == "" {
		return ""
	}
	if len(baseURL) > 8 && baseURL[:8] == "https://" {
		return "wss://" + baseURL[8:]
	}
	if len(baseURL) > 7 && baseURL[:7] == "http://" {
		return "ws://" + baseURL[7:]
	}
	return "ws://" + baseURL
}

func (c *ApiClient) ConnectWebSocket(ctx context.Context, url string) (*websocket.Conn, error) {
	token := c.httpClient.GetAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no auth token available")
	}

	header := make(map[string][]string)
	header["Authorization"] = []string{"Bearer " + token}

	dialer := websocket.Dialer{}
	ws, _, err := dialer.DialContext(ctx, url, header)
	return ws, err
}
