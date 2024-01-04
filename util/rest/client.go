package rest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type RESTClient struct {
  baseURL    string
  token      string
  tokenKey   string
  tokenValue string
  HTTPClient *http.Client
}

func NewClient(baseURL string, token string) *RESTClient {
  return &RESTClient{
    baseURL: strings.TrimSuffix(baseURL, "/"),
    token: token,
    tokenKey: "Authorization",
    tokenValue: "Bearer %s",
    HTTPClient: &http.Client{
      Timeout: 10 * time.Second,
    },
  }
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

func (c *RESTClient) Get(path string, response interface{}) (int, error) {
  req, err := http.NewRequest(http.MethodGet, c.baseURL + path, nil)
  if err != nil {
    return 0, err
  }

  req.Header.Set("Accept", "application/json")
  req.Header.Set("Content-Type", "application/json")
  if c.token != "" {
    req.Header.Set(c.tokenKey, fmt.Sprintf(c.tokenValue, c.token))
  }

  resp, err := c.HTTPClient.Do(req);
  if err != nil {
    return 0, err
  }

  if resp.StatusCode != http.StatusOK {
    return resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
  }

  defer resp.Body.Close()

  err = json.NewDecoder(resp.Body).Decode(response)
  return resp.StatusCode, err
}

func (c *RESTClient) SendData(method string, path string, request interface{}, response interface{}) (int, error) {
  jsonData, err := json.Marshal(request)
  if err != nil {
    return 0, err
  }

  req, err := http.NewRequest(method, c.baseURL + path, bytes.NewReader(jsonData))
  if err != nil {
    return 0, err
  }

  req.Header.Set("Accept", "application/json")
  req.Header.Set("Content-Type", "application/json")
  if c.token != "" {
    req.Header.Set(c.tokenKey, fmt.Sprintf(c.tokenValue, c.token))
  }

  resp, err := c.HTTPClient.Do(req);
  if err != nil {
    return 0, err
  }

  if resp.StatusCode != http.StatusOK {
    return resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
  }

  defer resp.Body.Close()

  err = json.NewDecoder(resp.Body).Decode(response)
  return resp.StatusCode, err
}

func (c *RESTClient) Post(path string, request interface{}, response interface{}) (int, error) {
  return c.SendData(http.MethodPost, path, request, response)
}

func (c *RESTClient) Put(path string, request interface{}, response interface{}) (int, error) {
  return c.SendData(http.MethodPut, path, request, response)
}

func (c *RESTClient) Delete(path string, request interface{}, response interface{}) (int, error) {
  return c.SendData(http.MethodDelete, path, request, response)
}
