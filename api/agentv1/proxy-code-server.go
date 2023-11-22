package agentv1

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

func agentProxyCodeServer(w http.ResponseWriter, r *http.Request) {
  target, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", codeServerPort))
  proxy := httputil.NewSingleHostReverseProxy(target)

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/code-server")

  proxy.ServeHTTP(w, r)
}
