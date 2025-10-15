package proxy

import (
	"net/http"
	"strconv"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

func handlePortTunnel(w http.ResponseWriter, r *http.Request) {
	var err error

	user := r.Context().Value("user").(*model.User)

	spaceName := r.PathValue("space_name")
	if !validate.Name(spaceName) {
		log.Debug("Invalid space name", "space_name", spaceName)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	port := r.PathValue("port")
	portUInt, err := strconv.ParseUint(port, 10, 16)
	if err != nil || !validate.IsNumber(int(portUInt), 0, 65535) {
		log.Debug("Invalid port", "port", port)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil {
		log.Error("Error loading space", "error", err, "space_name", spaceName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil {
		log.Debug("Space session not found", "space_name", spaceName)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if err := tunnel_server.HandleCreatePortTunnel(w, r, agentSession.MuxSession, agentSession.Id, uint16(portUInt), space, user); err != nil {
		return
	}
}
