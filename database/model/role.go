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
)

type PermissionName struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

var PermissionNames = []PermissionName{
	{PermissionManageGroups, "Manage Groups"},
	{PermissionManageRoles, "Manage Roles"},
	{PermissionManageSpaces, "Manage Spaces"},
	{PermissionManageTemplates, "Manage Templates"},
	{PermissionManageUsers, "Manage Users"},
	{PermissionManageVariables, "Manage Variables"},
	{PermissionManageVolumes, "Manage Volumes"},
	{PermissionUseSpaces, "Can Use Spaces"},
	{PermissionUseTunnels, "Can Use Tunnels"},
	{PermissionViewAuditLogs, "View Audit Logs"},
	{PermissionTransferSpaces, "Can Transfer Spaces"},
}

// Role
type Role struct {
	Id            string          `json:"role_id"`
	Name          string          `json:"name"`
	Permissions   JSONDbUIntArray `json:"permissions"`
	CreatedUserId string          `json:"created_user_id"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedUserId string          `json:"updated_user_id"`
	UpdatedAt     time.Time       `json:"updated_at"`
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
		},
	}

	roleCacheMutex.Lock()
	defer roleCacheMutex.Unlock()

	for _, role := range roles {
		roleCache[role.Id] = role
	}
}

func GetRoleFromCache(roleId string) *Role {
	roleCacheMutex.RLock()
	defer roleCacheMutex.RUnlock()

	role, ok := roleCache[roleId]
	if !ok {
		return nil
	}

	return role
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

	role := &Role{
		Id:            id.String(),
		Name:          name,
		Permissions:   permissions,
		CreatedUserId: userId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: userId,
		UpdatedAt:     time.Now().UTC(),
	}

	return role
}

func RoleExists(roleId string) bool {
	_, ok := roleCache[roleId]
	return ok
}
