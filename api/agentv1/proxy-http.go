package agentv1

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

var (
  HttpPortMap map[string]bool
)

func agentProxyHTTP(w http.ResponseWriter, r *http.Request) {
  port := chi.URLParam(r, "port")

  log.Debug().Msgf("proxy of http port %s", port)

  // Check port is in the list of allowed ports
  if !HttpPortMap[port] {
    log.Error().Msgf("proxy of http port %s is not allowed", port)
    w.WriteHeader(http.StatusForbidden)
    return
  }

  target, _ := url.Parse("http://127.0.0.1:" + port)
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/http/" + port)

  proxy.ServeHTTP(w, r)
}
