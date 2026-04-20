package api

import (
	"context"
	"net/http"

	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/oauth2"
)

func ApiRoutes(router *http.ServeMux) {
	// Core
	router.HandleFunc("GET /api/ping", middleware.ApiAuth(HandlePing))
	router.HandleFunc("POST /api/auth/logout", middleware.ApiAuth(HandleLogout))

	// Users
	router.HandleFunc("GET /api/users", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSpaces(HandleGetUsers)))
	router.HandleFunc("POST /api/users", middleware.ApiAuth(middleware.ApiPermissionManageUsers(HandleCreateUser)))
	router.HandleFunc("GET /api/users/whoami", middleware.ApiAuth(HandleWhoAmI))
	router.HandleFunc("GET /api/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUser)))
	router.HandleFunc("PUT /api/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleUpdateUser)))
	router.HandleFunc("DELETE /api/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleDeleteUser)))
	router.HandleFunc("GET /api/users/{user_id}/quota", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUserQuota)))
	router.HandleFunc("GET /api/users/{user_id}/permissions", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUserPermissions)))
	router.HandleFunc("GET /api/users/{user_id}/has-permission", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUserHasPermission)))

	// Groups
	router.HandleFunc("GET /api/groups", middleware.ApiAuth(HandleGetGroups))
	router.HandleFunc("POST /api/groups", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleCreateGroup)))
	router.HandleFunc("PUT /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleUpdateGroup)))
	router.HandleFunc("DELETE /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleDeleteGroup)))
	router.HandleFunc("GET /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleGetGroup)))

	// Permissions
	router.HandleFunc("GET /api/permissions", middleware.ApiAuth(HandleGetPermissions))

	// Icons
	router.HandleFunc("GET /api/icons", middleware.ApiAuth(HandleGetIcons))

	// Roles
	router.HandleFunc("GET /api/roles", middleware.ApiAuth(HandleGetRoles))
	router.HandleFunc("POST /api/roles", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleCreateRole)))
	router.HandleFunc("PUT /api/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleUpdateRole)))
	router.HandleFunc("DELETE /api/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleDeleteRole)))
	router.HandleFunc("GET /api/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleGetRole)))

	// Sessions
	router.HandleFunc("GET /api/sessions", middleware.ApiAuth(HandleGetSessions))
	router.HandleFunc("DELETE /api/sessions/{session_id}", middleware.ApiAuth(HandleDeleteSessions))

	// Tokens
	router.HandleFunc("GET /api/tokens", middleware.ApiAuth(HandleGetTokens))
	router.HandleFunc("POST /api/tokens", middleware.ApiAuth(HandleCreateToken))
	router.HandleFunc("DELETE /api/tokens/{token_id}", middleware.ApiAuth(HandleDeleteToken))

	// Spaces
	router.HandleFunc("GET /api/spaces", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpaces)))
	router.HandleFunc("POST /api/spaces", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleCreateSpace)))
	router.HandleFunc("PUT /api/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleUpdateSpace)))
	router.HandleFunc("PUT /api/spaces/{space_id}/custom-field", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSetSpaceCustomField)))
	router.HandleFunc("GET /api/spaces/{space_id}/custom-field/{field_name}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpaceCustomField)))
	router.HandleFunc("DELETE /api/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleDeleteSpace)))
	router.HandleFunc("GET /api/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpace)))
	router.HandleFunc("POST /api/spaces/{space_id}/start", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStart)))
	router.HandleFunc("POST /api/spaces/{space_id}/stop", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStop)))
	router.HandleFunc("POST /api/spaces/{space_id}/restart", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceRestart)))
	router.HandleFunc("POST /api/spaces/{user_id}/stop-for-user", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStopUsersSpaces)))
	router.HandleFunc("POST /api/spaces/{space_id}/transfer", middleware.ApiAuth(middleware.ApiPermissionTransferSpaces(HandleSpaceTransfer)))
	router.HandleFunc("POST /api/spaces/{space_id}/share", middleware.ApiAuth(middleware.ApiPermissionTransferSpaces(HandleSpaceAddShare)))
	router.HandleFunc("DELETE /api/spaces/{space_id}/share", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceRemoveShare)))
	router.HandleFunc("POST /api/spaces/stacks/{stack_name}/start", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleStackStart)))
	router.HandleFunc("POST /api/spaces/stacks/{stack_name}/stop", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleStackStop)))
	router.HandleFunc("POST /api/spaces/stacks/{stack_name}/restart", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleStackRestart)))
	router.HandleFunc("POST /api/spaces/{space_id}/files/read", middleware.ApiAuth(middleware.ApiPermissionCopyFiles(HandleReadSpaceFile)))
	router.HandleFunc("POST /api/spaces/{space_id}/files/write", middleware.ApiAuth(middleware.ApiPermissionCopyFiles(HandleWriteSpaceFile)))
	router.HandleFunc("POST /api/spaces/{space_id}/run-command", middleware.ApiAuth(middleware.ApiPermissionRunCommands(HandleRunCommand)))

	// Templates
	router.HandleFunc("GET /api/templates", middleware.ApiAuth(HandleGetTemplates))
	router.HandleFunc("GET /api/templates/{template_id}", middleware.ApiAuth(HandleGetTemplate))
	router.HandleFunc("GET /api/templates/{template_id}/nodes", middleware.ApiAuth(HandleGetTemplateNodes))
	router.HandleFunc("POST /api/templates/validate", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleValidateTemplate)))
	router.HandleFunc("POST /api/templates", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleCreateTemplate)))
	router.HandleFunc("PUT /api/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleUpdateTemplate)))
	router.HandleFunc("DELETE /api/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleDeleteTemplate)))

	// Volumes
	router.HandleFunc("GET /api/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolumes)))
	router.HandleFunc("POST /api/volumes/validate", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleValidateVolume)))
	router.HandleFunc("POST /api/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleCreateVolume)))
	router.HandleFunc("GET /api/volumes/nodes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolumeNodes)))
	router.HandleFunc("PUT /api/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleUpdateVolume)))
	router.HandleFunc("DELETE /api/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleDeleteVolume)))
	router.HandleFunc("GET /api/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolume)))
	router.HandleFunc("POST /api/volumes/{volume_id}/start", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleVolumeStart)))
	router.HandleFunc("POST /api/volumes/{volume_id}/stop", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleVolumeStop)))

	// Template Variables
	router.HandleFunc("GET /api/templatevars", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleGetTemplateVars)))
	router.HandleFunc("POST /api/templatevars", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleCreateTemplateVar)))
	router.HandleFunc("PUT /api/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleUpdateTemplateVar)))
	router.HandleFunc("DELETE /api/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleDeleteTemplateVar)))
	router.HandleFunc("GET /api/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleGetTemplateVar)))

	// Scripts
	router.HandleFunc("GET /api/scripts", middleware.ApiAuth(HandleGetScripts))
	router.HandleFunc("GET /api/scripts/global", middleware.ApiAuth(HandleGetGlobalScripts))
	router.HandleFunc("GET /api/scripts/{script_id}", middleware.ApiAuth(HandleGetScript))
	router.HandleFunc("GET /api/scripts/name/{script_name}", middleware.ApiAuth(HandleGetScriptDetailsByName))
	router.HandleFunc("GET /api/scripts/name/{script_name}/{script_type}", middleware.ApiAuth(HandleGetScriptByName))
	router.HandleFunc("POST /api/scripts", middleware.ApiAuth(middleware.ApiPermissionManageScripts(HandleCreateScript)))
	router.HandleFunc("PUT /api/scripts/{script_id}", middleware.ApiAuth(middleware.ApiPermissionManageScripts(HandleUpdateScript)))
	router.HandleFunc("DELETE /api/scripts/{script_id}", middleware.ApiAuth(middleware.ApiPermissionManageScripts(HandleDeleteScript)))
	router.HandleFunc("POST /api/spaces/{space_id}/execute-script", middleware.ApiAuth(HandleExecuteScript))
	router.HandleFunc("GET /api/spaces/{space_id}/execute-script-stream", middleware.ApiAuth(HandleExecuteScriptStream))

	// Skills
	router.HandleFunc("GET /api/skill", middleware.ApiAuth(HandleGetSkills))
	router.HandleFunc("GET /api/skill/search", middleware.ApiAuth(HandleSearchSkills))
	router.HandleFunc("GET /api/skill/{skill_id}", middleware.ApiAuth(HandleGetSkill))
	router.HandleFunc("POST /api/skill", middleware.ApiAuth(HandleCreateSkill))
	router.HandleFunc("PUT /api/skill/{skill_id}", middleware.ApiAuth(HandleUpdateSkill))
	router.HandleFunc("DELETE /api/skill/{skill_id}", middleware.ApiAuth(HandleDeleteSkill))

	// Stack Definitions
	router.HandleFunc("GET /api/stack-definitions", middleware.ApiAuth(HandleGetStackDefinitions))
	router.HandleFunc("GET /api/stack-definitions/{stack_definition_id}", middleware.ApiAuth(HandleGetStackDefinition))
	router.HandleFunc("POST /api/stack-definitions/validate", middleware.ApiAuth(HandleValidateStackDefinition))
	router.HandleFunc("POST /api/stack-definitions", middleware.ApiAuth(HandleCreateStackDefinition))
	router.HandleFunc("PUT /api/stack-definitions/{stack_definition_id}", middleware.ApiAuth(HandleUpdateStackDefinition))
	router.HandleFunc("DELETE /api/stack-definitions/{stack_definition_id}", middleware.ApiAuth(HandleDeleteStackDefinition))

	// Tunnels
	router.HandleFunc("GET /api/tunnels", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnels)))
	router.HandleFunc("GET /api/tunnels/server-info", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnelServerInfo)))
	router.HandleFunc("DELETE /api/tunnels/{tunnel_name}", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleDeleteTunnel)))

	// Audit Logs
	router.HandleFunc("GET /api/audit-logs", middleware.ApiAuth(middleware.ApiPermissionViewAuditLogs(HandleGetAuditLogs)))
	router.HandleFunc("GET /api/audit-logs/export", middleware.ApiAuth(middleware.ApiPermissionDownloadAuditLogs(HandleExportAuditLogs)))

	// Cluster Information
	router.HandleFunc("GET /api/cluster-info", middleware.ApiAuth(middleware.ApiPermissionViewClusterInfo(HandleGetClusterInfo)))
	router.HandleFunc("GET /api/cluster/node", HandleGetClusterNode)

	// Server-Sent Events for real-time updates
	router.HandleFunc("GET /api/events", HandleSSE)

	// Unauthenticated routes
	router.HandleFunc("POST /api/auth", HandleAuthorization)
	router.HandleFunc("POST /api/auth/web", HandleAuthorization)
	router.HandleFunc("GET /api/auth/using-totp", HandleUsingTotp)

	// OAuth2 routes
	router.HandleFunc("GET /authorize", middleware.WebAuth(oauth2.HandleAuthorize))
	router.HandleFunc("POST /token", oauth2.HandleToken)

	// OAuth2 Discovery
	router.HandleFunc("GET /.well-known/oauth-authorization-server", oauth2.HandleAuthorizationServerMetadata)

	// Start a cleanup job for the rate limiters
	go cleanupLimiters(context.Background())
}
