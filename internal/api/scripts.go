package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
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
			// Own scripts: need ManageOwnScripts OR ExecuteOwnScripts permission
			if !user.HasPermission(model.PermissionManageOwnScripts) && !user.HasPermission(model.PermissionExecuteOwnScripts) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
				return
			}
		} else {
			// Another user's scripts: need ManageScripts OR ExecuteScripts permission (admin only)
			if !user.HasPermission(model.PermissionManageScripts) && !user.HasPermission(model.PermissionExecuteScripts) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
				return
			}
		}
	} else {
		// No filter specified
		// Users with ManageScripts OR ExecuteScripts can see global scripts
		// Users with ManageOwnScripts OR ExecuteOwnScripts can see their own scripts
		canSeeGlobals := user.HasPermission(model.PermissionManageScripts) || user.HasPermission(model.PermissionExecuteScripts)
		canSeeOwn := user.HasPermission(model.PermissionManageOwnScripts) || user.HasPermission(model.PermissionExecuteOwnScripts)

		if !canSeeGlobals && !canSeeOwn {
			// No permission: return empty list
			rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
			return
		}

		// If user can see both, fetch global and own scripts
		// If user can only see own scripts, filter to own
		// If user can only see global scripts, leave filter empty (returns only global)
		if canSeeOwn && !canSeeGlobals {
			filterUserId = user.Id
		}
		// If canSeeGlobals is true, filterUserId remains "" to get global scripts
		// We'll fetch own scripts separately if needed
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

	// Track seen script IDs to avoid duplicates
	seenScripts := make(map[string]bool)

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
			IsManaged:   script.IsManaged,
		})
		seenScripts[script.Id] = true
		response.Count++
	}

	// If user can see both global and own scripts, and we only fetched globals, also fetch own scripts
	if filterUserId == "" && (user.HasPermission(model.PermissionManageOwnScripts) || user.HasPermission(model.PermissionExecuteOwnScripts)) {
		ownScripts, err := scriptService.ListScripts(service.ScriptListOptions{
			FilterUserId:         user.Id,
			User:                 user,
			IncludeDeleted:       false,
			CheckZoneRestriction: !allZones,
		})
		if err == nil {
			for _, script := range ownScripts {
				if !seenScripts[script.Id] {
					response.Scripts = append(response.Scripts, apiclient.ScriptInfo{
						Id:          script.Id,
						UserId:      script.UserId,
						Name:        script.Name,
						Description: script.Description,
						Groups:      script.Groups,
						Zones:       script.Zones,
						Active:      script.Active,
						ScriptType:  script.ScriptType,
						IsManaged:   script.IsManaged,
					})
					seenScripts[script.Id] = true
					response.Count++
				}
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

// HandleGetGlobalScripts returns global scripts for template editing.
// Users with PermissionManageTemplates can see global scripts even without PermissionManageScripts.
func HandleGetGlobalScripts(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	// Permission check - users can view global scripts if they have ManageTemplates permission
	if !user.HasPermission(model.PermissionManageTemplates) {
		rest.WriteResponse(http.StatusOK, w, r, apiclient.ScriptList{Count: 0, Scripts: []apiclient.ScriptInfo{}})
		return
	}

	allZones := r.URL.Query().Get("all_zones") == "true"

	scriptService := service.GetScriptService()
	scripts, err := scriptService.ListScripts(service.ScriptListOptions{
		FilterUserId:         "", // Empty string to get only global scripts
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
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	// Determine if creating user script or global script based on request body
	ownerUserId := request.UserId
	if ownerUserId == "current" {
		ownerUserId = user.Id
	}
	isUserScript := ownerUserId != ""

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
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
	} else {
		// In leaf mode, user scripts cannot have groups
		if isUserScript && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User scripts cannot have groups"})
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
	cfg := config.GetServerConfig()
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

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
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
	} else {
		// In leaf mode, user scripts cannot have groups
		if script.IsUserScript() && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User scripts cannot have groups"})
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
	cfg := config.GetServerConfig()
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

	// Permission check (bypass in leaf mode)
	if !cfg.LeafNode {
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
