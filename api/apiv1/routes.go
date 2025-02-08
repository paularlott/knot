package apiv1

import (
	"net/http"

	"github.com/paularlott/knot/internal/origin_leaf"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/middleware"
)

func ApiRoutes(router *http.ServeMux) {

	// Core
	router.HandleFunc("GET /api/v1/lookup/{service}", middleware.ApiAuth(HandleLookup))
	router.HandleFunc("GET /api/v1/ping", middleware.ApiAuth(HandlePing))
	router.HandleFunc("POST /api/v1/auth/logout", middleware.ApiAuth(HandleLogout))

	// Users
	router.HandleFunc("GET /api/v1/users", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSpaces(HandleGetUsers)))
	router.HandleFunc("POST /api/v1/users", middleware.ApiAuth(middleware.ApiPermissionManageUsers(HandleCreateUser)))
	router.HandleFunc("GET /api/v1/users/whoami", middleware.ApiAuth(HandleWhoAmI))
	router.HandleFunc("GET /api/v1/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUser)))
	router.HandleFunc("PUT /api/v1/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleUpdateUser)))
	router.HandleFunc("DELETE /api/v1/users/{user_id}", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleDeleteUser)))
	router.HandleFunc("GET /api/v1/users/{user_id}/quota", middleware.ApiAuth(middleware.ApiPermissionManageUsersOrSelf(HandleGetUserQuota)))

	// Groups
	router.HandleFunc("GET /api/v1/groups", middleware.ApiAuth(HandleGetGroups))
	router.HandleFunc("POST /api/v1/groups", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleCreateGroup)))
	router.HandleFunc("PUT /api/v1/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleUpdateGroup)))
	router.HandleFunc("DELETE /api/v1/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleDeleteGroup)))
	router.HandleFunc("GET /api/v1/groups/{group_id}", middleware.ApiAuth(middleware.ApiPermissionManageGroups(HandleGetGroup)))

	// Permissions
	router.HandleFunc("GET /api/v1/permissions", middleware.ApiAuth(HandleGetPermissions))

	// Roles
	router.HandleFunc("GET /api/v1/roles", middleware.ApiAuth(HandleGetRoles))
	router.HandleFunc("POST /api/v1/roles", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleCreateRole)))
	router.HandleFunc("PUT /api/v1/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleUpdateRole)))
	router.HandleFunc("DELETE /api/v1/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleDeleteRole)))
	router.HandleFunc("GET /api/v1/roles/{role_id}", middleware.ApiAuth(middleware.ApiPermissionManageRoles(HandleGetRole)))

	// Sessions
	router.HandleFunc("GET /api/v1/sessions", middleware.ApiAuth(HandleGetSessions))
	router.HandleFunc("DELETE /api/v1/sessions/{session_id}", middleware.ApiAuth(HandleDeleteSessions))

	// Tokens
	router.HandleFunc("GET /api/v1/tokens", middleware.ApiAuth(HandleGetTokens))
	router.HandleFunc("POST /api/v1/tokens", middleware.ApiAuth(HandleCreateToken))
	router.HandleFunc("DELETE /api/v1/tokens/{token_id}", middleware.ApiAuth(HandleDeleteToken))

	// Spaces
	router.HandleFunc("GET /api/v1/spaces", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpaces)))
	router.HandleFunc("POST /api/v1/spaces", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleCreateSpace)))
	router.HandleFunc("PUT /api/v1/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleUpdateSpace)))
	router.HandleFunc("DELETE /api/v1/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleDeleteSpace)))
	router.HandleFunc("GET /api/v1/spaces/{space_id}", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpace)))
	router.HandleFunc("GET /api/v1/spaces/{space_id}/service-state", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleGetSpaceServiceState)))
	router.HandleFunc("POST /api/v1/spaces/{space_id}/start", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStart)))
	router.HandleFunc("POST /api/v1/spaces/{space_id}/stop", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStop)))
	router.HandleFunc("POST /api/v1/spaces/{user_id}/stop-for-user", middleware.ApiAuth(middleware.ApiPermissionUseSpaces(HandleSpaceStopUsersSpaces)))

	// Templates
	router.HandleFunc("GET /api/v1/templates", middleware.ApiAuth(HandleGetTemplates))
	router.HandleFunc("GET /api/v1/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleGetTemplate)))
	router.HandleFunc("POST /api/v1/templates", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleCreateTemplate)))
	router.HandleFunc("PUT /api/v1/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleUpdateTemplate)))
	router.HandleFunc("DELETE /api/v1/templates/{template_id}", middleware.ApiAuth(middleware.ApiPermissionManageTemplates(HandleDeleteTemplate)))

	// Volumes
	router.HandleFunc("GET /api/v1/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolumes)))
	router.HandleFunc("POST /api/v1/volumes", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleCreateVolume)))
	router.HandleFunc("PUT /api/v1/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleUpdateVolume)))
	router.HandleFunc("DELETE /api/v1/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleDeleteVolume)))
	router.HandleFunc("GET /api/v1/volumes/{volume_id}", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleGetVolume)))
	router.HandleFunc("POST /api/v1/volumes/{volume_id}/start", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleVolumeStart)))
	router.HandleFunc("POST /api/v1/volumes/{volume_id}/stop", middleware.ApiAuth(middleware.ApiPermissionManageVolumes(HandleVolumeStop)))

	// Template Variables
	router.HandleFunc("GET /api/v1/templatevars", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleGetTemplateVars)))
	router.HandleFunc("POST /api/v1/templatevars", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleCreateTemplateVar)))
	router.HandleFunc("PUT /api/v1/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleUpdateTemplateVar)))
	router.HandleFunc("DELETE /api/v1/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleDeleteTemplateVar)))
	router.HandleFunc("GET /api/v1/templatevars/{templatevar_id}", middleware.ApiAuth(middleware.ApiPermissionManageVariables(HandleGetTemplateVar)))

	// Tunnels
	router.HandleFunc("GET /api/v1/tunnels", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnels)))
	router.HandleFunc("GET /api/v1/tunnels/domain", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleGetTunnelDomain)))
	router.HandleFunc("DELETE /api/v1/tunnels/{tunnel_name}", middleware.ApiAuth(middleware.ApiPermissionUseTunnels(HandleDeleteTunnel)))

	// Audit Logs
	router.HandleFunc("GET /api/v1/audit-logs", middleware.ApiAuth(middleware.ApiPermissionViewAuditLogs(HandleGetAuditLogs)))

	// Unauthenticated routes
	router.HandleFunc("POST /api/v1/auth", HandleAuthorization)
	router.HandleFunc("POST /api/v1/auth/web", HandleAuthorization)
	router.HandleFunc("GET /api/v1/auth/using-totp", HandleUsingTotp)

	// Additional endpoints exposed by origin servers
	if server_info.IsOrigin {
		// Remote server authenticated routes
		router.HandleFunc("GET /api/v1/leaf-server", middleware.LeafServerAuth(origin_leaf.OriginListenAndServe))
	}
}
