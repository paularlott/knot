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

func HandlePortForward(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && space.SharedWithUserId != user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the port forward request
	var request apiclient.PortForwardRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port forward request")
		return
	}

	// Validate the request
	if request.LocalPort < 1 || request.LocalPort > 65535 {
		writeJSONError(w, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
		return
	}

	if request.RemotePort < 1 || request.RemotePort > 65535 {
		writeJSONError(w, http.StatusBadRequest, "Invalid remote port, must be between 1 and 65535")
		return
	}

	if request.Space == "" {
		writeJSONError(w, http.StatusBadRequest, "Target space name is required")
		return
	}

	// Send the port forward message to the agent
	portForwardMsg := &msg.PortForwardRequest{
		LocalPort:  uint16(request.LocalPort),
		Space:      request.Space,
		RemotePort: uint16(request.RemotePort),
	}

	// Send the command to the agent and get the response
	response, err := agentSession.SendPortForward(portForwardMsg)
	if err != nil {
		log.WithError(err).Error("Failed to send port forward command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSONError(w, http.StatusInternalServerError, response.Error)
	}
}

func HandlePortList(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && space.SharedWithUserId != user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Send the command to the agent and get the response
	response, err := agentSession.SendPortList()
	if err != nil {
		log.WithError(err).Error("failed to send port list command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	forwards := make([]apiclient.PortForwardInfo, 0, len(response.Forwards))
	for _, fwd := range response.Forwards {
		forwards = append(forwards, apiclient.PortForwardInfo{
			LocalPort:  fwd.LocalPort,
			Space:      fwd.Space,
			RemotePort: fwd.RemotePort,
		})
	}

	portListResponse := apiclient.PortListResponse{
		Forwards: forwards,
	}

	rest.WriteResponse(http.StatusOK, w, r, portListResponse)
}

func HandlePortStop(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && space.SharedWithUserId != user.Id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the port stop request
	var request apiclient.PortStopRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port stop request")
		return
	}

	// Validate the request
	if request.LocalPort < 1 || request.LocalPort > 65535 {
		writeJSONError(w, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
		return
	}

	// Send the port stop message to the agent
	portStopMsg := &msg.PortStopRequest{
		LocalPort: uint16(request.LocalPort),
	}

	// Send the command to the agent and get the response
	response, err := agentSession.SendPortStop(portStopMsg)
	if err != nil {
		log.WithError(err).Error("Failed to send port stop command to agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to send command to agent")
		return
	}

	if response.Success {
		w.WriteHeader(http.StatusOK)
	} else {
		writeJSONError(w, http.StatusInternalServerError, response.Error)
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	rest.WriteResponse(status, w, nil, map[string]interface{}{
		"success": false,
		"error":   message,
	})
}
