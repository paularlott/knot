package mcp

import (
	"context"

	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/mcp"
)

func listIcons(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	iconService := service.GetIconService()
	icons := iconService.GetIcons()

	result := service.IconList{
		Icons: icons,
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}
