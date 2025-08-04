package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Active   bool     `json:"active"`
	Roles    []string `json:"roles"`
	Groups   []string `json:"groups"`
}

func listUsers(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageUsers) && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to manage users")
	}

	users, err := database.GetInstance().GetUsers()
	if err != nil {
		return nil, fmt.Errorf("Failed to get users: %v", err)
	}

	var result []User
	for _, u := range users {
		if u.IsDeleted {
			continue
		}

		result = append(result, User{
			ID:       u.Id,
			Username: u.Username,
			Email:    u.Email,
			Active:   u.Active,
			Roles:    u.Roles,
			Groups:   u.Groups,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func createUser(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageUsers) {
		return nil, fmt.Errorf("No permission to manage users")
	}

	username := req.StringOr("username", "")
	email := req.StringOr("email", "")
	password := req.StringOr("password", "")

	if !validate.Name(username) || !validate.Password(password) || !validate.Email(email) {
		return nil, fmt.Errorf("Invalid username, password, or email given for new user")
	}

	shell := req.StringOr("preferred_shell", "bash")
	if !validate.OneOf(shell, []string{"bash", "zsh", "fish", "sh"}) {
		return nil, fmt.Errorf("Invalid shell")
	}

	timezone := req.StringOr("timezone", "UTC")
	if !validate.OneOf(timezone, util.Timezones) {
		return nil, fmt.Errorf("Invalid timezone")
	}

	sshKey := req.StringOr("ssh_public_key", "")
	if !validate.MaxLength(sshKey, 16*1024) {
		return nil, fmt.Errorf("SSH public key too long")
	}

	githubUsername := req.StringOr("github_username", "")
	if !validate.MaxLength(githubUsername, 255) {
		return nil, fmt.Errorf("GitHub username too long")
	}

	maxSpaces := req.IntOr("max_spaces", 0)
	if !validate.IsNumber(maxSpaces, 0, 10000) {
		return nil, fmt.Errorf("Invalid max spaces")
	}

	computeUnits := req.IntOr("compute_units", 0)
	if !validate.IsPositiveNumber(computeUnits) {
		return nil, fmt.Errorf("Invalid compute units")
	}

	storageUnits := req.IntOr("storage_units", 0)
	if !validate.IsPositiveNumber(storageUnits) {
		return nil, fmt.Errorf("Invalid storage units")
	}

	maxTunnels := req.IntOr("max_tunnels", 0)
	if !validate.IsPositiveNumber(maxTunnels) {
		return nil, fmt.Errorf("Invalid tunnel limit")
	}

	var roles []string
	if r, err := req.StringSlice("roles"); err != mcp.ErrUnknownParameter {
		for _, role := range r {
			if model.RoleExists(role) {
				roles = append(roles, role)
			}
		}
	}

	var groups []string
	if g, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
		db := database.GetInstance()
		for _, groupId := range g {
			if _, err := db.GetGroup(groupId); err == nil {
				groups = append(groups, groupId)
			}
		}
	}

	newUser := model.NewUser(username, email, password, roles, groups, sshKey, shell, timezone, uint32(maxSpaces), githubUsername, uint32(computeUnits), uint32(storageUnits), uint32(maxTunnels))

	err := database.GetInstance().SaveUser(newUser, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to save user: %v", err)
	}

	service.GetTransport().GossipUser(newUser)

	if middleware.HasUsers {
		audit.Log(
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventUserCreate,
			fmt.Sprintf("Created user %s (%s)", newUser.Username, newUser.Email),
			&map[string]interface{}{
				"user_id":    newUser.Id,
				"user_name":  newUser.Username,
				"user_email": newUser.Email,
			},
		)
	}

	middleware.HasUsers = true

	result := map[string]interface{}{
		"status":  true,
		"user_id": newUser.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateUser(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	activeUser := ctx.Value("user").(*model.User)
	userId := req.StringOr("user_id", "")

	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	if !activeUser.HasPermission(model.PermissionManageUsers) && activeUser.Id != userId {
		return nil, fmt.Errorf("No permission to manage users")
	}

	db := database.GetInstance()
	user, err := db.GetUser(userId)
	if err != nil {
		return nil, fmt.Errorf("User not found: %v", err)
	}

	// Update email if provided
	if email, err := req.String("email"); err != mcp.ErrUnknownParameter {
		if !validate.Email(email) {
			return nil, fmt.Errorf("Invalid email")
		}
		user.Email = email
	}

	// Update password if provided
	if password, err := req.String("password"); err != mcp.ErrUnknownParameter {
		if !validate.Password(password) {
			return nil, fmt.Errorf("Invalid password given")
		}
		user.SetPassword(password)
	}

	// Update shell if provided
	if shell, err := req.String("preferred_shell"); err != mcp.ErrUnknownParameter {
		if !validate.OneOf(shell, []string{"bash", "zsh", "fish", "sh"}) {
			return nil, fmt.Errorf("Invalid shell")
		}
		user.PreferredShell = shell
	}

	// Update timezone if provided
	if timezone, err := req.String("timezone"); err != mcp.ErrUnknownParameter {
		if !validate.OneOf(timezone, util.Timezones) {
			return nil, fmt.Errorf("Invalid timezone")
		}
		user.Timezone = timezone
	}

	// Update SSH key if provided
	if sshKey, err := req.String("ssh_public_key"); err != mcp.ErrUnknownParameter {
		if !validate.MaxLength(sshKey, 16*1024) {
			return nil, fmt.Errorf("SSH public key too long")
		}
		user.SSHPublicKey = sshKey
	}

	// Update GitHub username if provided
	if githubUsername, err := req.String("github_username"); err != mcp.ErrUnknownParameter {
		if !validate.MaxLength(githubUsername, 255) {
			return nil, fmt.Errorf("GitHub username too long")
		}
		user.GitHubUsername = githubUsername
	}

	// Admin-only fields
	if activeUser.HasPermission(model.PermissionManageUsers) {
		if active, err := req.Bool("active"); err != mcp.ErrUnknownParameter && activeUser.Id != user.Id {
			user.Active = active
		}

		if maxSpaces, err := req.Int("max_spaces"); err != mcp.ErrUnknownParameter {
			if !validate.IsNumber(maxSpaces, 0, 10000) {
				return nil, fmt.Errorf("Invalid max spaces")
			}
			user.MaxSpaces = uint32(maxSpaces)
		}

		if computeUnits, err := req.Int("compute_units"); err != mcp.ErrUnknownParameter {
			if !validate.IsPositiveNumber(computeUnits) {
				return nil, fmt.Errorf("Invalid compute units")
			}
			user.ComputeUnits = uint32(computeUnits)
		}

		if storageUnits, err := req.Int("storage_units"); err != mcp.ErrUnknownParameter {
			if !validate.IsPositiveNumber(storageUnits) {
				return nil, fmt.Errorf("Invalid storage units")
			}
			user.StorageUnits = uint32(storageUnits)
		}

		if maxTunnels, err := req.Int("max_tunnels"); err != mcp.ErrUnknownParameter {
			if !validate.IsPositiveNumber(maxTunnels) {
				return nil, fmt.Errorf("Invalid tunnel limit")
			}
			user.MaxTunnels = uint32(maxTunnels)
		}

		// Handle role operations
		if action, err := req.String("role_action"); err != mcp.ErrUnknownParameter {
			switch action {
			case "replace":
				if roles, err := req.StringSlice("roles"); err != mcp.ErrUnknownParameter {
					user.Roles = []string{}
					for _, role := range roles {
						if model.RoleExists(role) {
							user.Roles = append(user.Roles, role)
						}
					}
				}
			case "add":
				if roles, err := req.StringSlice("roles"); err != mcp.ErrUnknownParameter {
					for _, role := range roles {
						if model.RoleExists(role) {
							exists := false
							for _, existing := range user.Roles {
								if existing == role {
									exists = true
									break
								}
							}
							if !exists {
								user.Roles = append(user.Roles, role)
							}
						}
					}
				}
			case "remove":
				if roles, err := req.StringSlice("roles"); err != mcp.ErrUnknownParameter {
					for _, role := range roles {
						newRoles := []string{}
						for _, existing := range user.Roles {
							if existing != role {
								newRoles = append(newRoles, existing)
							}
						}
						user.Roles = newRoles
					}
				}
			default:
				return nil, fmt.Errorf("Invalid role_action. Must be 'replace', 'add', or 'remove'")
			}
		}

		// Handle group operations
		if action, err := req.String("group_action"); err != mcp.ErrUnknownParameter {
			switch action {
			case "replace":
				if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
					user.Groups = []string{}
					for _, groupId := range groups {
						if _, err := db.GetGroup(groupId); err == nil {
							user.Groups = append(user.Groups, groupId)
						}
					}
				}
			case "add":
				if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
					for _, groupId := range groups {
						if _, err := db.GetGroup(groupId); err == nil {
							exists := false
							for _, existing := range user.Groups {
								if existing == groupId {
									exists = true
									break
								}
							}
							if !exists {
								user.Groups = append(user.Groups, groupId)
							}
						}
					}
				}
			case "remove":
				if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
					for _, groupId := range groups {
						newGroups := []string{}
						for _, existing := range user.Groups {
							if existing != groupId {
								newGroups = append(newGroups, existing)
							}
						}
						user.Groups = newGroups
					}
				}
			default:
				return nil, fmt.Errorf("Invalid group_action. Must be 'replace', 'add', or 'remove'")
			}
		}
	}

	user.UpdatedAt = hlc.Now()
	err = db.SaveUser(user, []string{"Email", "Password", "PreferredShell", "Timezone", "SSHPublicKey", "GitHubUsername", "Active", "Roles", "Groups", "MaxSpaces", "ComputeUnits", "StorageUnits", "MaxTunnels", "UpdatedAt"})
	if err != nil {
		return nil, fmt.Errorf("Failed to save user: %v", err)
	}

	service.GetTransport().GossipUser(user)
	go service.GetUserService().UpdateUserSpaces(user)

	audit.Log(
		activeUser.Username,
		model.AuditActorTypeUser,
		model.AuditEventUserUpdate,
		fmt.Sprintf("Updated user %s (%s)", user.Username, user.Email),
		&map[string]interface{}{
			"user_id":    user.Id,
			"user_name":  user.Username,
			"user_email": user.Email,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteUser(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	activeUser := ctx.Value("user").(*model.User)
	if !activeUser.HasPermission(model.PermissionManageUsers) {
		return nil, fmt.Errorf("No permission to manage users")
	}

	userId := req.StringOr("user_id", "")
	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	if activeUser.Id == userId {
		return nil, fmt.Errorf("Cannot delete self")
	}

	db := database.GetInstance()
	toDelete, err := db.GetUser(userId)
	if err != nil {
		return nil, fmt.Errorf("User not found: %v", err)
	}

	if err := service.GetUserService().DeleteUser(toDelete); err != nil {
		return nil, fmt.Errorf("Failed to delete user: %v", err)
	}

	audit.Log(
		activeUser.Username,
		model.AuditActorTypeUser,
		model.AuditEventUserDelete,
		fmt.Sprintf("Deleted user %s (%s)", toDelete.Username, toDelete.Email),
		&map[string]interface{}{
			"user_id":    toDelete.Id,
			"user_name":  toDelete.Username,
			"user_email": toDelete.Email,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}