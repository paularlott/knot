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

// resolveSpaceForFileOps does the common space resolution, permission checks,
// deployed check, and session lookup shared by every file-search/edit handler.
// On failure it writes the HTTP response and returns nil; on success it returns
// the agent session.
func resolveSpaceForFileOps(w http.ResponseWriter, r *http.Request) *agent_server.Session {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return nil
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to get template"})
		return nil
	}

	if !template.WithRunCommand {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "File operations are not allowed in this space"})
		return nil
	}

	if space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to access files in this space"})
		return nil
	}

	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return nil
	}

	session := agent_server.GetSession(space.Id)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return nil
	}
	return session
}

// HandleGrep searches file contents in a space.
func HandleGrep(w http.ResponseWriter, r *http.Request) {
	session := resolveSpaceForFileOps(w, r)
	if session == nil {
		return
	}

	var req msg.GrepMessage
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	ch, err := session.SendGrep(&req)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send grep to agent: %v", err)})
		return
	}
	resp := <-ch
	if resp == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, resp)
}

// HandleFind finds files/directories in a space.
func HandleFind(w http.ResponseWriter, r *http.Request) {
	session := resolveSpaceForFileOps(w, r)
	if session == nil {
		return
	}

	var req msg.FindMessage
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if req.Path == "" {
		req.Path = "."
	}
	if req.Type == "" {
		req.Type = "any"
	}

	ch, err := session.SendFind(&req)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send find to agent: %v", err)})
		return
	}
	resp := <-ch
	if resp == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, resp)
}

// HandleSed performs in-place edits or capture extraction in a space.
func HandleSed(w http.ResponseWriter, r *http.Request) {
	session := resolveSpaceForFileOps(w, r)
	if session == nil {
		return
	}

	var req msg.SedMessage
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}
	switch req.Mode {
	case "replace", "replace_pattern", "extract":
	default:
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("invalid sed mode: %q", req.Mode)})
		return
	}

	ch, err := session.SendSed(&req)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send sed to agent: %v", err)})
		return
	}
	resp := <-ch
	if resp == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, resp)
}

// HandleEditFile performs a targeted search-and-replace edit in a space.
func HandleEditFile(w http.ResponseWriter, r *http.Request) {
	session := resolveSpaceForFileOps(w, r)
	if session == nil {
		return
	}

	var req msg.EditFileMessage
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}
	if req.Search == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "search is required"})
		return
	}

	ch, err := session.SendEditFile(&req)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send edit to agent: %v", err)})
		return
	}
	resp := <-ch
	if resp == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, resp)
}
