package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/vmihailenco/msgpack/v5"
)

var apiMux *http.ServeMux

// SetAPIMux stores the API mux for direct calls
func SetAPIMux(mux *http.ServeMux) {
	apiMux = mux
}

// newMuxRequest is a panic-safe wrapper around httptest.NewRequest.
//
// httptest.NewRequest panics on malformed paths (e.g. paths containing raw
// spaces), because it builds an HTTP request-line string and runs it through
// http.ReadRequest. MuxClient is called from scriptling via the embedded
// apiclient, and any panic here surfaces as "panic in builtin: ..." to the
// caller — ugly and confusing. Validate first and return a clean error.
func newMuxRequest(method, path string, body io.Reader) (*http.Request, error) {
	if _, err := url.Parse(path); err != nil {
		return nil, fmt.Errorf("invalid request path %q: %v", path, err)
	}
	// url.Parse doesn't catch everything that httptest.NewRequest rejects
	// (e.g. raw spaces). Validate the path contains no ASCII whitespace.
	for _, r := range path {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return nil, fmt.Errorf("invalid request path %q: contains whitespace; URL-encode path segments before calling", path)
		}
	}
	return httptest.NewRequest(method, path, body), nil
}

// MuxClient calls API handlers directly via mux without HTTP overhead
type MuxClient struct {
	user        *model.User
	headers     map[string]string
	timeout     time.Duration
	contentType string
	userAgent   string
	accept      []string
}

// NewMuxClient creates a client that calls mux directly
func NewMuxClient(user *model.User) RESTClient {
	if apiMux == nil {
		panic("apiMux not set - call rest.SetAPIMux() first")
	}
	return &MuxClient{
		user:        user,
		headers:     make(map[string]string),
		timeout:     10 * time.Second,
		contentType: ContentTypeJSON,
		userAgent:   "knot-mux-client",
		accept:      []string{ContentTypeJSON, ContentTypeMsgPack},
	}
}

func (c *MuxClient) SetTimeout(timeout time.Duration) RESTClient {
	c.timeout = timeout
	return c
}

func (c *MuxClient) SetContentType(contentType string) RESTClient {
	c.contentType = contentType
	return c
}

func (c *MuxClient) SetAccept(accept ...string) RESTClient {
	c.accept = accept
	return c
}

func (c *MuxClient) SetUserAgent(userAgent string) RESTClient {
	c.userAgent = userAgent
	return c
}

func (c *MuxClient) AppendUserAgent(userAgent string) RESTClient {
	c.userAgent = strings.TrimSpace(c.userAgent + " " + userAgent)
	return c
}

func (c *MuxClient) SetHeader(key, value string) RESTClient {
	c.headers[key] = value
	return c
}

func (c *MuxClient) DeleteHeader(key string) RESTClient {
	delete(c.headers, key)
	return c
}

func (c *MuxClient) ClearHeaders() RESTClient {
	c.headers = make(map[string]string)
	return c
}

func (c *MuxClient) Get(ctx context.Context, path string, response interface{}) (int, error) {
	req, err := newMuxRequest(http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	ctx = context.WithValue(ctx, "user", c.user)
	req = req.WithContext(ctx)

	req.Header.Set("Accept", strings.Join(c.accept, ", "))
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	apiMux.ServeHTTP(rec, req)

	if response != nil {
		contentType := rec.Header().Get("Content-Type")
		if strings.Contains(contentType, ContentTypeMsgPack) {
			err := msgpack.NewDecoder(rec.Body).Decode(response)
			return rec.Code, err
		} else {
			err := json.NewDecoder(rec.Body).Decode(response)
			return rec.Code, err
		}
	}
	return rec.Code, nil
}

func (c *MuxClient) sendData(ctx context.Context, method string, path string, request interface{}, response interface{}, successCode int) (int, error) {
	var data []byte
	var err error

	if c.contentType == ContentTypeMsgPack {
		data, err = msgpack.Marshal(request)
	} else {
		data, err = json.Marshal(request)
	}
	if err != nil {
		return 0, err
	}

	req, err := newMuxRequest(method, path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	ctx = context.WithValue(ctx, "user", c.user)
	req = req.WithContext(ctx)

	req.Header.Set("Accept", strings.Join(c.accept, ", "))
	req.Header.Set("Content-Type", c.contentType)
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	apiMux.ServeHTTP(rec, req)

	if (successCode == 0 && rec.Code >= http.StatusBadRequest) || (successCode > 0 && rec.Code != successCode) {
		bodyBytes, _ := io.ReadAll(rec.Body)
		return rec.Code, fmt.Errorf("unexpected status code: %d: %s", rec.Code, string(bodyBytes))
	}

	if response != nil {
		contentType := rec.Header().Get("Content-Type")
		if strings.Contains(contentType, ContentTypeMsgPack) {
			err = msgpack.NewDecoder(rec.Body).Decode(response)
		} else {
			err = json.NewDecoder(rec.Body).Decode(response)
		}
		if err != nil {
			return rec.Code, err
		}
	}
	return rec.Code, nil
}

func (c *MuxClient) Post(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.sendData(ctx, http.MethodPost, path, request, response, successCode)
}

func (c *MuxClient) PostJSON(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	oldContentType := c.contentType
	oldAccept := c.accept
	c.contentType = ContentTypeJSON
	c.accept = []string{ContentTypeJSON}
	defer func() {
		c.contentType = oldContentType
		c.accept = oldAccept
	}()
	return c.sendData(ctx, http.MethodPost, path, request, response, successCode)
}

func (c *MuxClient) Put(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.sendData(ctx, http.MethodPut, path, request, response, successCode)
}

func (c *MuxClient) PutJSON(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	oldContentType := c.contentType
	oldAccept := c.accept
	c.contentType = ContentTypeJSON
	c.accept = []string{ContentTypeJSON}
	defer func() {
		c.contentType = oldContentType
		c.accept = oldAccept
	}()
	return c.sendData(ctx, http.MethodPut, path, request, response, successCode)
}

func (c *MuxClient) Delete(ctx context.Context, path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.sendData(ctx, http.MethodDelete, path, request, response, successCode)
}

func (c *MuxClient) GetJSON(ctx context.Context, path string, response interface{}) (int, error) {
	oldAccept := c.accept
	c.accept = []string{ContentTypeJSON}
	defer func() {
		c.accept = oldAccept
	}()
	return c.Get(ctx, path, response)
}

func (c *MuxClient) SetAuthToken(token string) RESTClient {
	return c
}

func (c *MuxClient) SetBaseUrl(baseURL string) (RESTClient, error) {
	return c, nil
}

func (c *MuxClient) GetBaseURL() string {
	return ""
}

func (c *MuxClient) GetAuthToken() string {
	return ""
}

func (c *MuxClient) SetTokenKey(key string) RESTClient {
	return c
}

func (c *MuxClient) SetTokenFormat(format string) RESTClient {
	return c
}

// RoundTrip implements http.RoundTripper by routing the request through the API
// mux with the user injected into context for authentication.
func (c *MuxClient) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := context.WithValue(req.Context(), "user", c.user)
	muxReq := req.Clone(ctx)

	rec := httptest.NewRecorder()
	apiMux.ServeHTTP(rec, muxReq)

	return rec.Result(), nil
}

// NewMuxHTTPClient creates an *http.Client that routes requests through the API
// mux with the given user injected into context for authentication. This allows
// standard HTTP clients (like mcpopenai.Client) to make in-process API calls
// without needing a valid auth token.
func NewMuxHTTPClient(user *model.User) *http.Client {
	if apiMux == nil {
		panic("apiMux not set - call rest.SetAPIMux() first")
	}
	return &http.Client{
		Transport: &MuxClient{user: user},
	}
}
