package agent

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

func proxyHTTP(w http.ResponseWriter, r *http.Request) {
  port := chi.URLParam(r, "port")

  target, _ := url.Parse("http://127.0.0.1:" + port)
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/http/" + port)

  proxy.ServeHTTP(w, r)
}
