package proxy

import (
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

func HandleSpacesSSHProxy(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
  spaceName := chi.URLParam(r, "space_name")

  // Load the space
  db := database.GetInstance()
  space, err := db.GetSpaceByName(user.Id, spaceName)
  if err != nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Get the space auth
  agentState, ok := database.AgentStateGet(space.Id)
  if !ok || agentState.SSHPort == 0 {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Look up the IP + Port from consul / DNS
  target, _ := url.Parse(fmt.Sprintf("%s/tcp/%d", strings.TrimSuffix(util.ResolveSRVHttp(space.AgentURL, viper.GetString("agent.nameserver")), "/"), agentState.SSHPort))
  proxy := httputil.NewSingleHostReverseProxy(target)

  originalDirector := proxy.Director
  proxy.Director = func(r *http.Request) {
    originalDirector(r)
    r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", agentState.AccessToken))
  }

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/ssh", spaceName))

  proxy.ServeHTTP(w, r)
}
