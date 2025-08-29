package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/api/api_utils"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

// resolveSpaceNameToID resolves a space name to its ID for the given user
func resolveSpaceNameToID(spaceName string, user *model.User) (string, error) {
	db := database.GetInstance()
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil || space == nil {
		return "", fmt.Errorf("Space '%s' not found", spaceName)
	}
	return space.Id, nil
}

// resolveTemplateNameToID resolves a template name to its ID
func resolveTemplateNameToID(templateName string) (string, error) {
	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		return "", fmt.Errorf("Failed to get templates: %v", err)
	}

	for _, template := range templates {
		if template.Name == templateName && !template.IsDeleted {
			return template.Id, nil
		}
	}

	return "", fmt.Errorf("Template '%s' not found", templateName)
}

type Space struct {
	ID           string                   `json:"id"`
	Name         string                   `json:"name"`
	State        string                   `json:"state"`
	Description  string                   `json:"description"`
	Note         string                   `json:"note"`
	Zone         string                   `json:"zone"`
	Platform     string                   `json:"platform"`
	WebPorts     []string                 `json:"web_ports"`
	TCPPorts     []string                 `json:"tcp_ports"`
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

type SpaceList struct {
	Spaces []Space `json:"spaces"`
}

type SpaceDefinition struct {
	UserId       string                       `json:"user_id"`
	TemplateId   string                       `json:"template_id"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Shell        string                       `json:"shell"`
	Zone         string                       `json:"zone"`
	AltNames     []string                     `json:"alt_names"`
	IsDeployed   bool                         `json:"is_deployed"`
	IsPending    bool                         `json:"is_pending"`
	IsDeleting   bool                         `json:"is_deleting"`
	StartedAt    time.Time                    `json:"started_at"`
	CreatedAt    time.Time                    `json:"created_at"`
	IconURL      string                       `json:"icon_url"`
	CustomFields []apiclient.CustomFieldValue `json:"custom_fields"`
}

func getSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	spaceName := req.StringOr("space_name", "")

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	data, err := api_utils.GetSpaceDetails(spaceId, user)
	if err != nil {
		return nil, err
	}

	result := SpaceDefinition{
		UserId:       data.UserId,
		TemplateId:   data.TemplateId,
		Name:         data.Name,
		Description:  data.Description,
		Shell:        data.Shell,
		Zone:         data.Zone,
		AltNames:     data.AltNames,
		IsDeployed:   data.IsDeployed,
		IsPending:    data.IsPending,
		IsDeleting:   data.IsDeleting,
		StartedAt:    data.StartedAt,
		CreatedAt:    data.CreatedAt,
		IconURL:      data.IconURL,
		CustomFields: data.CustomFields,
	}

	if result.AltNames == nil {
		result.AltNames = make([]string, 0)
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
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
		webPorts := make([]string, 0)
		tcpPorts := make([]string, 0)
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
				for name, port := range state.HttpPorts {
					webPorts = append(webPorts, fmt.Sprintf("%s:%s", name, port))
				}
				for name, port := range state.TcpPorts {
					tcpPorts = append(tcpPorts, fmt.Sprintf("%s:%s", name, port))
				}
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

	spaceList := SpaceList{Spaces: result}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(spaceList),
		mcp.NewToolResponseStructured(spaceList),
	), nil
}

func createSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	templateName := req.StringOr("template_name", "")
	templateId, err := resolveTemplateNameToID(templateName)
	if err != nil {
		return nil, err
	}

	// Load the template
	template, err := database.GetInstance().GetTemplate(templateId)
	if err != nil {
		return nil, fmt.Errorf("failed to load template: %v", err)
	}

	space := model.NewSpace(
		req.StringOr("name", ""),
		req.StringOr("description", ""),
		user.Id,
		templateId,
		req.StringOr("shell", "bash"),
		&[]string{}, // no alt names in MCP
		"",          // zone will be set by service
		req.StringOr("icon_url", template.IconURL),
		parseSpaceCustomFields(req), // parse custom fields from request
	)

	spaceService := service.GetSpaceService()
	err = spaceService.CreateSpace(space, user)
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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func updateSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	spaceName := req.StringOr("space_name", "")

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

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
	if templateName, err := req.String("template_name"); err != mcp.ErrUnknownParameter {
		templateId, err := resolveTemplateNameToID(templateName)
		if err != nil {
			return nil, err
		}
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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func deleteSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	spaceName := req.StringOr("space_name", "")

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	spaceService := service.GetSpaceService()

	// Get space name for audit log before deletion
	space, err := spaceService.GetSpace(spaceId, user)
	if err != nil {
		return nil, err
	}

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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(result),
		mcp.NewToolResponseStructured(result),
	), nil
}

func startSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to use spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	spaceID, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to start this space")
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get template: %v", err)
	}

	containerService := service.GetContainerService()
	err = containerService.StartSpace(space, template, user)
	if err != nil {
		return nil, fmt.Errorf("Failed to start space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' is starting", space.Name)), nil
}

func stopSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to use spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	spaceID, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to stop this space")
	}

	containerService := service.GetContainerService()
	err = containerService.StopSpace(space)
	if err != nil {
		return nil, fmt.Errorf("Failed to stop space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' is stopping", space.Name)), nil
}

func restartSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to use spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	spaceID, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to restart this space")
	}

	containerService := service.GetContainerService()
	err = containerService.RestartSpace(space)
	if err != nil {
		return nil, fmt.Errorf("Failed to restart space: %v", err)
	}

	return mcp.NewToolResponseText(fmt.Sprintf("Space '%s' is restarting", space.Name)), nil
}

func shareSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to share spaces")
	}

	spaceName := req.StringOr("space_name", "")
	if spaceName == "" {
		return nil, fmt.Errorf("Space name is required")
	}

	userId := req.StringOr("user_id", "")
	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("space not found: %v", err)
	}

	if space.UserId != user.Id {
		return nil, fmt.Errorf("space not found")
	}

	if space.IsDeleting {
		return nil, fmt.Errorf("space cannot be shared at this time")
	}

	if space.UserId == userId {
		return nil, fmt.Errorf("cannot share with yourself")
	}

	newUser, err := db.GetUser(userId)
	if err != nil || newUser == nil || !newUser.Active {
		return nil, fmt.Errorf("User not found")
	}

	space.SharedWithUserId = newUser.Id
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"SharedWithUserId", "UpdatedAt"})
	if err != nil {
		return nil, fmt.Errorf("failed to share space: %v", err)
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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(map[string]interface{}{"status": true}),
		mcp.NewToolResponseStructured(map[string]interface{}{"status": true}),
	), nil
}

func stopSharingSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("no permission to manage space sharing")
	}

	spaceName := req.StringOr("space_name", "")
	if spaceName == "" {
		return nil, fmt.Errorf("space name is required")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("space not found: %v", err)
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id {
		return nil, fmt.Errorf("space not found")
	}

	if space.SharedWithUserId == "" {
		return nil, fmt.Errorf("space is not shared")
	}

	space.SharedWithUserId = ""
	space.UpdatedAt = hlc.Now()
	err = db.SaveSpace(space, []string{"SharedWithUserId", "UpdatedAt"})
	if err != nil {
		return nil, fmt.Errorf("failed to stop sharing space: %v", err)
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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(map[string]interface{}{"status": true}),
		mcp.NewToolResponseStructured(map[string]interface{}{"status": true}),
	), nil
}

func transferSpace(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionTransferSpaces) {
		return nil, fmt.Errorf("No permission to transfer spaces")
	}

	spaceName := req.StringOr("space_name", "")
	if spaceName == "" {
		return nil, fmt.Errorf("Space name is required")
	}

	userId := req.StringOr("user_id", "")
	if !validate.UUID(userId) {
		return nil, fmt.Errorf("Invalid user ID")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("space not found: %v", err)
	}

	if space.UserId != user.Id {
		return nil, fmt.Errorf("space not found")
	}

	if space.IsDeployed || space.IsPending || space.IsDeleting {
		return nil, fmt.Errorf("space cannot be transferred at this time")
	}

	if space.UserId == userId {
		return nil, fmt.Errorf("cannot transfer to yourself")
	}

	newUser, err := db.GetUser(userId)
	if err != nil || newUser == nil || !newUser.Active || newUser.IsDeleted {
		return nil, fmt.Errorf("User not found")
	}

	userQuota, err := database.GetUserQuota(newUser)
	if err != nil {
		return nil, fmt.Errorf("failed to check user quota: %v", err)
	}

	userUsage, err := database.GetUserUsage(newUser.Id, "")
	if err != nil {
		return nil, fmt.Errorf("failed to check user usage: %v", err)
	}

	if userQuota.MaxSpaces > 0 && uint32(userUsage.NumberSpaces) >= userQuota.MaxSpaces {
		return nil, fmt.Errorf("Space quota exceeded")
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %v", err)
	}

	if userQuota.StorageUnits > 0 && userUsage.StorageUnits+template.StorageUnits > userQuota.StorageUnits {
		return nil, fmt.Errorf("storage unit quota exceeded")
	}

	name := space.Name
	attempt := 1
	for {
		existing, err := db.GetSpaceByName(userId, name)
		if err == nil && existing != nil {
			name = fmt.Sprintf("%s-%d", space.Name, attempt)
			attempt++
			if attempt > 10 {
				return nil, fmt.Errorf("user already has a space with the same name")
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
		return nil, fmt.Errorf("failed to transfer space: %v", err)
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

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(map[string]interface{}{"status": true}),
		mcp.NewToolResponseStructured(map[string]interface{}{"status": true}),
	), nil
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
