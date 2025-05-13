package model

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
)

type PermissionName struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

var PermissionNames = []PermissionName{
	{PermissionUseTunnels, "Can Use Tunnels"},
	{PermissionManageSpaces, "Manage Spaces"},
	{PermissionUseSpaces, "Can Use Spaces"},
	{PermissionTransferSpaces, "Can Transfer Spaces"},
	{PermissionShareSpaces, "Can Share Spaces"},
	{PermissionManageTemplates, "Manage Templates"},
	{PermissionManageVariables, "Manage Variables"},
	{PermissionManageVolumes, "Manage Volumes"},
	{PermissionManageGroups, "Manage Groups"},
	{PermissionManageRoles, "Manage Roles"},
	{PermissionManageUsers, "Manage Users"},
	{PermissionViewAuditLogs, "View Audit Logs"},
	{PermissionClusterInfo, "View Cluster Info"},
}

// Role
type Role struct {
	Id            string    `json:"role_id" db:"role_id,pk" msgpack:"role_id"`
	Name          string    `json:"name" db:"name" msgpack:"name"`
	Permissions   []uint16  `json:"permissions" db:"permissions,json" msgpack:"permissions"`
	IsDeleted     bool      `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	CreatedUserId string    `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId string    `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt     time.Time `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
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
	log.Info().Msg("server: loading roles to cache")

	// Create the admin role
	adminTime := time.Date(2025, time.January, 1, 10, 0, 0, 0, time.UTC)
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
		},
		CreatedAt: adminTime,
		UpdatedAt: adminTime,
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
		log.Fatal().Msg(err.Error())
	}

	now := time.Now().UTC()
	role := &Role{
		Id:            id.String(),
		Name:          name,
		Permissions:   permissions,
		CreatedUserId: userId,
		CreatedAt:     now,
		UpdatedUserId: userId,
		UpdatedAt:     now,
	}

	return role
}

func RoleExists(roleId string) bool {
	_, ok := roleCache[roleId]
	return ok
}
