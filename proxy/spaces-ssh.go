package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
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
	if err != nil || agentState == nil || agentState.SSHPort == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Look up the IP + Port from consul / DNS
	target, _ := url.Parse(fmt.Sprintf("%s/tcp/%d", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL()), "/"), agentState.SSHPort))
	r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/ssh", spaceName))

	token := "Bearer " + agentState.AccessToken
	proxy := util.NewReverseProxy(target, &token)
	proxy.ServeHTTP(w, r)
}
