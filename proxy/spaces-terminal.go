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

func HandleSpacesTerminalProxy(w http.ResponseWriter, r *http.Request) {
  spaceId := chi.URLParam(r, "space_id")
  shell := chi.URLParam(r, "shell")
  user := r.Context().Value("user").(*model.User)

  // Load the space
  db := database.GetInstance()
  space, err := db.GetSpace(spaceId)
  if err != nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Check user access to the space
  if space.UserId != user.Id {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Get the space auth
  agentState, ok := database.AgentStateGet(space.Id)
  if !ok {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Look up the IP + Port from consul / DNS
  target, _ := url.Parse(fmt.Sprintf("%s/terminal/%s", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("agent.nameserver")), "/"), shell))
  proxy := httputil.NewSingleHostReverseProxy(target)

  originalDirector := proxy.Director
  proxy.Director = func(r *http.Request) {
    originalDirector(r)
    r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", agentState.AccessToken))
  }

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/terminal/%s", spaceId, shell))

  proxy.ServeHTTP(w, r)
}
