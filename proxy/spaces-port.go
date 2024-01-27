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

func HandleSpacesPortProxy(w http.ResponseWriter, r *http.Request) {
  user := r.Context().Value("user").(*model.User)
  spaceName := chi.URLParam(r, "space_name")
  port := chi.URLParam(r, "port")

  // Load the space
  db := database.GetInstance()
  space, err := db.GetSpaceByName(user.Id, spaceName)
  if err != nil {
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
  target, _ := url.Parse(fmt.Sprintf("%s/tcp/%s", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("agent.nameserver")), "/"), port))
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

  r.URL.Path = ""

  proxy.ServeHTTP(w, r)
}

func HandleSpacesWebPortProxy(w http.ResponseWriter, r *http.Request) {
  // Split the domain into parts
  domainParts := strings.Split(r.Host, ".")
  if len(domainParts) < 1 {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Extract the user, space and port from the domain
  domainParts = strings.Split(domainParts[0], "--")
  if len(domainParts) != 3 {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  db := database.GetInstance()

  // Load the user
  user, err := db.GetUserByUsername(domainParts[0])
  if err != nil {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Load the space
  space, err := db.GetSpaceByName(user.Id, domainParts[1])
  if err != nil {
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
  target, _ := url.Parse(fmt.Sprintf("%s/http/%s", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("agent.nameserver")), "/"), domainParts[2]))
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

  proxy.ServeHTTP(w, r)
}
