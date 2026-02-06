package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

type RunCommandRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout"`
	Workdir string   `json:"workdir,omitempty"`
}

type RunCommandResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

func HandleRunCommand(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")

	var req RunCommandRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	// Validate command
	if req.Command == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "command is required"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	// Support lookup by both ID and name
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	// Check permissions
	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to run commands in this space"})
		return
	}

	// Check if space is running
	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return
	}

	// Get agent session
	session := agent_server.GetSession(space.Id)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return
	}

	// Send run command message
	runCmd := &msg.RunCommandMessage{
		Command: req.Command,
		Args:    req.Args,
		Timeout: req.Timeout,
		Workdir: req.Workdir,
	}

	responseChannel, err := session.SendRunCommand(runCmd)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send run command to agent: %v", err)})
		return
	}

	response := <-responseChannel
	if response == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}

	result := RunCommandResponse{
		Success: response.Success,
	}

	if !response.Success {
		result.Error = response.Error
	} else {
		result.Output = string(response.Output)
	}

	rest.WriteResponse(http.StatusOK, w, r, result)
}
