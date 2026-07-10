package web

import (
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

// HandleTunnelStart starts an agent-owned web tunnel inside a space. The request
// is relayed to the space's agent, which owns the tunnel for the life of the
// agent (daemon implied).
func HandleTunnelStart(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var request apiclient.SpaceTunnelStartRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode tunnel start request")
		return
	}

	if request.Protocol != "http" && request.Protocol != "https" {
		writeJSONError(w, http.StatusBadRequest, "Invalid protocol, must be http or https")
		return
	}
	if request.Port < 1 || request.Port > 65535 {
		writeJSONError(w, http.StatusBadRequest, "Invalid port, must be between 1 and 65535")
		return
	}
	if !validate.Name(request.Name) {
		writeJSONError(w, http.StatusBadRequest, "Invalid name, must be all lowercase and only contain letters, numbers and dashes")
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		writeJSONError(w, http.StatusConflict, "Space is not running")
		return
	}

	response, err := agentSession.SendTunnelStart(&msg.TunnelStartRequest{
		Protocol: request.Protocol,
		Port:     request.Port,
		Name:     request.Name,
	})
	if err != nil {
		log.WithError(err).Error("Failed to send tunnel start command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		rest.WriteResponse(http.StatusOK, w, r, apiclient.SpaceTunnelStartResponse{
			Success: true,
			URL:     response.URL,
		})
	} else {
		// Agent operational failure (e.g. name in use, connection failed):
		// return 200 with a parseable body so the client can surface the error.
		rest.WriteResponse(http.StatusOK, w, r, apiclient.SpaceTunnelStartResponse{
			Success: false,
			Error:   response.Error,
		})
	}
}

// HandleTunnelList lists the agent-owned web tunnels in a space.
func HandleTunnelList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		// Space not running — no live tunnels (they are not persisted).
		rest.WriteResponse(http.StatusOK, w, r, apiclient.SpaceTunnelListResponse{})
		return
	}

	response, err := agentSession.SendTunnelList()
	if err != nil {
		log.WithError(err).Error("Failed to send tunnel list command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	tunnels := make([]apiclient.SpaceTunnelInfo, 0, len(response.Tunnels))
	for _, t := range response.Tunnels {
		tunnels = append(tunnels, apiclient.SpaceTunnelInfo{
			Port:     t.Port,
			Protocol: t.Protocol,
			Name:     t.Name,
			URL:      t.URL,
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.SpaceTunnelListResponse{Tunnels: tunnels})
}

// HandleTunnelStop stops an agent-owned web tunnel in a space by name.
func HandleTunnelStop(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var request apiclient.SpaceTunnelStopRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode tunnel stop request")
		return
	}
	if request.Name == "" {
		writeJSONError(w, http.StatusBadRequest, "Tunnel name is required")
		return
	}

	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		writeJSONError(w, http.StatusConflict, "Space is not running")
		return
	}

	response, err := agentSession.SendTunnelStop(&msg.TunnelStopRequest{Name: request.Name})
	if err != nil {
		log.WithError(err).Error("Failed to send tunnel stop command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSONError(w, http.StatusInternalServerError, response.Error)
	}
}
