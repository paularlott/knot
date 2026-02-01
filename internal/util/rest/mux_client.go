package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	req := httptest.NewRequest(http.MethodGet, path, nil)
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

	req := httptest.NewRequest(method, path, bytes.NewReader(data))
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
