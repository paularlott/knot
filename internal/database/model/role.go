package model

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

// Permissions
const (
	PermissionManageUsers               = iota // Can Manage Users
	PermissionManageTemplates                  // Can Manage Templates
	PermissionManageSpaces                     // Can Manage Spaces
	PermissionManageVolumes                    // Can Manage Volumes
	PermissionManageGroups                     // Can Manage Groups
	PermissionManageRoles                      // Can Manage Roles
	PermissionManageVariables                  // Can Manage Variables
	PermissionUseSpaces                        // Can Use Spaces
	PermissionUseTunnels                       // Can Use Tunnels
	PermissionViewAuditLogs                    // Can View Audit Logs
	PermissionTransferSpaces                   // Can Transfer Spaces
	PermissionShareSpaces                      // Can Share Spaces
	PermissionClusterInfo                      // Can View Cluster Info
	PermissionUseVNC                           // Can use VNC
	PermissionUseWebTerminal                   // Can use the web terminal
	PermissionUseSSH                           // Can use ssh connections
	PermissionUseCodeServer                    // Can use code-server
	PermissionUseVSCodeTunnel                  // Can use VSCode Tunnel
	PermissionUseLogs                          // Can use the log window
	PermissionRunCommands                      // Can run commands in spaces
	PermissionCopyFiles                        // Can copy files to/from spaces
	PermissionUseMCPServer                     // Can use MCP server
	PermissionUseWebAssistant                  // Can use web-based AI assistant
	PermissionManageScripts                    // Can Manage System/Global Scripts
	PermissionExecuteScripts                   // Can Execute System/Global Scripts
	PermissionManageOwnScripts                 // Can Manage Own Scripts
	PermissionExecuteOwnScripts                // Can Execute Own Scripts
	PermissionManageGlobalSkills               // Can Manage Global Skills
	PermissionManageOwnSkills                  // Can Manage Own Skills
	PermissionSetSpaceDependencies             // Can configure space dependencies in the UI
	PermissionUseSpaceStartupScript            // Can configure user startup script in the space form UI
	PermissionDownloadAuditLogs                // Can Download Audit Logs
	PermissionManageStackDefinitions           // Can create/edit/delete global (system) stack definitions
	PermissionManageOwnStackDefinitions        // Can create/edit/delete personal stack definitions
	PermissionUseStackDefinitions              // Can create instances from stack definitions
	PermissionUseMethods                       // Can use shared space methods
	PermissionUsePools                         // Can use space pools
	PermissionManageEvents                     // Can Manage Own Event Sinks
	PermissionManageGlobalEvents               // Can Manage Global Event Sinks
	PermissionManageGlobalSlashCommands        // Can Manage Global Slash Commands
	PermissionManageOwnSlashCommands           // Can Manage Own Slash Commands
	PermissionManageMCPServers                 // Can Manage MCP Servers
	// PermissionUseLogSinks is enforced only by Knot Pro (spaces registering
	// as log sinks); the constant exists in both editions so the permission
	// ids stay aligned, but Core never grants or checks it.
	PermissionUseLogSinks // Can register a space as a log sink receiving own space logs
)

type PermissionName struct {
	Id          int    `json:"id"`
	Group       string `json:"group"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

var PermissionNames = []PermissionName{
	{PermissionViewAuditLogs, "Audit", "View Audit Logs", "View the audit log of system activity."},
	{PermissionDownloadAuditLogs, "Audit", "Download Audit Logs", "Export audit log entries to a file."},

	{PermissionClusterInfo, "System", "View Cluster Info", "View cluster node and topology information."},

	{PermissionManageGroups, "User Management", "Manage Groups", "Create, edit, and delete user groups."},
	{PermissionManageRoles, "User Management", "Manage Roles", "Create, edit, and delete roles and their permissions."},
	{PermissionManageUsers, "User Management", "Manage Users", "Create, edit, and delete user accounts."},

	{PermissionManageSpaces, "Resource Management", "Manage Spaces", "Manage any space, including those owned by other users."},
	{PermissionManageTemplates, "Resource Management", "Manage Templates", "Create, edit, and delete space templates."},
	{PermissionManageVariables, "Resource Management", "Manage Variables", "Create, edit, and delete system variables."},
	{PermissionManageVolumes, "Resource Management", "Manage Volumes", "Create, edit, and delete volumes."},

	{PermissionUseMCPServer, "AI Tools", "Use MCP Server", "Connect to the knot MCP server."},
	{PermissionUseWebAssistant, "AI Tools", "Use Web Assistant", "Use the built-in web AI assistant."},

	{PermissionManageScripts, "Scripting", "Manage System Scripts", "Create and edit system (global) scripts."},
	{PermissionExecuteScripts, "Scripting", "Execute System Scripts", "Run system (global) scripts."},
	{PermissionManageOwnScripts, "Scripting", "Manage Own Scripts", "Create and edit your own scripts."},
	{PermissionExecuteOwnScripts, "Scripting", "Execute Own Scripts", "Run your own scripts."},

	{PermissionManageEvents, "Events", "Manage Own Event Sinks", "Create and manage your own event sinks."},
	{PermissionManageGlobalEvents, "Events", "Manage Global Event Sinks", "Create and manage global event sinks."},

	{PermissionManageGlobalSkills, "Skills", "Manage Global Skills", "Create and edit global skills."},
	{PermissionManageOwnSkills, "Skills", "Manage Own Skills", "Create and edit your own skills."},

	{PermissionManageGlobalSlashCommands, "Slash Commands", "Manage Global Slash Commands", "Create and edit global slash commands."},
	{PermissionManageOwnSlashCommands, "Slash Commands", "Manage Own Slash Commands", "Create and edit your own slash commands."},

	{PermissionManageMCPServers, "AI Tools", "Manage MCP Servers", "Register and configure MCP servers."},

	{PermissionManageStackDefinitions, "Stacks", "Manage Global Stack Definitions", "Create, edit, and delete global (system) stack definitions."},
	{PermissionManageOwnStackDefinitions, "Stacks", "Manage Own Stack Definitions", "Create, edit, and delete personal stack definitions."},
	{PermissionUseStackDefinitions, "Stacks", "Use Stack Definitions", "Create spaces from stack definitions."},

	{PermissionUseMethods, "Methods", "Use Space Methods Shared by Others", "Call space methods shared by other users."},

	{PermissionUseTunnels, "Public Tunnels", "Use Tunnels", "Expose a local or space port as a public URL via a knot tunnel."},

	{PermissionUseSpaces, "Space Operations", "Use Spaces", "Create and run spaces."},
	{PermissionUsePools, "Space Operations", "Use Space Pools", "Create and run space pools."},
	{PermissionSetSpaceDependencies, "Space Operations", "Set Space Dependencies", "Configure dependencies between spaces."},
	{PermissionUseSpaceStartupScript, "Space Operations", "Use User Startup Script", "Set a user startup script that runs when a space starts."},
	{PermissionShareSpaces, "Space Operations", "Share Spaces", "Share your spaces with other users."},
	{PermissionTransferSpaces, "Space Operations", "Transfer Spaces", "Transfer ownership of your spaces to another user."},
	{PermissionUseCodeServer, "Space Operations", "Use Code Server", "Open code-server in a space."},
	{PermissionUseLogs, "Space Operations", "View Logs", "View the log window for a space."},
	{PermissionUseSSH, "Space Operations", "Use SSH", "Connect to spaces over SSH."},
	{PermissionUseVNC, "Space Operations", "Use VNC", "Use the VNC graphical desktop in a space."},
	{PermissionUseVSCodeTunnel, "Space Operations", "Use VSCode Tunnel", "Connect to a space via a VS Code tunnel."},
	{PermissionUseWebTerminal, "Space Operations", "Use Web Terminal", "Use the web terminal in a space."},
	{PermissionRunCommands, "Space Operations", "Run Commands", "Execute commands inside a space."},
	{PermissionCopyFiles, "Space Operations", "Copy Files", "Copy files to and from a space."},
}

// Role
type Role struct {
	Id            string        `json:"role_id" db:"role_id,pk" msgpack:"role_id"`
	Name          string        `json:"name" db:"name" msgpack:"name"`
	Permissions   []uint16      `json:"permissions" db:"permissions,json" msgpack:"permissions"`
	IsDeleted     bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	CreatedUserId string        `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt     time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId string        `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt     hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

// Roles
const (
	RoleAdminUUID = "00000000-0000-0000-0000-000000000000"
)

var (
	roleCacheMutex = sync.RWMutex{}
	roleCache      = make(map[string]*Role)
)

func SetRoleCache(roles []*Role) {
	logger := log.WithGroup("server")
	logger.Info("loading roles to cache")

	// Create the admin role
	adminTime := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)
	roleCache[RoleAdminUUID] = &Role{
		Id:   RoleAdminUUID,
		Name: "Admin",
		Permissions: []uint16{
			PermissionManageUsers,
			PermissionManageTemplates,
			PermissionManageSpaces,
			PermissionManageVolumes,
			PermissionManageGroups,
			PermissionManageRoles,
			PermissionManageVariables,
			PermissionUseSpaces,
			PermissionSetSpaceDependencies,
			PermissionUseSpaceStartupScript,
			PermissionUseTunnels,
			PermissionViewAuditLogs,
			PermissionDownloadAuditLogs,
			PermissionTransferSpaces,
			PermissionShareSpaces,
			PermissionClusterInfo,
			PermissionUseVNC,
			PermissionUseWebTerminal,
			PermissionUseSSH,
			PermissionUseCodeServer,
			PermissionUseVSCodeTunnel,
			PermissionUseLogs,
			PermissionRunCommands,
			PermissionCopyFiles,
			PermissionUseMCPServer,
			PermissionUseWebAssistant,
			PermissionManageScripts,
			PermissionExecuteScripts,
			PermissionManageOwnScripts,
			PermissionExecuteOwnScripts,
			PermissionManageGlobalSkills,
			PermissionManageOwnSkills,
			PermissionManageGlobalSlashCommands,
			PermissionManageOwnSlashCommands,
			PermissionManageMCPServers,
			PermissionManageStackDefinitions,
			PermissionManageOwnStackDefinitions,
			PermissionUseStackDefinitions,
			PermissionUseMethods,
			PermissionUsePools,
			PermissionManageEvents,
			PermissionManageGlobalEvents,
		},
		CreatedAt: adminTime,
		UpdatedAt: hlc.Timestamp(0),
	}

	roleCacheMutex.Lock()
	defer roleCacheMutex.Unlock()

	for _, role := range roles {
		roleCache[role.Id] = role
	}
}

func GetRolesFromCache() []*Role {
	roleCacheMutex.RLock()
	defer roleCacheMutex.RUnlock()

	roles := make([]*Role, 0, len(roleCache))
	for _, role := range roleCache {
		roles = append(roles, role)
	}

	return roles
}

func DeleteRoleFromCache(roleId string) {
	roleCacheMutex.Lock()
	defer roleCacheMutex.Unlock()

	delete(roleCache, roleId)
}

func SaveRoleToCache(role *Role) {
	roleCacheMutex.Lock()
	defer roleCacheMutex.Unlock()

	roleCache[role.Id] = role
}

func NewRole(name string, permissions []uint16, userId string) *Role {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	role := &Role{
		Id:            id.String(),
		Name:          name,
		Permissions:   permissions,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     hlc.Now(),
	}

	return role
}

func RoleExists(roleId string) bool {
	_, ok := roleCache[roleId]
	return ok
}

// RoleName resolves a role id to its cached name, falling back to the id for
// unknown roles (e.g. audit entries emitted before the cache was warm).
func RoleName(roleId string) string {
	if role, ok := roleCache[roleId]; ok {
		return role.Name
	}
	return roleId
}

// RoleNames resolves role ids to cached names for audit properties.
func RoleNames(roleIds []string) []string {
	names := make([]string, 0, len(roleIds))
	for _, id := range roleIds {
		names = append(names, RoleName(id))
	}
	return names
}

// GetUserPermissions returns all permission integers for a user (resolves from roles)
func GetUserPermissions(user *User) []uint16 {
	permissions := make(map[uint16]bool)
	for _, role := range user.Roles {
		if r, ok := roleCache[role]; ok {
			for _, p := range r.Permissions {
				permissions[p] = true
			}
		}
	}
	result := make([]uint16, 0, len(permissions))
	for p := range permissions {
		result = append(result, p)
	}
	return result
}
