package mcp

import (
	"context"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/mcp"
)

type Permission struct {
	ID    int    `json:"id"`
	Group string `json:"group"`
	Name  string `json:"name"`
}

func listPermissions(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	var permissions []Permission
	for _, permission := range model.PermissionNames {
		permissions = append(permissions, Permission{
			ID:    permission.Id,
			Group: permission.Group,
			Name:  permission.Name,
		})
	}

	return mcp.NewToolResponseJSON(permissions), nil
}
