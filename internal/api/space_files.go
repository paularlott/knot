package api

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
)

type ReadFileRequest struct {
	Path string `json:"path"`
}

type ReadFileResponse struct {
	Success bool   `json:"success"`
	Content string `json:"content,omitempty"`
	Size    int    `json:"size,omitempty"`
	Error   string `json:"error,omitempty"`
}

type WriteFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type WriteFileResponse struct {
	Success      bool   `json:"success"`
	BytesWritten int    `json:"bytes_written,omitempty"`
	Error        string `json:"error,omitempty"`
}

func HandleReadSpaceFile(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	var req ReadFileRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Path == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "path is required"})
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to get template"})
		return
	}

	if !template.WithRunCommand {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "File operations are not allowed in this space"})
		return
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to read files in this space"})
		return
	}

	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return
	}

	copyCmd := &msg.CopyFileMessage{
		SourcePath: req.Path,
		Direction:  "from_space",
		Workdir:    "",
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send read file command to agent: %v", err)})
		return
	}

	response := <-responseChannel
	if response == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}

	result := ReadFileResponse{
		Success: response.Success,
	}

	if !response.Success {
		result.Error = response.Error
	} else {
		result.Content = string(response.Content)
		result.Size = len(response.Content)
	}

	rest.WriteResponse(http.StatusOK, w, r, result)
}

func HandleWriteSpaceFile(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	var req WriteFileRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Path == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "path is required"})
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to get template"})
		return
	}

	if !template.WithRunCommand {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "File operations are not allowed in this space"})
		return
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to write files in this space"})
		return
	}

	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return
	}

	var contentBytes []byte
	if decoded, err := base64.StdEncoding.DecodeString(req.Content); err == nil {
		contentBytes = decoded
	} else {
		contentBytes = []byte(req.Content)
	}

	copyCmd := &msg.CopyFileMessage{
		DestPath:  req.Path,
		Content:   contentBytes,
		Direction: "to_space",
		Workdir:   "",
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send write file command to agent: %v", err)})
		return
	}

	response := <-responseChannel
	if response == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}

	result := WriteFileResponse{
		Success: response.Success,
	}

	if !response.Success {
		result.Error = response.Error
	} else {
		result.BytesWritten = len(contentBytes)
	}

	rest.WriteResponse(http.StatusOK, w, r, result)
}
