package proxy

import (
	"net/http"

	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
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

	// Get the space session
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || agentSession.SSHPort == 0 {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Open a new stream to the agent
	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Write the command
	if err := msg.WriteCommand(stream, msg.MSG_PROXY_TCP_PORT); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := msg.WriteMessage(stream, &msg.TcpPort{
		Port: uint16(agentSession.SSHPort),
	}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Upgrade the connection to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	copier := util.NewCopier(stream, ws)
	copier.Run()
}
