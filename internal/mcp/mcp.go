package mcp

import (
	"net/http"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/paularlott/mcp"
)

func InitializeMCPServer(routes *http.ServeMux) *mcp.Server {
	server := mcp.NewServer("knot-mcp-server", build.Version)
	routes.HandleFunc("POST /mcp", middleware.ApiAuth(server.HandleRequest))

	// Permissions
	server.RegisterTool(
		mcp.NewTool("list_permissions", "Get a list of all the permissions available within the system."),
		listPermissions,
	)

	// Groups
	server.RegisterTool(
		mcp.NewTool("list_groups", "Get a list of all the groups available within the system."),
		listGroups,
	)

	// Roles
	server.RegisterTool(
		mcp.NewTool("list_roles", "Get a list of all the roles available within the system."),
		listRoles,
	)

	// Templates
	server.RegisterTool(
		mcp.NewTool("list_templates", "Get a list of all templates available within the system."),
		listTemplates,
	)

	// Spaces
	server.RegisterTool(
		mcp.NewTool("list_spaces", "Get a list of all spaces on this server (zone) for the current user. Returns status, sharing and other details about the spaces."),
		listSpaces,
	)
	server.RegisterTool(
		mcp.NewTool("start_space", "Start a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to start", true),
		startSpace,
	)
	server.RegisterTool(
		mcp.NewTool("stop_space", "Stop a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to stop", true),
		stopSpace,
	)

	// TODO users
	// TODO tunnels

	// Specifications
	server.RegisterTool(
		mcp.NewTool("get_docker_podman_spec", "Get the complete Docker/Podman job specification documentation in markdown format"),
		getContainerSpec,
	)

	return server
}
