package agentv1

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/util"

	"github.com/spf13/viper"
)

func agentProxyVNCHttp(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(fmt.Sprintf("https://127.0.0.1:%d", vncHttpServerPort))

	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/vnc")

	token := "Basic " + base64.StdEncoding.EncodeToString([]byte("knot:"+viper.GetString("agent.service_password")))
	proxy := util.NewReverseProxy(target, &token)
	proxy.ServeHTTP(w, r)
}
