package proxy

import (
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleSpacesSSHProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceName := r.PathValue("space_name")
	if !validate.Name(spaceName) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Check if the user has permission to use SSH
	if !user.HasPermission(model.PermissionUseSSH) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

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

	// SSH sessions are audited at authentication time: the agent-managed sshd
	// reports each public-key attempt (success or failure, with the key
	// fingerprint), which is the signal a raw TCP proxy cannot see.

	proxyAgentPort(w, r, agentSession, uint16(agentSession.SSHPort))
}
