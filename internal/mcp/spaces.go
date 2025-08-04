package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"

	"github.com/paularlott/mcp"
)

type Space struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	State       string            `json:"state"`
	Description string            `json:"description"`
	Note        string            `json:"note"`
	Zone        string            `json:"zone"`
	Platform    string            `json:"platform"`
	WebPorts    map[string]string `json:"web_ports"`
	TCPPorts    map[string]string `json:"tcp_ports"`
	SSH         bool              `json:"ssh"`
	WebTerminal bool              `json:"web_terminal"`
	SharedWith  SharedWith        `json:"shared_with,omitempty"`
}

type SharedWith struct {
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
}

// Generic response structure for space operations
type SpaceOperationResponse struct {
	Message   string `json:"message"`
	SpaceName string `json:"space_name"`
	SpaceID   string `json:"space_id"`
}

func listSpaces(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()

	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return nil, fmt.Errorf("Failed to get spaces: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []Space
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
		webPorts := make(map[string]string)
		tcpPorts := make(map[string]string)
		sshAvailable := false
		webTerminal := false
		platform := ""

		template, err := db.GetTemplate(space.TemplateId)
		if err == nil {
			platform = template.Platform
		}

		if space.IsDeployed {
			// Load the space state
			state := agent_server.GetSession(space.Id)
			if state != nil {
				webPorts = state.HttpPorts
				tcpPorts = state.TcpPorts
				sshAvailable = state.SSHPort > 0
				webTerminal = state.HasTerminal
			}
		}

		s := Space{
			ID:          space.Id,
			Name:        space.Name,
			State:       state,
			Description: space.Description,
			Note:        space.Note,
			Zone:        space.Zone,
			Platform:    platform,
			WebPorts:    webPorts,
			TCPPorts:    tcpPorts,
			SSH:         sshAvailable,
			WebTerminal: webTerminal,
		}

		if space.SharedWithUserId != "" {
			user, err := db.GetUser(space.SharedWithUserId)
			if err == nil {
				s.SharedWith.UserID = user.Id
				s.SharedWith.Username = user.Username
				s.SharedWith.Email = user.Email
			}
		}

		result = append(result, s)
	}

	return mcp.NewToolResponseJSON(result), nil
}

func startSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	fmt.Println("Starting space: %s", spaceID)

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

	response := SpaceOperationResponse{
		Message:   fmt.Sprintf("Space '%s' is starting", space.Name),
		SpaceName: space.Name,
		SpaceID:   spaceID,
	}

	return mcp.NewToolResponseJSON(response), nil
}

func stopSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
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

	response := SpaceOperationResponse{
		Message:   fmt.Sprintf("Space '%s' is stopping", space.Name),
		SpaceName: space.Name,
		SpaceID:   spaceID,
	}

	return mcp.NewToolResponseJSON(response), nil
}
