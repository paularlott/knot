package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

type UnifiedScriptExecuteRequest struct {
	ScriptId   string   `json:"script_id,omitempty"`
	ScriptName string   `json:"script_name,omitempty"`
	Content    string   `json:"content,omitempty"`
	Arguments  []string `json:"arguments"`
}

func HandleExecuteScript(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")

	request := UnifiedScriptExecuteRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	// Support lookup by both ID and name
	var space *model.Space
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}
	spaceId = space.Id // Use the resolved ID for subsequent operations

	if !user.HasPermission(model.PermissionManageSpaces) && space.UserId != user.Id {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to access this space"})
		return
	}

	var scriptContent string
	var scriptId string
	var scriptName string

	// Determine script source: ID, name, or content
	if request.ScriptId != "" {
		script, err := db.GetScript(request.ScriptId)
		if err != nil || script.IsDeleted || !script.Active {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
			return
		}
		if !service.CanUserExecuteScript(user, script) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute this script"})
			return
		}
		scriptContent = script.Content
		scriptId = script.Id
		scriptName = script.Name
	} else if request.ScriptName != "" {
		script, err := service.ResolveScriptByName(request.ScriptName, user.Id)
		if err != nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
			return
		}
		if !service.CanUserExecuteScript(user, script) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute this script"})
			return
		}
		scriptContent = script.Content
		scriptId = script.Id
		scriptName = script.Name
	} else if request.Content != "" {
		if len(request.Content) > 4*1024*1024 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Script content exceeds 4MB limit"})
			return
		}
		if !user.HasPermission(model.PermissionExecuteScripts) && !user.HasPermission(model.PermissionExecuteOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute scripts"})
			return
		}
		scriptContent = request.Content
		scriptName = "inline"
	} else {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Must provide script_id, script_name, or content"})
		return
	}

	session := agent_server.GetSession(space.Id)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Space agent is not connected"})
		return
	}

	// Unified script execution runs without timeout
	// Prepend script name to arguments (argv[0] convention)
	argv := append([]string{scriptName}, request.Arguments...)
	execMsg := &msg.ExecuteScriptMessage{
		Content:      scriptContent,
		Arguments:    argv,
		IsSystemCall: false,
	}

	respChan, err := session.SendExecuteScript(execMsg)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("failed to send script to agent: %v", err)})
		return
	}

	resp := <-respChan

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptExecute,
		fmt.Sprintf("Executed script %s in space %s", scriptName, space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       scriptId,
			"script_name":     scriptName,
			"space_id":        space.Id,
			"space_name":      space.Name,
		},
	)

	response := apiclient.ScriptExecuteResponse{
		Output:   resp.Output,
		ExitCode: resp.ExitCode,
	}
	if !resp.Success {
		response.Error = resp.Error
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}
