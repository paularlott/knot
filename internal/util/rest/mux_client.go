package rest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/toolsurface"
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

// SetResponseCheck is a no-op: a MuxClient calls its own server.
func (c *MuxClient) SetResponseCheck(check func(*http.Response) error) RESTClient {
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
	req = req.WithContext(muxContext(ctx, c.user))

	req.Header.Set("Accept", strings.Join(c.accept, ", "))
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	rec := newMuxRecorder(ctx)
	apiMux.ServeHTTP(rec, req)
	if err := rec.err(); err != nil {
		return rec.Code, err
	}

	if response != nil {
		err := decodeBody(rec.Header().Get("Content-Type"), rec.Body, response)
		return rec.Code, err
	}
	return rec.Code, nil
}

func (c *MuxClient) sendData(ctx context.Context, method string, path string, request interface{}, response interface{}, successCode int) (int, error) {
	var data []byte
	var err error

	data, err = encodeBody(c.contentType, request)
	if err != nil {
		return 0, err
	}

	req, err := newMuxRequest(method, path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req = req.WithContext(muxContext(ctx, c.user))

	req.Header.Set("Accept", strings.Join(c.accept, ", "))
	req.Header.Set("Content-Type", c.contentType)
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}

	rec := newMuxRecorder(ctx)
	apiMux.ServeHTTP(rec, req)
	if err := rec.err(); err != nil {
		return rec.Code, err
	}

	if (successCode == 0 && rec.Code >= http.StatusBadRequest) || (successCode > 0 && rec.Code != successCode) {
		bodyBytes, _ := io.ReadAll(rec.Body)
		return rec.Code, NewHTTPError(rec.Code, method, path, bodyBytes)
	}

	if response != nil {
		err = decodeBody(rec.Header().Get("Content-Type"), rec.Body, response)
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

// DoRaw sends a request with a raw body and extra headers through the mux
// and returns the response unread, so callers can move bytes rather than JSON.
// It matches HTTPClient.DoRaw, letting raw transfers work in either client.
func (c *MuxClient) DoRaw(ctx context.Context, method string, path string, body io.Reader, size int64, headers map[string]string) (*http.Response, error) {
	req, err := newMuxRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(muxContext(ctx, c.user))
	if body != nil {
		req.ContentLength = size
	}

	req.Header.Set("Accept", strings.Join(c.accept, ", "))
	req.Header.Set("User-Agent", c.userAgent)
	for key, value := range c.headers {
		req.Header.Set(key, value)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := newMuxRecorder(ctx)
	apiMux.ServeHTTP(rec, req)
	if err := rec.err(); err != nil {
		return nil, err
	}

	resp := rec.Result()
	resp.Request = req
	return resp, nil
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
	muxReq := req.Clone(muxContext(req.Context(), c.user))

	rec := newMuxRecorder(req.Context())
	apiMux.ServeHTTP(rec, muxReq)
	if err := rec.err(); err != nil {
		return nil, err
	}

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

// muxContext is the context of an in-process API request: the caller's,
// carrying the user to act as, without the caller's tool-call surface. An
// approval obtained for the calling request (toolsurface) must never reach
// a nested request, such as a chat completion or a tool call it makes.
func muxContext(ctx context.Context, user *model.User) context.Context {
	return context.WithValue(toolsurface.Strip(ctx), "user", user)
}

// ErrResponseTooLarge is returned by the in-process client when a response
// grows past the limit set with WithResponseLimit.
var ErrResponseTooLarge = errors.New("response too large")

type responseLimitKey struct{}

// WithResponseLimit caps, at limit bytes, the body of every in-process API
// response made under ctx: the client buffers a response whole, so a caller
// that must bound its memory (an AI-written script) sets one. A response
// over the limit is refused with ErrResponseTooLarge.
func WithResponseLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, responseLimitKey{}, limit)
}

// ResponseLimit returns the limit set on ctx with WithResponseLimit, 0 when
// there is none.
func ResponseLimit(ctx context.Context) int64 {
	if ctx == nil {
		return 0
	}
	limit, _ := ctx.Value(responseLimitKey{}).(int64)
	return limit
}

// muxRecorder is the in-process client's response recorder: an
// httptest.ResponseRecorder that refuses writes past the context's response
// limit, so a handler streaming a large body stops rather than grows the
// buffer.
type muxRecorder struct {
	*httptest.ResponseRecorder
	limit   int64
	written int64
	over    bool
}

func newMuxRecorder(ctx context.Context) *muxRecorder {
	return &muxRecorder{ResponseRecorder: httptest.NewRecorder(), limit: ResponseLimit(ctx)}
}

func (r *muxRecorder) Write(p []byte) (int, error) {
	if r.limit > 0 && r.written+int64(len(p)) > r.limit {
		r.over = true
		return 0, ErrResponseTooLarge
	}
	r.written += int64(len(p))
	return r.ResponseRecorder.Write(p)
}

func (r *muxRecorder) WriteString(s string) (int, error) {
	if r.limit > 0 && r.written+int64(len(s)) > r.limit {
		r.over = true
		return 0, ErrResponseTooLarge
	}
	r.written += int64(len(s))
	return r.ResponseRecorder.WriteString(s)
}

// err reports a response refused for its size.
func (r *muxRecorder) err() error {
	if r.over {
		return fmt.Errorf("%w: the response is over the %d byte limit", ErrResponseTooLarge, r.limit)
	}
	return nil
}
