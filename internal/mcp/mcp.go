package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
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

	// Register tools for exposing data
	server.RegisterTool(
		mcp.NewTool("list_permissions", "Get a list of all the permissions available within the system."),
		handler.listPermissions,
	)
	server.RegisterTool(
		mcp.NewTool("list_groups", "Get a list of all the groups available within the system."),
		handler.listGroups,
	)
	server.RegisterTool(
		mcp.NewTool("list_roles", "Get a list of all the roles available within the system."),
		handler.listRoles,
	)
	server.RegisterTool(
		mcp.NewTool("list_spaces", "Get a list of all spaces on this server (zone) for the current user."),
		handler.listSpaces,
	)
	server.RegisterTool(
		mcp.NewTool("list_templates", "Get a list of all templates available within the system."),
		handler.listTemplates,
	)

	// TODO users
	// TODO tunnels

	// Register tools for working with spaces
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

	// Register tools for working with templates
	server.RegisterTool(
		mcp.NewTool("get_docker_podman_spec", "Get the complete Docker/Podman job specification documentation in markdown format"),
		handler.getContainerSpec,
	)

	return server
}

func (h *MCPHandler) listPermissions(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	var builder strings.Builder
	builder.WriteString("| ID | Group | Name |\n")
	builder.WriteString("|----|-------|-------|\n")

	for _, permission := range model.PermissionNames {
		builder.WriteString(fmt.Sprintf("| %d | %s | %s |\n", permission.Id, permission.Group, permission.Name))
	}

	return mcp.NewToolResponseText(builder.String()), nil
}

func (h *MCPHandler) listGroups(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()
	groups, err := db.GetGroups()
	if err != nil {
		return nil, fmt.Errorf("Failed to get groups: %v", err)
	}

	var builder strings.Builder
	builder.WriteString("| ID | Name | Max Spaces | Compute Units | Storage Units | Max Tunnels |\n")
	builder.WriteString("|----|------|------------|---------------|---------------|-------------|\n")

	for _, group := range groups {
		if group.IsDeleted {
			continue
		}

		builder.WriteString(fmt.Sprintf("| %s | %s | %d | %d | %d | %d |\n", group.Id, group.Name, group.MaxSpaces, group.ComputeUnits, group.StorageUnits, group.MaxTunnels))
	}

	return mcp.NewToolResponseText(builder.String()), nil
}

func (h *MCPHandler) listRoles(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()
	roles, err := db.GetRoles()
	if err != nil {
		return nil, fmt.Errorf("Failed to get roles: %v", err)
	}

	var builder strings.Builder
	builder.WriteString("| ID | Name |\n")

	for _, role := range roles {
		if role.IsDeleted {
			continue
		}

		builder.WriteString(fmt.Sprintf("| %s | %s |\n", role.Id, role.Name))
	}

	return mcp.NewToolResponseText(builder.String()), nil
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
	builder.WriteString("| ID | Name | State | Description | Note | Zone | Platform | Web Ports | TCP Ports | SSH | Web Terminal | Shared With |\n")
	builder.WriteString("|----|------|-------|-------------|------|------|----------|-----------|-----------|-----|--------------|-------------|\n")

	cfg := config.GetServerConfig()
	for _, space := range spaces {
		// Ignore deleted spaces or spaces not in this zone
		if space.IsDeleted || (space.Zone != "" && space.Zone != cfg.Zone) {
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
		ports := "-"
		tcpPorts := "-"
		sshAvailable := "No"
		webTerminal := "No"
		platform := "-"
		note := space.Note
		sharedWith := "-"

		template, err := db.GetTemplate(space.TemplateId)
		if err == nil {
			platform = template.Platform
		}

		if space.SharedWithUserId != "" {
			user, err := db.GetUser(space.SharedWithUserId)
			if err == nil {
				sharedWith = fmt.Sprintf("%s (ID: %s)", user.Username, user.Id)
			}
		}

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

		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			space.Id, space.Name, state, space.Description, note, space.Zone, platform, ports, tcpPorts, sshAvailable, webTerminal, sharedWith))
	}

	return mcp.NewToolResponseText(builder.String()), nil
}

func (h *MCPHandler) listTemplates(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		return nil, fmt.Errorf("Failed to get templates: %v", err)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Found %d templates:\n\n", len(templates)))
	builder.WriteString("| ID | Name | Description | Platform | Groups | Compute Units | Storage Units | Schedule Enabled | Is Managed | Schedule | Zones | Custom Fields | Max Uptime | Max Uptime Unit |\n")
	builder.WriteString("|----|------|-------------|----------|--------|---------------|---------------|------------------|------------|----------|-------|---------------|------------|-----------------|\n")

	// Load the groups so we can look up their names & convert to map id => group
	groups, err := db.GetGroups()
	if err != nil {
		return nil, fmt.Errorf("Failed to get groups: %v", err)
	}
	groupMap := make(map[string]*model.Group)
	for _, group := range groups {
		groupMap[group.Id] = group
	}

	cfg := config.GetServerConfig()
	for _, template := range templates {
		if template.IsDeleted || !template.Active || (len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups)) || !template.IsValidForZone(cfg.Zone) {
			continue
		}

		// Build the groups list
		var groupsList []string
		for _, group := range template.Groups {
			if grp, ok := groupMap[group]; ok {
				groupsList = append(groupsList, fmt.Sprintf("%s (ID: %s)", grp.Name, grp.Id))
			}
		}

		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %d | %d | %t | %t | %v | %s | %s | %d | %s |\n",
			template.Id, template.Name, template.Description, template.Platform, strings.Join(groupsList, ", "), template.ComputeUnits, template.StorageUnits, template.ScheduleEnabled, template.IsManaged, template.Schedule, strings.Join(template.Zones, ", "), template.CustomFields, template.MaxUptime, template.MaxUptimeUnit))
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
