package util

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/paularlott/knot/build"

	"github.com/spf13/viper"
)

func NewReverseProxy(target *url.URL, accessToken *string) *httputil.ReverseProxy {
  proxy := httputil.NewSingleHostReverseProxy(target)

  originalDirector := proxy.Director
  proxy.Director = func(req *http.Request) {
    originalDirector(req)
    req.Header.Set("X-Proxy", "knot " + build.Version)

    if accessToken != nil {
      req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", *accessToken))
    }
  }

  proxy.Transport = &http.Transport{
    TLSClientConfig: &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")},
    MaxConnsPerHost: 32 * 2,
    MaxIdleConns: 32 * 2,
    MaxIdleConnsPerHost: 32,
    IdleConnTimeout: 30 * time.Second,
    DisableCompression: true,
  }

  return proxy
}
