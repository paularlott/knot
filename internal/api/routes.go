package api

import (
	"net/http"

	"github.com/paularlott/knot/internal/middleware"
)

func ApiRoutes(router *http.ServeMux) {

	// Core
	router.HandleFunc("GET /api/lookup/{service}", middleware.ApiAuth(HandleLookup))
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

	// Groups
	router.HandleFunc("GET /api/groups", middleware.ApiAuth(HandleGetGroups))
	router.HandleFunc("POST /api/groups", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleCreateGroup)))
	router.HandleFunc("PUT /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleUpdateGroup)))
	router.HandleFunc("DELETE /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleDeleteGroup)))
	router.HandleFunc("GET /api/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleGetGroup)))

	// Permissions
	router.HandleFunc("GET /api/permissions", middleware.ApiAuth(HandleGetPermissions))

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
	router.HandleFunc("DELETE /api/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleDeleteSpace)))
	router.HandleFunc("GET /api/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpace)))
	router.HandleFunc("POST /api/spaces/{space_id}/start", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStart)))
	router.HandleFunc("POST /api/spaces/{space_id}/stop", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStop)))
	router.HandleFunc("POST /api/spaces/{user_id}/stop-for-user", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStopUsersSpaces)))
	router.HandleFunc("POST /api/spaces/{space_id}/transfer", middleware.ApiAuth(middleware.ApiPermissionTransferSpaces(HandleSpaceTransfer)))
	router.HandleFunc("POST /api/spaces/{space_id}/share", middleware.ApiAuth(middleware.ApiPermissionTransferSpaces(HandleSpaceAddShare)))
	router.HandleFunc("DELETE /api/spaces/{space_id}/share", middleware.ApiAuth(middleware.ApiPermissionTransferSpaces(HandleSpaceRemoveShare)))

	// Templates
	router.HandleFunc("GET /api/templates", middleware.ApiAuth(HandleGetTemplates))
	router.HandleFunc("GET /api/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleGetTemplate)))
	router.HandleFunc("POST /api/templates", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleCreateTemplate)))
	router.HandleFunc("PUT /api/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleUpdateTemplate)))
	router.HandleFunc("DELETE /api/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleDeleteTemplate)))

	// Volumes
	router.HandleFunc("GET /api/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolumes)))
	router.HandleFunc("POST /api/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleCreateVolume)))
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

	// Tunnels
	router.HandleFunc("GET /api/tunnels", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnels)))
	router.HandleFunc("GET /api/tunnels/domain", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnelDomain)))
	router.HandleFunc("DELETE /api/tunnels/{tunnel_name}", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleDeleteTunnel)))

	// Audit Logs
	router.HandleFunc("GET /api/audit-logs", middleware.ApiAuth(middleware.ApiPermissionViewAuditLogs(HandleGetAuditLogs)))

	// Cluster Information
	router.HandleFunc("GET /api/cluster-info", middleware.ApiAuth(middleware.ApiPermissionViewClusterInfo(HandleGetClusterInfo)))

	// Unauthenticated routes
	router.HandleFunc("POST /api/auth", HandleAuthorization)
	router.HandleFunc("POST /api/auth/web", HandleAuthorization)
	router.HandleFunc("GET /api/auth/using-totp", HandleUsingTotp)
}
