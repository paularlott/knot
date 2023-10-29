package agent

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func proxyCodeServer(w http.ResponseWriter, r *http.Request) {
  target, _ := url.Parse("http://127.0.0.1:" + codeServerPort)
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/code-server")

  proxy.ServeHTTP(w, r)
}
