package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type Template struct {
	ID              string                      `json:"id"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description"`
	Platform        string                      `json:"platform"`
	Groups          []TemplateGroup             `json:"groups"`
	ComputeUnits    uint32                      `json:"compute_units"`
	StorageUnits    uint32                      `json:"storage_units"`
	ScheduleEnabled bool                        `json:"schedule_enabled"`
	IsManaged       bool                        `json:"is_managed"`
	Schedule        string                      `json:"schedule"`
	Zones           []string                    `json:"zones"`
	CustomFields    []model.TemplateCustomField `json:"custom_fields"`
	MaxUptime       uint32                      `json:"max_uptime"`
	MaxUptimeUnit   string                      `json:"max_uptime_unit"`
}

type TemplateGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func listTemplates(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)

	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		return nil, fmt.Errorf("Failed to get templates: %v", err)
	}

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
	var result []Template
	for _, template := range templates {
		if template.IsDeleted || !template.Active || (len(template.Groups) > 0 && !user.HasAnyGroup(&template.Groups)) || !template.IsValidForZone(cfg.Zone) {
			continue
		}

		// Build the groups list
		var groupsList []TemplateGroup
		for _, group := range template.Groups {
			if grp, ok := groupMap[group]; ok {
				groupsList = append(groupsList, TemplateGroup{ID: grp.Id, Name: grp.Name})
			}
		}

		result = append(result, Template{
			ID:              template.Id,
			Name:            template.Name,
			Description:     template.Description,
			Platform:        template.Platform,
			Groups:          groupsList,
			ComputeUnits:    template.ComputeUnits,
			StorageUnits:    template.StorageUnits,
			ScheduleEnabled: template.ScheduleEnabled,
			IsManaged:       template.IsManaged,
			Schedule:        fmt.Sprintf("%v", template.Schedule),
			Zones:           template.Zones,
			CustomFields:    template.CustomFields,
			MaxUptime:       template.MaxUptime,
			MaxUptimeUnit:   template.MaxUptimeUnit,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func createTemplate(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageTemplates) {
		return nil, fmt.Errorf("No permission to manage templates")
	}

	name := req.StringOr("name", "")
	if !validate.Required(name) || !validate.MaxLength(name, 64) {
		return nil, fmt.Errorf("Invalid template name given")
	}

	platform := req.StringOr("platform", "")
	if !validate.OneOf(platform, []string{model.PlatformManual, model.PlatformDocker, model.PlatformPodman, model.PlatformNomad}) {
		return nil, fmt.Errorf("Invalid platform")
	}

	job := req.StringOr("job", "")
	if platform != model.PlatformManual {
		if !validate.Required(job) || !validate.MaxLength(job, 10*1024*1024) {
			return nil, fmt.Errorf("Job is required and must be less than 10MB")
		}
	} else {
		job = ""
	}

	description := req.StringOr("description", "")
	volumes := req.StringOr("volumes", "")
	if !validate.MaxLength(volumes, 10*1024*1024) {
		return nil, fmt.Errorf("Volumes must be less than 10MB")
	}

	computeUnits := req.IntOr("compute_units", 0)
	if !validate.IsPositiveNumber(computeUnits) {
		return nil, fmt.Errorf("Compute units must be a positive number")
	}

	storageUnits := req.IntOr("storage_units", 0)
	if !validate.IsPositiveNumber(storageUnits) {
		return nil, fmt.Errorf("Storage units must be a positive number")
	}

	withTerminal := req.BoolOr("with_terminal", false)
	withVSCodeTunnel := req.BoolOr("with_vscode_tunnel", false)
	withCodeServer := req.BoolOr("with_code_server", false)
	withSSH := req.BoolOr("with_ssh", false)
	active := req.BoolOr("active", true)

	groups := []string{}
	if g, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
		db := database.GetInstance()
		for _, groupId := range g {
			if _, err := db.GetGroup(groupId); err == nil {
				groups = append(groups, groupId)
			}
		}
	}

	template := model.NewTemplate(
		name,
		description,
		job,
		volumes,
		user.Id,
		groups,
		platform,
		withTerminal,
		withVSCodeTunnel,
		withCodeServer,
		withSSH,
		uint32(computeUnits),
		uint32(storageUnits),
		false,      // schedule disabled by default
		nil,        // no schedule
		[]string{}, // no zones
		false,      // auto start disabled
		active,
		0,          // max uptime disabled
		"disabled", // max uptime unit
		req.StringOr("icon_url", ""),
		[]model.TemplateCustomField{}, // no custom fields
	)

	err := database.GetInstance().SaveTemplate(template, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to save template: %v", err)
	}

	service.GetTransport().GossipTemplate(template)

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventTemplateCreate,
		fmt.Sprintf("Created template %s", template.Name),
		&map[string]interface{}{
			"template_id":   template.Id,
			"template_name": template.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
		"id":     template.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateTemplate(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageTemplates) {
		return nil, fmt.Errorf("No permission to manage templates")
	}

	templateId := req.StringOr("template_id", "")
	if !validate.UUID(templateId) {
		return nil, fmt.Errorf("Invalid template ID")
	}

	db := database.GetInstance()
	template, err := db.GetTemplate(templateId)
	if err != nil {
		return nil, fmt.Errorf("Template not found: %v", err)
	}

	if template.IsManaged {
		return nil, fmt.Errorf("Cannot update managed template")
	}

	// Update name if provided
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		if !validate.Required(name) || !validate.MaxLength(name, 64) {
			return nil, fmt.Errorf("Invalid template name given")
		}
		template.Name = name
	}

	// Update description if provided
	if description, err := req.String("description"); err != mcp.ErrUnknownParameter {
		template.Description = description
	}

	// Update job if provided
	if job, err := req.String("job"); err != mcp.ErrUnknownParameter {
		if template.Platform != model.PlatformManual {
			if !validate.Required(job) || !validate.MaxLength(job, 10*1024*1024) {
				return nil, fmt.Errorf("Job is required and must be less than 10MB")
			}
		} else {
			job = ""
		}
		template.Job = job
	}

	// Update volumes if provided
	if volumes, err := req.String("volumes"); err != mcp.ErrUnknownParameter {
		if !validate.MaxLength(volumes, 10*1024*1024) {
			return nil, fmt.Errorf("Volumes must be less than 10MB")
		}
		template.Volumes = volumes
	}

	// Update features if provided
	if withTerminal, err := req.Bool("with_terminal"); err != mcp.ErrUnknownParameter {
		template.WithTerminal = withTerminal
	}
	if withVSCodeTunnel, err := req.Bool("with_vscode_tunnel"); err != mcp.ErrUnknownParameter {
		template.WithVSCodeTunnel = withVSCodeTunnel
	}
	if withCodeServer, err := req.Bool("with_code_server"); err != mcp.ErrUnknownParameter {
		template.WithCodeServer = withCodeServer
	}
	if withSSH, err := req.Bool("with_ssh"); err != mcp.ErrUnknownParameter {
		template.WithSSH = withSSH
	}

	// Update resource limits if provided
	if computeUnits, err := req.Int("compute_units"); err != mcp.ErrUnknownParameter {
		if !validate.IsPositiveNumber(computeUnits) {
			return nil, fmt.Errorf("Compute units must be a positive number")
		}
		template.ComputeUnits = uint32(computeUnits)
	}

	if storageUnits, err := req.Int("storage_units"); err != mcp.ErrUnknownParameter {
		if !validate.IsPositiveNumber(storageUnits) {
			return nil, fmt.Errorf("Storage units must be a positive number")
		}
		template.StorageUnits = uint32(storageUnits)
	}

	// Update active status if provided
	if active, err := req.Bool("active"); err != mcp.ErrUnknownParameter {
		template.Active = active
	}

	// Handle group operations
	if action, err := req.String("group_action"); err != mcp.ErrUnknownParameter {
		switch action {
		case "replace":
			if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
				template.Groups = []string{}
				for _, groupId := range groups {
					if _, err := db.GetGroup(groupId); err == nil {
						template.Groups = append(template.Groups, groupId)
					}
				}
			}
		case "add":
			if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
				for _, groupId := range groups {
					if _, err := db.GetGroup(groupId); err == nil {
						exists := false
						for _, existing := range template.Groups {
							if existing == groupId {
								exists = true
								break
							}
						}
						if !exists {
							template.Groups = append(template.Groups, groupId)
						}
					}
				}
			}
		case "remove":
			if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
				for _, groupId := range groups {
					newGroups := []string{}
					for _, existing := range template.Groups {
						if existing != groupId {
							newGroups = append(newGroups, existing)
						}
					}
					template.Groups = newGroups
				}
			}
		default:
			return nil, fmt.Errorf("Invalid group_action. Must be 'replace', 'add', or 'remove'")
		}
	}

	template.UpdatedUserId = user.Id
	template.UpdatedAt = hlc.Now()
	template.UpdateHash()

	err = db.SaveTemplate(template, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to save template: %v", err)
	}

	service.GetTransport().GossipTemplate(template)

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventTemplateUpdate,
		fmt.Sprintf("Updated template %s", template.Name),
		&map[string]interface{}{
			"template_id":   template.Id,
			"template_name": template.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteTemplate(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageTemplates) {
		return nil, fmt.Errorf("No permission to manage templates")
	}

	templateId := req.StringOr("template_id", "")
	if !validate.UUID(templateId) {
		return nil, fmt.Errorf("Invalid template ID")
	}

	db := database.GetInstance()
	template, err := db.GetTemplate(templateId)
	if err != nil {
		return nil, fmt.Errorf("Template not found: %v", err)
	}

	// Check if template is in use
	spaces, err := db.GetSpacesByTemplateId(templateId)
	if err != nil {
		return nil, fmt.Errorf("Failed to check template usage: %v", err)
	}

	activeSpaces := 0
	for _, space := range spaces {
		if !space.IsDeleted {
			activeSpaces++
		}
	}

	if activeSpaces > 0 {
		return nil, fmt.Errorf("Template is in use by spaces")
	}

	template.IsDeleted = true
	template.UpdatedAt = hlc.Now()
	template.UpdatedUserId = user.Id
	err = db.SaveTemplate(template, []string{"IsDeleted", "UpdatedAt", "UpdatedUserId"})
	if err != nil {
		if errors.Is(err, database.ErrTemplateInUse) {
			return nil, fmt.Errorf("Template is in use")
		}
		return nil, fmt.Errorf("Failed to delete template: %v", err)
	}

	service.GetTransport().GossipTemplate(template)

	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventTemplateDelete,
		fmt.Sprintf("Deleted template %s", template.Name),
		&map[string]interface{}{
			"template_id":   template.Id,
			"template_name": template.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}
