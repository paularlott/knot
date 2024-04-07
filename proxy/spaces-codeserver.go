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
	if err != nil || agentState == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Look up the IP + Port from consul / DNS
	target, _ := url.Parse(fmt.Sprintf("%s/code-server/", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL()), "/")))
	r.URL.Path = strings.TrimPrefix(r.URL.Path, fmt.Sprintf("/proxy/spaces/%s/code-server", spaceId))

	token := "Bearer " + agentState.AccessToken
	proxy := util.NewReverseProxy(target, &token)
	proxy.ServeHTTP(w, r)
}
