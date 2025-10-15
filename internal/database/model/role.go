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
	PermissionManageUsers     = iota // Can Manage Users
	PermissionManageTemplates        // Can Manage Templates
	PermissionManageSpaces           // Can Manage Spaces
	PermissionManageVolumes          // Can Manage Volumes
	PermissionManageGroups           // Can Manage Groups
	PermissionManageRoles            // Can Manage Roles
	PermissionManageVariables        // Can Manage Variables
	PermissionUseSpaces              // Can Use Spaces
	PermissionUseTunnels             // Can Use Tunnels
	PermissionViewAuditLogs          // Can View Audit Logs
	PermissionTransferSpaces         // Can Transfer Spaces
	PermissionShareSpaces            // Can Share Spaces
	PermissionClusterInfo            // Can View Cluster Info
	PermissionUseVNC                 // Can use VNC
	PermissionUseWebTerminal         // Can use the web terminal
	PermissionUseSSH                 // Can use ssh connections
	PermissionUseCodeServer          // Can use code-server
	PermissionUseVSCodeTunnel        // Can use VSCode Tunnel
	PermissionUseLogs                // Can use the log window
	PermissionRunCommands            // Can run commands in spaces
	PermissionCopyFiles              // Can copy files to/from spaces
	PermissionUseMCPServer           // Can use MCP server
	PermissionUseWebAssistant        // Can use web-based AI assistant
)

type PermissionName struct {
	Id    int    `json:"id"`
	Group string `json:"group"`
	Name  string `json:"name"`
}

var PermissionNames = []PermissionName{
	{PermissionViewAuditLogs, "Audit", "View Audit Logs"},

	{PermissionClusterInfo, "System", "View Cluster Info"},

	{PermissionManageGroups, "User Management", "Manage Groups"},
	{PermissionManageRoles, "User Management", "Manage Roles"},
	{PermissionManageUsers, "User Management", "Manage Users"},

	{PermissionManageSpaces, "Resource Management", "Manage Spaces"},
	{PermissionManageTemplates, "Resource Management", "Manage Templates"},
	{PermissionManageVariables, "Resource Management", "Manage Variables"},
	{PermissionManageVolumes, "Resource Management", "Manage Volumes"},

	{PermissionUseMCPServer, "AI Tools", "Use MCP Server"},
	{PermissionUseWebAssistant, "AI Tools", "Use Web Assistant"},

	{PermissionUseSpaces, "Space Operations", "Use Spaces"},
	{PermissionShareSpaces, "Space Operations", "Share Spaces"},
	{PermissionTransferSpaces, "Space Operations", "Transfer Spaces"},
	{PermissionUseTunnels, "Space Operations", "Use Tunnels"},
	{PermissionUseCodeServer, "Space Operations", "Use Code Server"},
	{PermissionUseLogs, "Space Operations", "View Logs"},
	{PermissionUseSSH, "Space Operations", "Use SSH"},
	{PermissionUseVNC, "Space Operations", "Use VNC"},
	{PermissionUseVSCodeTunnel, "Space Operations", "Use VSCode Tunnel"},
	{PermissionUseWebTerminal, "Space Operations", "Use Web Terminal"},
	{PermissionRunCommands, "Space Operations", "Run Commands"},
	{PermissionCopyFiles, "Space Operations", "Copy Files"},
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
	log.Info("server: loading roles to cache")

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
			PermissionUseTunnels,
			PermissionViewAuditLogs,
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
