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

func shareSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to transfer spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	userId := req.StringOr("user_id", "")
	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
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
		model.AuditActorTypeUser,
		model.AuditEventSpaceShare,
		fmt.Sprintf("Shared space %s to user %s", space.Name, userId),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
			"user_id":    userId,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func stopSharingSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to transfer spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

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
		model.AuditActorTypeUser,
		model.AuditEventSpaceStopShare,
		fmt.Sprintf("Stop space share %s", space.Name),
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

func transferSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to transfer spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	userId := req.StringOr("user_id", "")
	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
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
		model.AuditActorTypeUser,
		model.AuditEventSpaceTransfer,
		fmt.Sprintf("Transfer space %s to user %s", space.Name, userId),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
			"user_id":    userId,
		},
	)

	result := map[string]interface{}{
		"status": true,
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

func restartSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to use spaces")
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

	// Check if user has permission to restart this space
	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to restart this space")
	}

	// Use the container service to restart the space
	containerService := service.GetContainerService()
	err = containerService.RestartSpace(space)
	if err != nil {
		return nil, fmt.Errorf("Failed to restart space: %v", err)
	}

	response := SpaceOperationResponse{
		Message:   fmt.Sprintf("Space '%s' is restarting", space.Name),
		SpaceName: space.Name,
		SpaceID:   spaceID,
	}

	return mcp.NewToolResponseJSON(response), nil
}

func createSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to manage or use spaces")
	}

	name := req.StringOr("name", "")
	if !validate.Name(name) {
		return nil, fmt.Errorf("Invalid name given for new space")
	}

	templateId := req.StringOr("template_id", "")
	if !validate.UUID(templateId) {
		return nil, fmt.Errorf("Invalid template ID")
	}

	description := req.StringOr("description", "")
	if !validate.MaxLength(description, 1024) {
		return nil, fmt.Errorf("Description too long")
	}

	shell := req.StringOr("shell", "bash")
	if !validate.OneOf(shell, []string{"bash", "zsh", "fish", "sh"}) {
		return nil, fmt.Errorf("Invalid shell given for space")
	}

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	template, err := db.GetTemplate(templateId)
	if err != nil || template == nil || template.IsDeleted || !template.Active {
		return nil, fmt.Errorf("Invalid template given for new space")
	}

	// Check template group permissions
	if !user.HasPermission(model.PermissionManageTemplates) && len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups) {
		return nil, fmt.Errorf("No permission to use this template")
	}

	// Check if space creation is disabled
	if cfg.DisableSpaceCreate {
		return nil, fmt.Errorf("Space creation is disabled")
	}

	// Check quotas if not on leaf node
	if !cfg.LeafNode {
		usage, err := database.GetUserUsage(user.Id, "")
		if err != nil {
			return nil, fmt.Errorf("Failed to check user usage: %v", err)
		}

		userQuota, err := database.GetUserQuota(user)
		if err != nil {
			return nil, fmt.Errorf("Failed to check user quota: %v", err)
		}

		if userQuota.MaxSpaces > 0 && uint32(usage.NumberSpaces+1) > userQuota.MaxSpaces {
			return nil, fmt.Errorf("Space quota exceeded")
		}

		if userQuota.StorageUnits > 0 && uint32(usage.StorageUnits+template.StorageUnits) > userQuota.StorageUnits {
			return nil, fmt.Errorf("Storage unit quota exceeded")
		}
	}

	space := model.NewSpace(name, description, user.Id, templateId, shell, &[]string{}, cfg.Zone, req.StringOr("icon_url", ""), []model.SpaceCustomField{})
	err = db.SaveSpace(space, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to save space: %v", err)
	}

	service.GetTransport().GossipSpace(space)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
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
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to manage or use spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("Space not found")
	}

	if space.Zone != "" && space.Zone != cfg.Zone {
		return nil, fmt.Errorf("Space not on this server")
	}

	// Update name if provided
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		if !validate.Name(name) {
			return nil, fmt.Errorf("Invalid name given for space")
		}
		space.Name = name
	}

	// Update description if provided
	if description, err := req.String("description"); err != mcp.ErrUnknownParameter {
		if !validate.MaxLength(description, 1024) {
			return nil, fmt.Errorf("Description too long")
		}
		space.Description = description
	}

	// Update template if provided
	if templateId, err := req.String("template_id"); err != mcp.ErrUnknownParameter {
		if !validate.UUID(templateId) {
			return nil, fmt.Errorf("Invalid template ID")
		}
		template, err := db.GetTemplate(templateId)
		if err != nil || template == nil {
			return nil, fmt.Errorf("Unknown template")
		}
		space.TemplateId = templateId
	}

	// Update shell if provided
	if shell, err := req.String("shell"); err != mcp.ErrUnknownParameter {
		if !validate.OneOf(shell, []string{"bash", "zsh", "fish", "sh"}) {
			return nil, fmt.Errorf("Invalid shell given for space")
		}
		space.Shell = shell
	}

	// Update icon URL if provided
	if iconURL, err := req.String("icon_url"); err != mcp.ErrUnknownParameter {
		space.IconURL = iconURL
	}

	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"Name", "Description", "TemplateId", "Shell", "IconURL", "UpdatedAt"})
	if err != nil {
		return nil, fmt.Errorf("Failed to save space: %v", err)
	}

	service.GetTransport().GossipSpace(space)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
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
	if !user.HasPermission(model.PermissionUseSpaces) && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to manage or use spaces")
	}

	spaceId := req.StringOr("space_id", "")
	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	db := database.GetInstance()
	cfg := config.GetServerConfig()

	space, err := db.GetSpace(spaceId)
	if err != nil || (space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces)) {
		return nil, fmt.Errorf("Space not found")
	}

	// Check if space can be deleted
	if space.IsDeployed || space.IsPending || space.IsDeleting || (space.Zone != "" && space.Zone != cfg.Zone) {
		return nil, fmt.Errorf("Space cannot be deleted")
	}

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSpaceDelete,
		fmt.Sprintf("Deleted space %s", space.Name),
		&map[string]interface{}{
			"space_id":   space.Id,
			"space_name": space.Name,
		},
	)

	// Mark as deleting and delete in background
	space.IsDeleting = true
	space.UpdatedAt = hlc.Now()
	db.SaveSpace(space, []string{"IsDeleting", "UpdatedAt"})
	service.GetTransport().GossipSpace(space)

	// Delete the space in the background
	service.GetContainerService().DeleteSpace(space)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}
