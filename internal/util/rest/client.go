package rest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/build"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	ContentTypeJSON    = "application/json"
	ContentTypeMsgPack = "application/msgpack"
)

type RESTClient struct {
	baseURL     *url.URL
	token       string
	tokenKey    string
	tokenFormat string
	userAgent   string
	contentType string
	HTTPClient  *http.Client
	headers     map[string]string
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) (*RESTClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %s, error: %v", baseURL, err)
	}

	restClient := &RESTClient{
		baseURL:     parsed,
		token:       token,
		tokenKey:    "Authorization",
		tokenFormat: "Bearer %s",
		userAgent:   "knot v" + build.Version,
		contentType: ContentTypeJSON,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		headers: make(map[string]string),
	}

	restClient.HTTPClient.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: insecureSkipVerify},
		MaxConnsPerHost:     32 * 2,
		MaxIdleConns:        32 * 2,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     30 * time.Second,
		//DisableCompression:  true,
	}

	return restClient, nil
}

func (c *RESTClient) Close() {
	c.HTTPClient.CloseIdleConnections()
}

func (c *RESTClient) SetTimeout(timeout time.Duration) *RESTClient {
	c.HTTPClient.Timeout = timeout
	return c
}

func (c *RESTClient) SetContentType(contentType string) *RESTClient {
	c.contentType = contentType
	return c
}

func (c *RESTClient) SetUserAgent(userAgent string) *RESTClient {
	c.userAgent = userAgent
	return c
}

func (c *RESTClient) SetBaseUrl(baseURL string) (*RESTClient, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %s, error: %v", baseURL, err)
	}

	c.baseURL = parsedURL

	return c, nil
}

func (c *RESTClient) AppendUserAgent(userAgent string) *RESTClient {
	c.userAgent = strings.TrimSpace(c.userAgent + " " + userAgent)
	return c
}

func (c *RESTClient) SetAuthToken(token string) *RESTClient {
	c.token = token
	return c
}

func (c *RESTClient) SetTokenKey(key string) *RESTClient {
	c.tokenKey = key
	return c
}

func (c *RESTClient) SetTokenFormat(format string) *RESTClient {
	c.tokenFormat = format
	return c
}

func (c *RESTClient) SetHeader(key, value string) *RESTClient {
	c.headers[key] = value
	return c
}

func (c *RESTClient) DeleteHeader(key string) *RESTClient {
	delete(c.headers, key)
	return c
}

func (c *RESTClient) ClearHeaders() *RESTClient {
	c.headers = make(map[string]string)
	return c
}

func (c *RESTClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, application/msgpack")
	req.Header.Set("Content-Type", c.contentType)
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set(c.tokenKey, fmt.Sprintf(c.tokenFormat, c.token))
	}

	// Add custom headers
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
}

func (c *RESTClient) Get(ctx context.Context, path string, response interface{}) (int, error) {
	rel, err := url.Parse(path)
	if err != nil {
		return 0, fmt.Errorf("invalid path: %s, error: %v", path, err)
	}

	u := c.baseURL.ResolveReference(rel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, err
	}

	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	if response != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, ContentTypeMsgPack) {
			err = msgpack.NewDecoder(resp.Body).Decode(response)
		} else {
			err = json.NewDecoder(resp.Body).Decode(response)
		}
	}
	return resp.StatusCode, err
}

func (c *RESTClient) SendData(ctx context.Context, method string, path string, request interface{}, response interface{}, successCode int) (int, error) {
	var data []byte
	var err error

	rel, err := url.Parse(path)
	if err != nil {
		return 0, fmt.Errorf("invalid path: %s, error: %v", path, err)
	}

	u := c.baseURL.ResolveReference(rel)

	if c.contentType == ContentTypeMsgPack {
		data, err = msgpack.Marshal(request)
	} else {
		data, err = json.Marshal(request)
	}
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if (successCode == 0 && resp.StatusCode >= http.StatusBadRequest) || (successCode > 0 && resp.StatusCode != successCode) {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if response != nil {
		contentType := resp.Header.Get("Content-Type")
		if strings.Contains(contentType, ContentTypeMsgPack) {
			err = msgpack.NewDecoder(resp.Body).Decode(response)
		} else {
			err = json.NewDecoder(resp.Body).Decode(response)
		}
		if err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *RESTClient) Post(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(ctx, http.MethodPost, path, request, response, successCode)
}

func (c *RESTClient) Put(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(ctx, http.MethodPut, path, request, response, successCode)
}

func (c *RESTClient) Delete(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(ctx, http.MethodDelete, path, request, response, successCode)
}

type StreamResponseFunc func(chunk interface{}) (isDone bool, err error)

// Core streaming logic - single implementation, no generics, no reflection
func (c *RESTClient) streamDataCore(
	ctx context.Context,
	method string,
	path string,
	request interface{},
	fn StreamResponseFunc,
	createChunk func() interface{}, // Factory function to create typed chunks
) error {
	// Marshal request based on content type
	var data []byte
	var err error

	if c.contentType == ContentTypeMsgPack {
		data, err = msgpack.Marshal(request)
	} else {
		data, err = json.Marshal(request)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	rel, err := url.Parse(path)
	if err != nil {
		return fmt.Errorf("invalid path: %s, error: %v", path, err)
	}
	u := c.baseURL.ResolveReference(rel)

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.setHeaders(req)

	// For streaming requests, we need to temporarily disable the client timeout
	// and rely on the context for cancellation instead
	originalTimeout := c.HTTPClient.Timeout
	c.HTTPClient.Timeout = 0 // Disable timeout for streaming

	// Make request
	resp, err := c.HTTPClient.Do(req)

	// Restore original timeout
	c.HTTPClient.Timeout = originalTimeout

	if err != nil {
		return fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// SSE streaming logic
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle different SSE line types
		var data string
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		} else if strings.HasPrefix(line, "data:") {
			// Handle case without space after colon
			data = strings.TrimPrefix(line, "data:")
		} else {
			// Skip non-data lines (event:, id:, retry:, etc.)
			continue
		}

		// Trim any remaining whitespace
		data = strings.TrimSpace(data)

		// Skip empty data
		if data == "" {
			continue
		}

		// Check for end marker
		if data == "[DONE]" {
			break
		}

		// Create new instance using factory function
		chunk := createChunk()

		// SSE always uses JSON
		if err := json.Unmarshal([]byte(data), chunk); err != nil {
			// Skip malformed chunks
			continue
		}

		// Call handler function
		isDone, err := fn(chunk)
		if err != nil {
			return err
		}

		// Check if caller indicates we're done
		if isDone {
			break
		}
	}

	return scanner.Err()
}

// Define a constraint for pointer types
type Pointer[T any] interface {
	*T
}

// Thin generic wrapper - only this small function is duplicated per type
func StreamData[P Pointer[T], T any](
	c *RESTClient,
	ctx context.Context,
	method string,
	path string,
	request interface{},
	fn func(P) (bool, error),
) error {
	return c.streamDataCore(ctx, method, path, request,
		func(chunk interface{}) (bool, error) {
			return fn(chunk.(P))
		},
		func() interface{} {
			// P is *T, so new(T) gives us *T directly
			return new(T)
		})
}
