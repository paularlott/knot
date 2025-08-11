package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type Space struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	State        string                   `json:"state"`
	Description  string                   `json:"description"`
	Note         string                   `json:"note"`
	Zone         string                   `json:"zone"`
	Platform     string                   `json:"platform"`
	WebPorts     map[string]string        `json:"web_ports"`
	TCPPorts     map[string]string        `json:"tcp_ports"`
	SSH          bool                     `json:"ssh"`
	WebTerminal  bool                     `json:"web_terminal"`
	CustomFields []model.SpaceCustomField `json:"custom_fields"`
	SharedWith   SharedWith               `json:"shared_with,omitempty"`
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
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to list spaces")
	}

	spaceService := service.GetSpaceService()
	spaces, err := spaceService.ListSpaces(service.SpaceListOptions{
		User:           user,
		UserId:         user.Id,
		IncludeDeleted: false,
		CheckZone:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get spaces: %v", err)
	}

	db := database.GetInstance()
	var result []Space
	for _, space := range spaces {

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
			ID:           space.Id,
			Name:         space.Name,
			State:        state,
			Description:  space.Description,
			Note:         space.Note,
			Zone:         space.Zone,
			Platform:     platform,
			WebPorts:     webPorts,
			TCPPorts:     tcpPorts,
			SSH:          sshAvailable,
			WebTerminal:  webTerminal,
			CustomFields: space.CustomFields,
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

func createSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	space := model.NewSpace(
		req.StringOr("name", ""),
		req.StringOr("description", ""),
		user.Id,
		req.StringOr("template_id", ""),
		req.StringOr("shell", "bash"),
		&[]string{}, // no alt names in MCP
		"",          // zone will be set by service
		req.StringOr("icon_url", ""),
		parseSpaceCustomFields(req), // parse custom fields from request
	)

	spaceService := service.GetSpaceService()
	err := spaceService.CreateSpace(space, user)
	if err != nil {
		return nil, err
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventSpaceCreate,
		fmt.Sprintf("Created space %s", space.Name),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
		},
	)

	result := map[string]interface{}{
		"status":   true,
		"space_id": space.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	spaceId := req.StringOr("space_id", "")

	spaceService := service.GetSpaceService()
	space, err := spaceService.GetSpace(spaceId, user)
	if err != nil {
		return nil, err
	}

	// Apply updates based on provided parameters
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		space.Name = name
	}
	if description, err := req.String("description"); err != mcp.ErrUnknownParameter {
		space.Description = description
	}
	if templateId, err := req.String("template_id"); err != mcp.ErrUnknownParameter {
		space.TemplateId = templateId
	}
	if shell, err := req.String("shell"); err != mcp.ErrUnknownParameter {
		space.Shell = shell
	}
	if iconURL, err := req.String("icon_url"); err != mcp.ErrUnknownParameter {
		space.IconURL = iconURL
	}

	// Handle custom fields
	if customFields := parseSpaceCustomFields(req); len(customFields) > 0 {
		space.CustomFields = customFields
	}

	err = spaceService.UpdateSpace(space, user)
	if err != nil {
		return nil, err
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventSpaceUpdate,
		fmt.Sprintf("Updated space %s", space.Name),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	spaceId := req.StringOr("space_id", "")

	spaceService := service.GetSpaceService()

	// Get space name for audit log before deletion
	space, err := spaceService.GetSpace(spaceId, user)
	if err != nil {
		return nil, err
	}
	spaceName := space.Name

	// Check if space can be deleted (MCP-specific logic)
	cfg := config.GetServerConfig()
	if space.IsDeployed || space.IsPending || space.IsDeleting || (space.Zone != "" && space.Zone != cfg.Zone) {
		return nil, fmt.Errorf("Space cannot be deleted")
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventSpaceDelete,
		fmt.Sprintf("Deleted space %s", spaceName),
		&map[string]interface{}{
			"space_id":   spaceId,
			"space_name": spaceName,
		},
	)

	// Mark as deleting and delete in background (MCP-specific logic)
	space.IsDeleting = true
	space.UpdatedAt = hlc.Now()
	db := database.GetInstance()
	db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
	service.GetTransport().GossipSpace(space)

	// Delete the space in the background
	service.GetContainerService().DeleteSpace(space)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func manageSpaceState(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to use spaces")
	}

	spaceID, err := req.String("space_id")
	if err != nil || spaceID == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	action, err := req.String("action")
	if err != nil || action == "" {
		return nil, mcp.NewToolErrorInvalidParams("action is required")
	}

	if !validate.OneOf(action, []string{"start", "stop", "restart"}) {
		return nil, mcp.NewToolErrorInvalidParams("action must be 'start', 'stop', or 'restart'")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check permissions based on action
	switch action {
	case "start", "stop":
		if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
			return nil, fmt.Errorf("No permission to %s this space", action)
		}
	case "restart":
		if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
			return nil, fmt.Errorf("No permission to restart this space")
		}
	}

	containerService := service.GetContainerService()
	var actionMessage string

	switch action {
	case "start":
		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			return nil, fmt.Errorf("Failed to get template: %v", err)
		}
		err = containerService.StartSpace(space, template, user)
		if err != nil {
			return nil, fmt.Errorf("Failed to start space: %v", err)
		}
		actionMessage = "starting"
	case "stop":
		err = containerService.StopSpace(space)
		if err != nil {
			return nil, fmt.Errorf("Failed to stop space: %v", err)
		}
		actionMessage = "stopping"
	case "restart":
		err = containerService.RestartSpace(space)
		if err != nil {
			return nil, fmt.Errorf("Failed to restart space: %v", err)
		}
		actionMessage = "restarting"
	}

	response := SpaceOperationResponse{
		Message:   fmt.Sprintf("Space '%s' is %s", space.Name, actionMessage),
		SpaceName: space.Name,
		SpaceID:   spaceID,
	}

	return mcp.NewToolResponseJSON(response), nil
}

func manageSpaceSharing(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to transfer spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	action, err := req.String("action")
	if err != nil || action == "" {
		return nil, mcp.NewToolErrorInvalidParams("action is required")
	}

	if !validate.OneOf(action, []string{"share", "stop_sharing", "transfer"}) {
		return nil, mcp.NewToolErrorInvalidParams("action must be 'share', 'stop_sharing', or 'transfer'")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	switch action {
	case "share":
		userId := req.StringOr("user_id", "")
		if !validate.UUID(userId) {
			return nil, fmt.Errorf("Invalid user ID")
		}

		if space.UserId != user.Id {
			return nil, fmt.Errorf("Space not found")
		}

		if space.IsDeleting {
			return nil, fmt.Errorf("Space cannot be shared at this time")
		}

		if space.UserId == userId {
			return nil, fmt.Errorf("Cannot share with yourself")
		}

		newUser, err := db.GetUser(userId)
		if err != nil || newUser == nil || !newUser.Active {
			return nil, fmt.Errorf("User not found")
		}

		space.SharedWithUserId = newUser.Id
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"SharedWithUserId", "UpdatedAt"})
		if err != nil {
			return nil, fmt.Errorf("Failed to share space: %v", err)
		}

		service.GetTransport().GossipSpace(space)
		service.GetUserService().UpdateSpaceSSHKeys(space, user)

		audit.Log(
			user.Username,
			model.AuditActorTypeMCP,
			model.AuditEventSpaceShare,
			fmt.Sprintf("Shared space %s to user %s", space.Name, userId),
			&map[string]interface{}{
				"space_id":   space.Id,
				"space_name": space.Name,
				"user_id":    userId,
			},
		)

	case "stop_sharing":
		if space.UserId != user.Id && space.SharedWithUserId != user.Id {
			return nil, fmt.Errorf("Space not found")
		}

		if space.SharedWithUserId == "" {
			return nil, fmt.Errorf("Space is not shared")
		}

		space.SharedWithUserId = ""
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"SharedWithUserId", "UpdatedAt"})
		if err != nil {
			return nil, fmt.Errorf("Failed to stop sharing space: %v", err)
		}

		service.GetTransport().GossipSpace(space)
		service.GetUserService().UpdateSpaceSSHKeys(space, user)

		audit.Log(
			user.Username,
			model.AuditActorTypeMCP,
			model.AuditEventSpaceStopShare,
			fmt.Sprintf("Stop space share %s", space.Name),
			&map[string]interface{}{
				"space_id":   space.Id,
				"space_name": space.Name,
			},
		)

	case "transfer":
		userId := req.StringOr("user_id", "")
		if !validate.UUID(userId) {
			return nil, fmt.Errorf("Invalid user ID")
		}

		if space.UserId != user.Id {
			return nil, fmt.Errorf("Space not found")
		}

		if space.IsDeployed || space.IsPending || space.IsDeleting {
			return nil, fmt.Errorf("Space cannot be transferred at this time")
		}

		if space.UserId == userId {
			return nil, fmt.Errorf("Cannot transfer to yourself")
		}

		newUser, err := db.GetUser(userId)
		if err != nil || newUser == nil || !newUser.Active || newUser.IsDeleted {
			return nil, fmt.Errorf("User not found")
		}

		// Check quotas
		userQuota, err := database.GetUserQuota(newUser)
		if err != nil {
			return nil, fmt.Errorf("Failed to check user quota: %v", err)
		}

		userUsage, err := database.GetUserUsage(newUser.Id, "")
		if err != nil {
			return nil, fmt.Errorf("Failed to check user usage: %v", err)
		}

		if userQuota.MaxSpaces > 0 && uint32(userUsage.NumberSpaces) >= userQuota.MaxSpaces {
			return nil, fmt.Errorf("Space quota exceeded")
		}

		template, err := db.GetTemplate(space.TemplateId)
		if err != nil {
			return nil, fmt.Errorf("Failed to get template: %v", err)
		}

		if userQuota.StorageUnits > 0 && userUsage.StorageUnits+template.StorageUnits > userQuota.StorageUnits {
			return nil, fmt.Errorf("Storage unit quota exceeded")
		}

		// Check name conflicts and rename if needed
		name := space.Name
		attempt := 1
		for {
			existing, err := db.GetSpaceByName(userId, name)
			if err == nil && existing != nil {
				name = fmt.Sprintf("%s-%d", space.Name, attempt)
				attempt++
				if attempt > 10 {
					return nil, fmt.Errorf("User already has a space with the same name")
				}
				continue
			}
			break
		}

		space.Name = name
		space.UserId = userId
		space.UpdatedAt = hlc.Now()
		err = db.SaveSpace(space, []string{"Name", "UserId", "UpdatedAt"})
		if err != nil {
			return nil, fmt.Errorf("Failed to transfer space: %v", err)
		}

		service.GetTransport().GossipSpace(space)

		audit.Log(
			user.Username,
			model.AuditActorTypeMCP,
			model.AuditEventSpaceTransfer,
			fmt.Sprintf("Transfer space %s to user %s", space.Name, userId),
			&map[string]interface{}{
				"space_id":   space.Id,
				"space_name": space.Name,
				"user_id":    userId,
			},
		)
	}

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

// parseSpaceCustomFields extracts custom field values from the MCP request
// Expects custom_fields as an array of objects with name and value properties
func parseSpaceCustomFields(req *mcp.ToolRequest) []model.SpaceCustomField {
	var fields []model.SpaceCustomField

	// Get array of custom field objects
	customFields, err := req.ObjectSlice("custom_fields")
	if err != mcp.ErrUnknownParameter {
		for _, fieldObj := range customFields {
			field := model.SpaceCustomField{}

			// Extract name (required)
			if name, ok := fieldObj["name"].(string); ok && name != "" {
				field.Name = name
			} else {
				continue // Skip fields without valid names
			}

			// Extract value (required)
			if value, ok := fieldObj["value"].(string); ok {
				field.Value = value
			}

			fields = append(fields, field)
		}
	}

	return fields
}
