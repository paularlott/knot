package proxy

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"

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

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || agentSession.SSHPort == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	proxyAgentPort(w, r, agentSession, uint16(agentSession.SSHPort))
}
