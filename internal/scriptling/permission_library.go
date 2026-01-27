package scriptling

import (
	"context"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling/object"
)

// GetPermissionLibrary returns the permission constants library for scriptling
func GetPermissionLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.permission", "Knot permission constants")

	// User Management
	builder.Constant("MANAGE_USERS", int64(model.PermissionManageUsers))
	builder.Constant("MANAGE_GROUPS", int64(model.PermissionManageGroups))
	builder.Constant("MANAGE_ROLES", int64(model.PermissionManageRoles))

	// Resource Management
	builder.Constant("MANAGE_SPACES", int64(model.PermissionManageSpaces))
	builder.Constant("MANAGE_TEMPLATES", int64(model.PermissionManageTemplates))
	builder.Constant("MANAGE_VOLUMES", int64(model.PermissionManageVolumes))
	builder.Constant("MANAGE_VARIABLES", int64(model.PermissionManageVariables))

	// Space Operations
	builder.Constant("USE_SPACES", int64(model.PermissionUseSpaces))
	builder.Constant("TRANSFER_SPACES", int64(model.PermissionTransferSpaces))
	builder.Constant("SHARE_SPACES", int64(model.PermissionShareSpaces))
	builder.Constant("USE_TUNNELS", int64(model.PermissionUseTunnels))

	// System & Audit
	builder.Constant("VIEW_AUDIT_LOGS", int64(model.PermissionViewAuditLogs))
	builder.Constant("CLUSTER_INFO", int64(model.PermissionClusterInfo))

	// Space Features
	builder.Constant("USE_VNC", int64(model.PermissionUseVNC))
	builder.Constant("USE_WEB_TERMINAL", int64(model.PermissionUseWebTerminal))
	builder.Constant("USE_SSH", int64(model.PermissionUseSSH))
	builder.Constant("USE_CODE_SERVER", int64(model.PermissionUseCodeServer))
	builder.Constant("USE_VSCODE_TUNNEL", int64(model.PermissionUseVSCodeTunnel))
	builder.Constant("USE_LOGS", int64(model.PermissionUseLogs))
	builder.Constant("RUN_COMMANDS", int64(model.PermissionRunCommands))
	builder.Constant("COPY_FILES", int64(model.PermissionCopyFiles))

	// AI Tools
	builder.Constant("USE_MCP_SERVER", int64(model.PermissionUseMCPServer))
	builder.Constant("USE_WEB_ASSISTANT", int64(model.PermissionUseWebAssistant))

	// Scripting
	builder.Constant("MANAGE_SCRIPTS", int64(model.PermissionManageScripts))
	builder.Constant("EXECUTE_SCRIPTS", int64(model.PermissionExecuteScripts))
	builder.Constant("MANAGE_OWN_SCRIPTS", int64(model.PermissionManageOwnScripts))
	builder.Constant("EXECUTE_OWN_SCRIPTS", int64(model.PermissionExecuteOwnScripts))

	// Skills
	builder.Constant("MANAGE_GLOBAL_SKILLS", int64(model.PermissionManageGlobalSkills))
	builder.Constant("MANAGE_OWN_SKILLS", int64(model.PermissionManageOwnSkills))

	// Aliases for convenience
	builder.Constant("SPACE_MANAGE", int64(model.PermissionManageSpaces))
	builder.Constant("SPACE_USE", int64(model.PermissionUseSpaces))
	builder.Constant("SCRIPT_MANAGE", int64(model.PermissionManageScripts))
	builder.Constant("SCRIPT_EXECUTE", int64(model.PermissionExecuteScripts))

	// list() function - returns all permissions with details
	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return permissionList(ctx, client)
	}, "list() - List all permissions with their IDs, names, and groups")

	return builder.Build()
}

// permissionList returns all permissions with their details
func permissionList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Permissions not available - API client not configured"}
	}

	// Call API to get permissions
	var response struct {
		Count       int `json:"count"`
		Permissions []struct {
			Id    int    `json:"id"`
			Name  string `json:"name"`
			Group string `json:"group"`
		} `json:"permissions"`
	}

	_, err := client.Do(ctx, "GET", "api/permissions", nil, &response)
	if err != nil {
		return &object.Error{Message: "Failed to list permissions: %v"}
	}

	// Convert to scriptling list of dicts
	permList := make([]object.Object, 0, len(response.Permissions))
	for _, p := range response.Permissions {
		permDict := &object.Dict{
			Pairs: map[string]object.DictPair{
				"id": {
					Key:   &object.String{Value: "id"},
					Value: &object.Integer{Value: int64(p.Id)},
				},
				"name": {
					Key:   &object.String{Value: "name"},
					Value: &object.String{Value: p.Name},
				},
				"group": {
					Key:   &object.String{Value: "group"},
					Value: &object.String{Value: p.Group},
				},
			},
		}
		permList = append(permList, permDict)
	}

	return &object.List{Elements: permList}
}
