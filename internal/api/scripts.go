package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetScripts(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	scripts, err := db.GetScripts()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.ScriptList{
		Count:   0,
		Scripts: []apiclient.ScriptInfo{},
	}

	for _, script := range scripts {
		if script.IsDeleted {
			continue
		}

		if !user.HasPermission(model.PermissionManageScripts) {
			if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
				continue
			}
		}

		response.Scripts = append(response.Scripts, apiclient.ScriptInfo{
			Id:          script.Id,
			Name:        script.Name,
			Description: script.Description,
			Groups:      script.Groups,
			Active:      script.Active,
			ScriptType:  script.ScriptType,
			Timeout:     script.Timeout,
		})
		response.Count++
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleGetScript(w http.ResponseWriter, r *http.Request) {
	scriptId := r.PathValue("script_id")
	if !validate.UUID(scriptId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script, err := db.GetScript(scriptId)
	if err != nil || script.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageScripts) {
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptDetails{
		Id:                 script.Id,
		Name:               script.Name,
		Description:        script.Description,
		Content:            script.Content,
		Groups:             script.Groups,
		Active:             script.Active,
		ScriptType:         script.ScriptType,
		MCPInputSchemaToml: script.MCPInputSchemaToml,
		MCPKeywords:        script.MCPKeywords,
		Timeout:            script.Timeout,
	})
}

func HandleCreateScript(w http.ResponseWriter, r *http.Request) {
	request := apiclient.ScriptCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.VarName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	if len(request.Content) > 4*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Script content exceeds 4MB limit"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script := model.NewScript(
		request.Name,
		request.Description,
		request.Content,
		request.Groups,
		request.Active,
		request.ScriptType,
		request.MCPInputSchemaToml,
		request.MCPKeywords,
		request.Timeout,
		user.Id,
	)

	err = db.SaveScript(script, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipScript(script)
	sse.PublishScriptsChanged(script.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptCreate,
		fmt.Sprintf("Created script %s", script.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       script.Id,
			"script_name":     script.Name,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.ScriptCreateResponse{
		Status: true,
		Id:     script.Id,
	})
}

func HandleUpdateScript(w http.ResponseWriter, r *http.Request) {
	scriptId := r.PathValue("script_id")
	if !validate.UUID(scriptId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script ID"})
		return
	}

	request := apiclient.ScriptUpdateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.VarName(request.Name) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	if len(request.Content) > 4*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Script content exceeds 4MB limit"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script, err := db.GetScript(scriptId)
	if err != nil || script.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	script.Name = request.Name
	script.Description = request.Description
	script.Content = request.Content
	script.Groups = request.Groups
	script.Active = request.Active
	script.ScriptType = request.ScriptType
	script.MCPInputSchemaToml = request.MCPInputSchemaToml
	script.MCPKeywords = request.MCPKeywords
	script.Timeout = request.Timeout
	script.UpdatedUserId = user.Id
	script.UpdatedAt = hlc.Now()

	err = db.SaveScript(script, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipScript(script)
	sse.PublishScriptsChanged(script.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptUpdate,
		fmt.Sprintf("Updated script %s", script.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       script.Id,
			"script_name":     script.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteScript(w http.ResponseWriter, r *http.Request) {
	scriptId := r.PathValue("script_id")
	if !validate.UUID(scriptId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script ID"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script, err := db.GetScript(scriptId)
	if err != nil || script.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	scriptName := script.Name
	script.Name = script.Id
	script.IsDeleted = true
	script.UpdatedUserId = user.Id
	script.UpdatedAt = hlc.Now()

	err = db.SaveScript(script, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipScript(script)
	sse.PublishScriptsDeleted(script.Id)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptDelete,
		fmt.Sprintf("Deleted script %s", scriptName),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       scriptId,
			"script_name":     scriptName,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetScriptByName(w http.ResponseWriter, r *http.Request) {
	scriptName := r.PathValue("script_name")
	if !validate.VarName(scriptName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script, err := db.GetScriptByName(scriptName)
	if err != nil || script.IsDeleted || script.ScriptType != "script" {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageScripts) {
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptDetails{
		Id:                 script.Id,
		Name:               script.Name,
		Description:        script.Description,
		Content:            script.Content,
		Groups:             script.Groups,
		Active:             script.Active,
		ScriptType:         script.ScriptType,
		MCPInputSchemaToml: script.MCPInputSchemaToml,
		MCPKeywords:        script.MCPKeywords,
		Timeout:            script.Timeout,
	})
}

func HandleGetScriptLibrary(w http.ResponseWriter, r *http.Request) {
	libraryName := r.PathValue("library_name")
	if !validate.VarName(libraryName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid library name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	script, err := db.GetScriptByName(libraryName)
	if err != nil || script.IsDeleted || !script.Active || script.ScriptType != "lib" {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Library not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageScripts) {
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Library not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptLibraryResponse{
		Name:    script.Name,
		Content: script.Content,
	})
}

func HandleExecuteScript(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")
	scriptId := r.PathValue("script_id")

	if !validate.UUID(spaceId) || !validate.UUID(scriptId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid ID"})
		return
	}

	request := apiclient.ScriptExecuteRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageSpaces) && space.UserId != user.Id {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to access this space"})
		return
	}

	script, err := db.GetScript(scriptId)
	if err != nil || script.IsDeleted || !script.Active {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	if !user.HasPermission(model.PermissionExecuteScripts) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute scripts"})
		return
	}

	if !user.HasPermission(model.PermissionManageScripts) {
		if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute this script"})
			return
		}
	}

	var output string
	var execErr error

	// Check if space has an agent session (remote execution)
	session := agent_server.GetSession(space.Id)
	if session != nil {
		timeout := script.Timeout
		if timeout == 0 {
			cfg := config.GetServerConfig()
			timeout = cfg.MaxScriptExecutionTime
		}
		execMsg := &msg.ExecuteScriptMessage{
			Content:      script.Content,
			Arguments:    request.Arguments,
			Timeout:      timeout,
			IsSystemCall: false,
		}

		respChan, err := session.SendExecuteScript(execMsg)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("failed to send script to agent: %v", err)})
			return
		}

		resp := <-respChan
		if !resp.Success {
			output = resp.Output
			execErr = fmt.Errorf("%s", resp.Error)
		} else {
			output = resp.Output
		}
	} else {
		// Local execution
		output, execErr = service.ExecuteScriptLocally(script, request.Arguments)
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptExecute,
		fmt.Sprintf("Executed script %s in space %s", script.Name, space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       script.Id,
			"script_name":     script.Name,
			"space_id":        space.Id,
			"space_name":      space.Name,
		},
	)

	response := apiclient.ScriptExecuteResponse{
		Output: output,
	}
	if execErr != nil {
		response.Error = execErr.Error()
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleExecuteScriptContent(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	request := apiclient.ScriptContentExecuteRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if len(request.Content) == 0 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Script content is required"})
		return
	}

	if len(request.Content) > 4*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Script content exceeds 4MB limit"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageSpaces) && space.UserId != user.Id {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to access this space"})
		return
	}

	if !user.HasPermission(model.PermissionExecuteScripts) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute scripts"})
		return
	}

	var output string
	var execErr error

	session := agent_server.GetSession(space.Id)
	if session != nil {
		cfg := config.GetServerConfig()
		timeout := cfg.MaxScriptExecutionTime
		if timeout == 0 {
			timeout = 120
		}
		execMsg := &msg.ExecuteScriptMessage{
			Content:      request.Content,
			Arguments:    request.Arguments,
			Timeout:      timeout,
			IsSystemCall: false,
		}

		respChan, err := session.SendExecuteScript(execMsg)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("failed to send script to agent: %v", err)})
			return
		}

		resp := <-respChan
		if !resp.Success {
			output = resp.Output
			execErr = fmt.Errorf("%s", resp.Error)
		} else {
			output = resp.Output
		}
	} else {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Space is not running or agent not connected"})
		return
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptExecute,
		fmt.Sprintf("Executed script content in space %s", space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"space_id":        space.Id,
			"space_name":      space.Name,
		},
	)

	response := apiclient.ScriptExecuteResponse{
		Output: output,
	}
	if execErr != nil {
		response.Error = execErr.Error()
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleExecuteScriptByName(w http.ResponseWriter, r *http.Request) {
	spaceId := r.PathValue("space_id")

	if !validate.UUID(spaceId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid space ID"})
		return
	}

	request := apiclient.ScriptNameExecuteRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if !validate.VarName(request.ScriptName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	space, err := db.GetSpace(spaceId)
	if err != nil || space.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}

	if !user.HasPermission(model.PermissionManageSpaces) && space.UserId != user.Id {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to access this space"})
		return
	}

	if !user.HasPermission(model.PermissionExecuteScripts) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute scripts"})
		return
	}

	scripts, err := db.GetScripts()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	var script *model.Script
	for _, s := range scripts {
		if s.IsDeleted || !s.Active || s.Name != request.ScriptName {
			continue
		}

		if !user.HasPermission(model.PermissionManageScripts) {
			if len(s.Groups) > 0 && !user.HasAnyGroup(&s.Groups) {
				continue
			}
		}

		script = s
		break
	}

	if script == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	var output string
	var execErr error

	session := agent_server.GetSession(space.Id)
	if session != nil {
		timeout := script.Timeout
		if timeout == 0 {
			cfg := config.GetServerConfig()
			timeout = cfg.MaxScriptExecutionTime
		}
		execMsg := &msg.ExecuteScriptMessage{
			Content:      script.Content,
			Arguments:    request.Arguments,
			Timeout:      timeout,
			IsSystemCall: false,
		}

		respChan, err := session.SendExecuteScript(execMsg)
		if err != nil {
			rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("failed to send script to agent: %v", err)})
			return
		}

		resp := <-respChan
		if !resp.Success {
			output = resp.Output
			execErr = fmt.Errorf("%s", resp.Error)
		} else {
			output = resp.Output
		}
	} else {
		output, execErr = service.ExecuteScriptLocally(script, request.Arguments)
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventScriptExecute,
		fmt.Sprintf("Executed script %s in space %s", script.Name, space.Name),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"script_id":       script.Id,
			"script_name":     script.Name,
			"space_id":        space.Id,
			"space_name":      space.Name,
		},
	)

	response := apiclient.ScriptExecuteResponse{
		Output: output,
	}
	if execErr != nil {
		response.Error = execErr.Error()
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}
