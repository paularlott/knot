package web

import (
	"fmt"
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
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
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
		Persistent: request.Persistent,
		Force:      request.Force,
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
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
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
			Persistent: fwd.Persistent,
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
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
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

func HandlePortApply(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil || (space.UserId != user.Id && !space.IsSharedWith(user.Id)) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session
	agentSession := agent_server.GetSession(spaceId)
	if agentSession == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Read the apply request
	var request apiclient.PortApplyRequest
	if err := rest.DecodeRequestBody(w, r, &request); err != nil {
		log.WithError(err).Error("Failed to decode port apply request")
		return
	}

	if len(request.Forwards) == 0 {
		writeJSONError(w, http.StatusBadRequest, "At least one forward is required")
		return
	}

	// Validate each forward
	for _, fwd := range request.Forwards {
		if fwd.LocalPort < 1 || fwd.LocalPort > 65535 {
			writeJSONError(w, http.StatusBadRequest, "Invalid local port, must be between 1 and 65535")
			return
		}
		if fwd.RemotePort < 1 || fwd.RemotePort > 65535 {
			writeJSONError(w, http.StatusBadRequest, "Invalid remote port, must be between 1 and 65535")
			return
		}
		if fwd.Space == "" {
			writeJSONError(w, http.StatusBadRequest, "Target space name is required")
			return
		}
	}

	// Fetch current forwards from the agent
	currentList, err := agentSession.SendPortList()
	if err != nil {
		log.WithError(err).Error("Failed to get current port list from agent")
		writeJSONError(w, http.StatusInternalServerError, "Failed to get current port forwards")
		return
	}

	// Build a map of current forwards keyed by local_port
	currentMap := make(map[uint16]*msg.PortForwardInfo)
	for i := range currentList.Forwards {
		currentMap[currentList.Forwards[i].LocalPort] = &currentList.Forwards[i]
	}

	// Build a set of desired local_ports
	desiredSet := make(map[uint16]apiclient.PortForwardRequest)
	for _, fwd := range request.Forwards {
		desiredSet[fwd.LocalPort] = fwd
	}

	var stopped []apiclient.PortForwardInfo
	var applied []apiclient.PortForwardInfo
	var errors []string

	// Phase 1: Stop forwards that are not in the desired list or have changed
	for port, current := range currentMap {
		desired, exists := desiredSet[port]
		needsStop := !exists || current.Space != desired.Space || current.RemotePort != desired.RemotePort

		if needsStop {
			stopMsg := &msg.PortStopRequest{LocalPort: port}
			resp, err := agentSession.SendPortStop(stopMsg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to stop port %d: %v", port, err))
			} else if !resp.Success {
				errors = append(errors, fmt.Sprintf("Failed to stop port %d: %s", port, resp.Error))
			} else {
				stopped = append(stopped, apiclient.PortForwardInfo{
					LocalPort:  current.LocalPort,
					Space:      current.Space,
					RemotePort: current.RemotePort,
					Persistent: current.Persistent,
				})
			}
		}
	}

	// Phase 2: Start forwards that are new or were just stopped
	for _, fwd := range request.Forwards {
		current, exists := currentMap[fwd.LocalPort]
		needsStart := !exists || current.Space != fwd.Space || current.RemotePort != fwd.RemotePort

		if needsStart {
			portForwardMsg := &msg.PortForwardRequest{
				LocalPort:  uint16(fwd.LocalPort),
				Space:      fwd.Space,
				RemotePort: uint16(fwd.RemotePort),
				Persistent: fwd.Persistent,
				Force:      fwd.Force,
			}
			resp, err := agentSession.SendPortForward(portForwardMsg)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to forward port %d: %v", fwd.LocalPort, err))
			} else if !resp.Success {
				errors = append(errors, fmt.Sprintf("Failed to forward port %d: %s", fwd.LocalPort, resp.Error))
			} else {
				applied = append(applied, apiclient.PortForwardInfo{
					LocalPort:  uint16(fwd.LocalPort),
					Space:      fwd.Space,
					RemotePort: uint16(fwd.RemotePort),
					Persistent: fwd.Persistent,
				})
			}
		}
	}

	response := apiclient.PortApplyResponse{
		Applied: applied,
		Stopped: stopped,
		Errors:  errors,
	}
	rest.WriteResponse(http.StatusOK, w, r, response)
}
