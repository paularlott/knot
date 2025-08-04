package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type Role struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RoleDetails struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
}

func listRoles(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	roles := model.GetRolesFromCache()

	var result []Role
	for _, role := range roles {
		if role.IsDeleted {
			continue
		}

		result = append(result, Role{
			ID:   role.Id,
			Name: role.Name,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func createRole(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageRoles) {
		return nil, fmt.Errorf("No permission to manage roles")
	}

	name := req.StringOr("name", "")
	if !validate.Required(name) || !validate.MaxLength(name, 64) {
		return nil, fmt.Errorf("Invalid user role name")
	}

	var permissions []uint16
	if perms, err := req.IntSlice("permissions"); err != mcp.ErrUnknownParameter {
		for _, perm := range perms {
			permissions = append(permissions, uint16(perm))
		}
	}

	role := model.NewRole(name, permissions, user.Id)

	err := database.GetInstance().SaveRole(role)
	if err != nil {
		return nil, fmt.Errorf("Failed to save role: %v", err)
	}

	model.SaveRoleToCache(role)
	service.GetTransport().GossipRole(role)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleCreate,
		fmt.Sprintf("Created role %s", role.Name),
		&map[string]interface{}{
			"role_id":   role.Id,
			"role_name": role.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
		"id":     role.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateRole(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageRoles) {
		return nil, fmt.Errorf("No permission to manage roles")
	}

	roleId := req.StringOr("role_id", "")
	if !validate.UUID(roleId) {
		return nil, fmt.Errorf("Invalid role ID")
	}

	if roleId == model.RoleAdminUUID {
		return nil, fmt.Errorf("Cannot update the admin role")
	}

	db := database.GetInstance()
	role, err := db.GetRole(roleId)
	if err != nil {
		return nil, fmt.Errorf("Role not found: %v", err)
	}

	// Update name if provided
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		if !validate.Required(name) || !validate.MaxLength(name, 64) {
			return nil, fmt.Errorf("Invalid user role name")
		}
		role.Name = name
	}

	// Handle permission operations
	if action, err := req.String("permission_action"); err != mcp.ErrUnknownParameter {
		switch action {
		case "replace":
			if perms, err := req.IntSlice("permissions"); err != mcp.ErrUnknownParameter {
				role.Permissions = []uint16{}
				for _, perm := range perms {
					role.Permissions = append(role.Permissions, uint16(perm))
				}
			}
		case "add":
			if perms, err := req.IntSlice("permissions"); err != mcp.ErrUnknownParameter {
				for _, perm := range perms {
					permVal := uint16(perm)
					// Check if permission already exists
					exists := false
					for _, existing := range role.Permissions {
						if existing == permVal {
							exists = true
							break
						}
					}
					if !exists {
						role.Permissions = append(role.Permissions, permVal)
					}
				}
			}
		case "remove":
			if perms, err := req.IntSlice("permissions"); err != mcp.ErrUnknownParameter {
				for _, perm := range perms {
					permVal := uint16(perm)
					// Remove permission
					newPerms := []uint16{}
					for _, existing := range role.Permissions {
						if existing != permVal {
							newPerms = append(newPerms, existing)
						}
					}
					role.Permissions = newPerms
				}
			}
		default:
			return nil, fmt.Errorf("Invalid permission_action. Must be 'replace', 'add', or 'remove'")
		}
	}

	role.UpdatedUserId = user.Id
	role.UpdatedAt = hlc.Now()

	err = db.SaveRole(role)
	if err != nil {
		return nil, fmt.Errorf("Failed to save role: %v", err)
	}

	model.SaveRoleToCache(role)
	service.GetTransport().GossipRole(role)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleUpdate,
		fmt.Sprintf("Updated role %s", role.Name),
		&map[string]interface{}{
			"role_id":   role.Id,
			"role_name": role.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteRole(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageRoles) {
		return nil, fmt.Errorf("No permission to manage roles")
	}

	roleId := req.StringOr("role_id", "")
	if !validate.UUID(roleId) {
		return nil, fmt.Errorf("Invalid role ID")
	}

	if roleId == model.RoleAdminUUID {
		return nil, fmt.Errorf("Cannot delete the admin role")
	}

	db := database.GetInstance()
	role, err := db.GetRole(roleId)
	if err != nil {
		return nil, fmt.Errorf("Role not found: %v", err)
	}

	role.UpdatedAt = hlc.Now()
	role.UpdatedUserId = user.Id
	role.IsDeleted = true
	err = db.SaveRole(role)
	if err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			return nil, fmt.Errorf("Role is in use and cannot be deleted")
		}
		return nil, fmt.Errorf("Failed to delete role: %v", err)
	}

	model.DeleteRoleFromCache(roleId)
	service.GetTransport().GossipRole(role)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventRoleDelete,
		fmt.Sprintf("Deleted role %s", role.Name),
		&map[string]interface{}{
			"role_id":   role.Id,
			"role_name": role.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func getRole(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageRoles) {
		return nil, fmt.Errorf("No permission to manage roles")
	}

	roleId := req.StringOr("role_id", "")
	if !validate.UUID(roleId) {
		return nil, fmt.Errorf("Invalid role ID")
	}

	db := database.GetInstance()
	role, err := db.GetRole(roleId)
	if err != nil {
		return nil, fmt.Errorf("Role not found: %v", err)
	}
	if role == nil {
		return nil, fmt.Errorf("Role not found")
	}

	// Build permission list with names
	var permissions []Permission
	for _, permId := range role.Permissions {
		for _, permName := range model.PermissionNames {
			if permName.Id == int(permId) {
				permissions = append(permissions, Permission{
					ID:   permName.Id,
					Name: permName.Name,
				})
				break
			}
		}
	}

	result := RoleDetails{
		ID:          role.Id,
		Name:        role.Name,
		Permissions: permissions,
	}

	return mcp.NewToolResponseJSON(result), nil
}
