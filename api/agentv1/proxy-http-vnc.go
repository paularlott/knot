package agentv1

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/spf13/viper"
)

func agentProxyVNCHttp(w http.ResponseWriter, r *http.Request) {
  target, _ := url.Parse(fmt.Sprintf("https://127.0.0.1:%d", vncHttpServerPort))
  proxy := httputil.NewSingleHostReverseProxy(target)

  fmt.Println("Proxying to", target)
  fmt.Println("password ", viper.GetString("agent.service_password"))

  r.URL.Path = strings.TrimPrefix(r.URL.Path, "/vnc")

  originalDirector := proxy.Director
  proxy.Director = func(r *http.Request) {
    originalDirector(r)
    encoded := base64.StdEncoding.EncodeToString([]byte("knot:" + viper.GetString("agent.service_password")))
    r.Header.Set("Authorization", "Basic "+encoded)
  }

  if viper.GetBool("tls_skip_verify") {
    proxy.Transport = &http.Transport{
      TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
  }

  proxy.ServeHTTP(w, r)
}
