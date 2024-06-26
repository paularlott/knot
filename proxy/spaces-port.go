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
	agentState, err := database.GetCacheInstance().GetAgentState(space.Id)
	if err != nil || agentState == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Look up the IP + Port from consul / DNS
	target, _ := url.Parse(fmt.Sprintf("%s/tcp/%s", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL()), "/"), port))
	r.URL.Path = ""

	token := "Bearer " + agentState.AccessToken
	proxy := util.NewReverseProxy(target, &token)
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
	if err != nil || space == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space auth
	agentState, err := database.GetCacheInstance().GetAgentState(space.Id)
	if err != nil || agentState == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var target *url.URL

	// If the last part is VNC then doing proxy for HTTP based VNC server
	if domainParts[2] == "vnc" {
		target, _ = url.Parse(fmt.Sprintf("%s/vnc/", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL()), "/")))
	} else {
		target, _ = url.Parse(fmt.Sprintf("%s/http/%s", strings.TrimSuffix(util.ResolveSRVHttp(space.GetAgentURL()), "/"), domainParts[2]))
	}

	token := "Bearer " + agentState.AccessToken
	proxy := util.NewReverseProxy(target, &token)
	proxy.ServeHTTP(w, r)
}
