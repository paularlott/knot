package mcp

import (
	"context"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/tunnel_server"

	"github.com/paularlott/mcp"
)

type Tunnel struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

func listTunnels(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseTunnels) {
		return nil, fmt.Errorf("No permission to use tunnels")
	}

	tunnels := tunnel_server.GetTunnelsForUser(user.Id)
	cfg := config.GetServerConfig()

	sort.Strings(tunnels)

	var result []Tunnel
	for _, tunnel := range tunnels {
		result = append(result, Tunnel{
			Name:    user.Username + "--" + tunnel,
			Address: "https://" + user.Username + "--" + tunnel + cfg.TunnelDomain,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteTunnel(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseTunnels) {
		return nil, fmt.Errorf("No permission to use tunnels")
	}

	tunnelName := req.StringOr("tunnel_name", "")
	if tunnelName == "" {
		return nil, fmt.Errorf("tunnel_name parameter is required")
	}

	err := tunnel_server.DeleteTunnel(user.Id, tunnelName)
	if err != nil {
		return nil, fmt.Errorf("Failed to delete tunnel: %v", err)
	}

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}