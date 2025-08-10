package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

type User struct {
	ID       string   `json:"id"`
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Active   bool     `json:"active"`
	Groups   []string `json:"groups"`
}

func listUsers(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageUsers) && !user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to manage users")
	}

	db := database.GetInstance()
	users, err := db.GetUsers()
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
			Groups:   u.Groups,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}
