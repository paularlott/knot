package proxy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/paularlott/knot/api/agentv1"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/rest"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
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
  agentState, err := database.GetCacheInstance().GetAgentState(space.Id)
  if err != nil || agentState.SSHPort == 0 {
    w.WriteHeader(http.StatusNotFound)
    return
  }

  // Write the users SSH public key to the agent
  if user.SSHPublicKey != "" {
    log.Debug().Msg("Sending SSH public key to agent")

    // Send the public SSH key to the agent
    client := rest.NewClient(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("server.namespace")), agentState.AccessToken, viper.GetBool("tls_skip_verify"))
    if !agentv1.CallAgentUpdateAuthorizedKeys(client, user.SSHPublicKey) {
      log.Debug().Msg("Failed to send SSH public key to agent")
      w.WriteHeader(http.StatusInternalServerError)
      return
    }
  }

  // Look up the IP + Port from consul / DNS
  target, _ := url.Parse(fmt.Sprintf("%s/tcp/%d", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL(), viper.GetString("agent.nameserver")), "/"), agentState.SSHPort))
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

  r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/ssh", spaceName))

  proxy.ServeHTTP(w, r)
}
