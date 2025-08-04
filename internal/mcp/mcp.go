package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/mcp"
)

//go:embed docker-podman-spec.md
var dockerPodmanSpec string

type MCPHandler struct {
	server *mcp.Server
}

func InitializeMCPServer(routes *http.ServeMux) *mcp.Server {
	handler := &MCPHandler{}
	server := mcp.NewServer("knot-mcp-server", build.Version)
	handler.server = server
	routes.HandleFunc("POST /mcp", middleware.ApiAuth(server.HandleRequest))

	// Register tools
	server.RegisterTool(
		mcp.NewTool("list_spaces", "List all spaces for a user or all users"),
		handler.listSpaces,
	)

	server.RegisterTool(
		mcp.NewTool("start_space", "Start a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to start", true),
		handler.startSpace,
	)

	server.RegisterTool(
		mcp.NewTool("stop_space", "Stop a space by its ID").
			AddParam("space_id", mcp.String, "The ID of the space to stop", true),
		handler.stopSpace,
	)

	server.RegisterTool(
		mcp.NewTool("get_docker_podman_spec", "Get the complete Docker/Podman job specification documentation in markdown format"),
		handler.getContainerSpec,
	)

	return server
}

func (h *MCPHandler) listSpaces(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()

	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get spaces: %v", err)
	}

	if len(spaces) == 0 {
		return mcp.NewToolResponseText("No spaces found."), nil
	}

	// Create markdown table for better AI consumption
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Found %d spaces:\n\n", len(spaces)))
	builder.WriteString("| ID | Name | State | Description | Note | Zone | Platform | Web Ports | TCP Ports | SSH | Web Terminal |\n")
	builder.WriteString("|----|------|-------|-------------|------|------|----------|-----------|-----------|-----|--------------|\n")

	for _, space := range spaces {
		// Ignore deleted spaces
		if space.IsDeleted {
			continue
		}

		state := "stopped"
		if space.IsDeleting {
			state = "deleting"
		} else if space.IsPending {
			state = "pending"
		} else if space.IsDeployed {
			state = "running"
		}

		// Get additional space details
		ports := "N/A"
		tcpPorts := "N/A"
		sshAvailable := "No"
		webTerminal := "No"
		platform := space.Zone
		note := space.Note

		if space.IsDeployed {
			// Load the space state
			state := agent_server.GetSession(space.Id)
			if state != nil {
				if len(state.HttpPorts) > 0 {
					var portList []string
					for portName, port := range state.HttpPorts {
						if portName == port {
							portList = append(portList, port)
						} else {
							portList = append(portList, fmt.Sprintf("%s = %s", portName, port))
						}
					}
					ports = strings.Join(portList, ", ")
				}
				if len(state.TcpPorts) > 0 {
					var portList []string
					for portName, port := range state.TcpPorts {
						if portName == port {
							portList = append(portList, port)
						} else {
							portList = append(portList, fmt.Sprintf("%s = %s", portName, port))
						}
					}
					tcpPorts = strings.Join(portList, ", ")
				}
				if state.SSHPort > 0 {
					sshAvailable = "Yes"
				}
				if state.HasTerminal {
					webTerminal = "Yes"
				}
			}
		}

		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			space.Id, space.Name, state, space.Description, note, space.Zone, platform, ports, tcpPorts, sshAvailable, webTerminal))
	}

	return mcp.NewToolResponseText(builder.String()), nil
}

func (h *MCPHandler) startSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check if user has permission to start this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to start this space")
	}

	// Get the templates
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get template: %v", err)
	}

	// Use the container service to start the space
	containerService := service.GetContainerService()
	err = containerService.StartSpace(space, template, user)
	if err != nil {
		return nil, fmt.Errorf("Failed to start space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' (%s) is starting", space.Name, spaceID)), nil
}

func (h *MCPHandler) stopSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check if user has permission to stop this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to stop this space")
	}

	// Use the container service to stop the space
	containerService := service.GetContainerService()
	err = containerService.StopSpace(space)
	if err != nil {
		return nil, fmt.Errorf("Failed to stop space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' (%s) is stopping", space.Name, spaceID)), nil
}

func (h *MCPHandler) getContainerSpec(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	return mcp.NewToolResponseText(dockerPodmanSpec), nil
}
