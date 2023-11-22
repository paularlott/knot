package web

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
)

func HandleSpacesCodeServerProxy(w http.ResponseWriter, r *http.Request) {
  spaceId := chi.URLParam(r, "space_id")

  // Get the space auth
  agentState, ok := database.AgentStateGet(spaceId)
  if !ok {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Load the space
  db := database.GetInstance()
  space, err := db.GetSpace(spaceId)
  if err != nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Look up the IP + Port from consul / DNS
  target, _ := url.Parse(fmt.Sprintf("%s/code-server/", strings.TrimSuffix(util.ResolveSRVHttp(space.AgentURL, viper.GetString("agent.nameserver")), "/")))
  proxy := httputil.NewSingleHostReverseProxy(target)

  originalDirector := proxy.Director
  proxy.Director = func(r *http.Request) {
    originalDirector(r)
    r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", agentState.AccessToken))
  }

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/spaces/%s/code-server", spaceId))

  proxy.ServeHTTP(w, r)
}
