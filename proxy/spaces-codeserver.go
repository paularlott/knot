package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/viper"
)

func HandleSpacesCodeServerProxy(w http.ResponseWriter, r *http.Request) {
  spaceId := chi.URLParam(r, "space_id")
  user := r.Context().Value("user").(*model.User)

  // Load the space
  db := database.GetInstance()
  space, err := db.GetSpace(spaceId)
  if err != nil || space == nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Check user access to the space
  if space.UserId != user.Id {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Get the space auth
  agentState, err := database.GetCacheInstance().GetAgentState(space.Id)
  if err != nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Look up the IP + Port from consul / DNS
  target, _ := url.Parse(fmt.Sprintf("%s/code-server/", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("agent.nameserver")), "/")))
  proxy := httputil.NewSingleHostReverseProxy(target)

  originalDirector := proxy.Director
  proxy.Director = func(r *http.Request) {
    originalDirector(r)
    r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", agentState.AccessToken))
  }

  if viper.GetBool("tls_skip_verify") {
    proxy.Transport = &http.Transport{
      TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
    }
  }

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/code-server", spaceId))

  proxy.ServeHTTP(w, r)
}
