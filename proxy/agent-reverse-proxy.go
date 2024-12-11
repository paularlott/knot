package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/paularlott/knot/build"

	"github.com/spf13/viper"
)

func createAgentReverseProxy(targetURL *url.URL, stream net.Conn, accessToken *string, host string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Proxy", "knot "+build.Version)
		req.URL.Scheme = "http" // Force http as the agent will upgrade the connection to https

		// Set the host header
		if host != "" {
			req.Host = host
		} else {
			req.Host = targetURL.Host // Set the Host header
		}

		if accessToken != nil {
			req.Header.Set("Authorization", *accessToken)
		}
	}

	proxy.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")},
		MaxConnsPerHost:     32 * 2,
		MaxIdleConns:        32 * 2,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return stream, nil
		},
	}

	return proxy
}
