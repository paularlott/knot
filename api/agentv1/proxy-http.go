package agentv1

import (
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

var (
	HttpPortMap  map[string]bool
	HttpsPortMap map[string]bool
)

func agentProxyHTTP(w http.ResponseWriter, r *http.Request) {
	var target *url.URL

	port := chi.URLParam(r, "port")

	log.Debug().Msgf("proxy of http port %s", port)

	// If port in http map then proxy to http
	if HttpPortMap[port] {
		target, _ = url.Parse("http://127.0.0.1:" + port)
	} else if HttpsPortMap[port] {
		// If port in https map then proxy to https
		target, _ = url.Parse("https://127.0.0.1:" + port)
	} else {
		log.Error().Msgf("proxy of http port %s is not allowed", port)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxConnsPerHost:     32 * 2,
		MaxIdleConns:        32 * 2,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
	}

	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/http/"+port)

	proxy.ServeHTTP(w, r)
}
