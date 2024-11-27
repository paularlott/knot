package rest

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database/model"

	"github.com/rs/zerolog/log"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	ContentTypeJSON    = "application/json"
	ContentTypeMsgPack = "application/msgpack"
)

type RESTClient struct {
	baseURL          string
	token            string
	tokenKey         string
	tokenValue       string
	userAgent        string
	useSessionCookie bool
	contentType      string
	HTTPClient       *http.Client
}

func NewClient(baseURL string, token string, insecureSkipVerify bool) *RESTClient {
	restClient := &RESTClient{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		token:            token,
		tokenKey:         "Authorization",
		tokenValue:       "Bearer %s",
		userAgent:        "knot v" + build.Version,
		useSessionCookie: false,
		contentType:      ContentTypeJSON,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	restClient.HTTPClient.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: insecureSkipVerify},
		MaxConnsPerHost:     32 * 2,
		MaxIdleConns:        32 * 2,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
	}

	return restClient
}

func (c *RESTClient) SetContentType(contentType string) *RESTClient {
	c.contentType = contentType
	return c
}

func (c *RESTClient) SetUserAgent(userAgent string) *RESTClient {
	c.userAgent = userAgent
	return c
}

func (c *RESTClient) SetBaseUrl(baseURL string) *RESTClient {
	c.baseURL = strings.TrimSuffix(baseURL, "/")
	return c
}

func (c *RESTClient) UseSessionCookie(useCookie bool) *RESTClient {
	c.useSessionCookie = useCookie
	return c
}

func (c *RESTClient) AppendUserAgent(userAgent string) *RESTClient {
	c.userAgent = c.userAgent + " " + userAgent
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

func (c *RESTClient) SetTokenValue(value string) *RESTClient {
	c.tokenValue = value
	return c
}

func (c *RESTClient) setHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, application/msgpack")
	req.Header.Set("Content-Type", c.contentType)
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {

		// If set the session cookie if using session cookies; else set the token header
		if c.useSessionCookie {
			cookie := &http.Cookie{
				Name:     model.WEBUI_SESSION_COOKIE,
				Value:    c.token,
				Path:     "/",
				HttpOnly: true,
				Secure:   false,
				SameSite: http.SameSiteLaxMode,
			}

			req.AddCookie(cookie)
		} else {
			req.Header.Set(c.tokenKey, fmt.Sprintf(c.tokenValue, c.token))
		}
	}
}

func (c *RESTClient) Get(path string, response interface{}) (int, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return 0, err
	}

	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") == ContentTypeMsgPack {
		err = msgpack.NewDecoder(resp.Body).Decode(response)
	} else {
		err = json.NewDecoder(resp.Body).Decode(response)
	}
	return resp.StatusCode, err
}

func (c *RESTClient) SendData(method string, path string, request interface{}, response interface{}, successCode int) (int, error) {
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

	req, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return 0, err
	}

	c.setHeaders(req)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != successCode {

		// Get the body as a string and wrap in the error message
		var bodyBytes []byte
		if resp.Body != nil {
			bodyBytes, _ = io.ReadAll(resp.Body)
		}
		bodyString := string(bodyBytes)

		log.Debug().Msgf("rest: %s, status: %d, error: %s", path, resp.StatusCode, bodyString)
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d, %w", resp.StatusCode, fmt.Errorf(bodyString))
	}

	if response == nil {
		return resp.StatusCode, nil
	} else {
		if resp.Header.Get("Content-Type") == ContentTypeMsgPack {
			err = msgpack.NewDecoder(resp.Body).Decode(response)
		} else {
			err = json.NewDecoder(resp.Body).Decode(response)
		}
		return resp.StatusCode, err
	}
}

func (c *RESTClient) Post(path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(http.MethodPost, path, request, response, successCode)
}

func (c *RESTClient) Put(path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(http.MethodPut, path, request, response, successCode)
}

func (c *RESTClient) Delete(path string, request interface{}, response interface{}, successCode int) (int, error) {
	return c.SendData(http.MethodDelete, path, request, response, successCode)
}
