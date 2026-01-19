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

	// Get filter parameters
	filterUserId := r.URL.Query().Get("user_id")
	allZones := r.URL.Query().Get("all_zones") == "true"

	// Permission check - return empty list if not authorized (more robust than 403)
	if filterUserId != "" {
		// User scripts requested
		if filterUserId == user.Id {
			// Own scripts: need ManageOwnScripts permission
			if !user.HasPermission(model.PermissionManageOwnScripts) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
				return
			}
		} else {
			// Another user's scripts: need ManageScripts permission (admin only)
			if !user.HasPermission(model.PermissionManageScripts) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
				return
			}
		}
	} else {
		// No filter specified
		// If user has ManageScripts permission, show all scripts
		// If user only has ManageOwnScripts permission, show only their own scripts
		if user.HasPermission(model.PermissionManageScripts) {
			// Admin: show all scripts (filterUserId remains empty)
		} else if user.HasPermission(model.PermissionManageOwnScripts) {
			// Regular user: filter to own scripts
			filterUserId = user.Id
		} else {
			// No permission: return empty list
			rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
			return
		}
	}

	scriptService := service.GetScriptService()
	scripts, err := scriptService.ListScripts(service.ScriptListOptions{
		FilterUserId:         filterUserId,
		User:                 user,
		IncludeDeleted:       false,
		CheckZoneRestriction: !allZones,
	})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.ScriptList{
		Count:   0,
		Scripts: []apiclient.ScriptInfo{},
	}

	for _, script := range scripts {
		response.Scripts = append(response.Scripts, apiclient.ScriptInfo{
			Id:          script.Id,
			UserId:      script.UserId,
			Name:        script.Name,
			Description: script.Description,
			Groups:      script.Groups,
			Zones:       script.Zones,
			Active:      script.Active,
			ScriptType:  script.ScriptType,
			Timeout:     script.Timeout,
			IsManaged:   script.IsManaged,
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

	// Permission check
	if script.IsUserScript() {
		// User script: must be owner or have ManageScripts permission
		if script.UserId != user.Id && !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
			return
		}
		if script.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to view this script"})
			return
		}
	} else {
		// Global script
		if !user.HasPermission(model.PermissionManageScripts) {
			if len(script.Groups) > 0 && !user.HasAnyGroup(&script.Groups) {
				rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
				return
			}
		}
	}

	// Apply variable replacement to global scripts
	service.ApplyVariablesToScriptIfGlobal(script, db)

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptDetails{
		Id:                 script.Id,
		UserId:             script.UserId,
		Name:               script.Name,
		Description:        script.Description,
		Content:            script.Content,
		Groups:             script.Groups,
		Zones:              script.Zones,
		Active:             script.Active,
		ScriptType:         script.ScriptType,
		MCPInputSchemaToml: script.MCPInputSchemaToml,
		MCPKeywords:        script.MCPKeywords,
		Timeout:            script.Timeout,
		IsManaged:          script.IsManaged,
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

	// Determine if creating user script or global script based on request body
	ownerUserId := request.UserId
	if ownerUserId == "current" {
		ownerUserId = user.Id
	}
	isUserScript := ownerUserId != ""

	// Permission check
	if isUserScript {
		// Creating user script
		if ownerUserId != user.Id && !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create scripts for other users"})
			return
		}
		if ownerUserId == user.Id && !user.HasPermission(model.PermissionManageOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create own scripts"})
			return
		}
		// User scripts cannot have groups
		if len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User scripts cannot have groups"})
			return
		}
	} else {
		// Creating global script
		if !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create global scripts"})
			return
		}
	}

	script := model.NewScript(
		request.Name,
		request.Description,
		request.Content,
		request.Groups,
		request.Zones,
		request.Active,
		request.ScriptType,
		request.MCPInputSchemaToml,
		request.MCPKeywords,
		request.Timeout,
		ownerUserId,
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
			"is_user_script":  isUserScript,
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

	// Cannot edit managed scripts
	if script.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot edit managed script"})
		return
	}

	// Permission check
	if script.IsUserScript() {
		if script.UserId != user.Id && !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this script"})
			return
		}
		if script.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit own scripts"})
			return
		}
		if len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User scripts cannot have groups"})
			return
		}
	} else {
		if !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit global scripts"})
			return
		}
	}

	script.Name = request.Name
	script.Description = request.Description
	script.Content = request.Content
	script.Groups = request.Groups
	script.Zones = request.Zones
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
			"is_user_script":  script.IsUserScript(),
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

	// Cannot delete managed scripts
	if script.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot delete managed script"})
		return
	}

	// Permission check
	if script.IsUserScript() {
		if script.UserId != user.Id && !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this script"})
			return
		}
		if script.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete own scripts"})
			return
		}
	} else {
		if !user.HasPermission(model.PermissionManageScripts) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete global scripts"})
			return
		}
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
			"is_user_script":  script.IsUserScript(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleGetScriptDetailsByName(w http.ResponseWriter, r *http.Request) {
	scriptName := r.PathValue("script_name")
	if !validate.VarName(scriptName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	// Resolve script with user override
	script, err := service.ResolveScriptByName(scriptName, user.Id)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	// Permission check
	if !service.CanUserExecuteScript(user, script) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	// Apply variable replacement to global scripts
	service.ApplyVariablesToScriptIfGlobal(script, db)

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptDetails{
		Id:                 script.Id,
		UserId:             script.UserId,
		Name:               script.Name,
		Description:        script.Description,
		Content:            script.Content,
		Groups:             script.Groups,
		Zones:              script.Zones,
		Active:             script.Active,
		ScriptType:         script.ScriptType,
		MCPInputSchemaToml: script.MCPInputSchemaToml,
		MCPKeywords:        script.MCPKeywords,
		Timeout:            script.Timeout,
		IsManaged:          script.IsManaged,
	})
}

func HandleGetScriptByName(w http.ResponseWriter, r *http.Request) {
	scriptName := r.PathValue("script_name")
	if !validate.VarName(scriptName) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid script name"})
		return
	}

	scriptType := r.PathValue("script_type")
	user := r.Context().Value("user").(*model.User)

	// Resolve script with user override
	script, err := service.ResolveScriptByName(scriptName, user.Id)
	if err != nil || script.ScriptType != scriptType {
		rest.WriteResponse(http.StatusNotFound, w, r, "")
		return
	}

	// Permission check
	if !service.CanUserExecuteScript(user, script) {
		rest.WriteResponse(http.StatusNotFound, w, r, "")
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, script.Content)
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

	// Permission check
	if !service.CanUserExecuteScript(user, script) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute this script"})
		return
	}

	// Apply variable replacement to global scripts
	service.ApplyVariablesToScriptIfGlobal(script, db)

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
		// No agent connection - reject execution
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Space agent is not connected - cannot execute script"})
		return
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

	// Need execute permission (either global or own)
	if !user.HasPermission(model.PermissionExecuteScripts) && !user.HasPermission(model.PermissionExecuteOwnScripts) {
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

	// Resolve script with user override
	script, err := service.ResolveScriptByName(request.ScriptName, user.Id)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Script not found"})
		return
	}

	// Permission check
	if !service.CanUserExecuteScript(user, script) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to execute this script"})
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
		// No agent connection - reject execution
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Space agent is not connected - cannot execute script"})
		return
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
