package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"

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

	templateService := service.GetTemplateService()
	templates, err := templateService.ListTemplates(service.TemplateListOptions{
		User:                 user,
		IncludeInactive:      false,
		IncludeDeleted:       false,
		CheckPermissions:     true,
		CheckZoneRestriction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("Failed to get templates: %v", err)
	}

	// Load the groups so we can look up their names & convert to map id => group
	db := database.GetInstance()
	groups, err := db.GetGroups()
	if err != nil {
		return nil, fmt.Errorf("Failed to get groups: %v", err)
	}
	groupMap := make(map[string]*model.Group)
	for _, group := range groups {
		groupMap[group.Id] = group
	}

	var result []Template
	for _, template := range templates {
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

	template := model.NewTemplate(
		req.StringOr("name", ""),
		req.StringOr("description", ""),
		req.StringOr("job", ""),
		req.StringOr("volumes", ""),
		user.Id,
		req.StringSliceOr("groups", []string{}),
		req.StringOr("platform", ""),
		req.BoolOr("with_terminal", false),
		req.BoolOr("with_vscode_tunnel", false),
		req.BoolOr("with_code_server", false),
		req.BoolOr("with_ssh", false),
		uint32(req.IntOr("compute_units", 0)),
		uint32(req.IntOr("storage_units", 0)),
		false, // schedule disabled by default
		nil,   // no schedule
		req.StringSliceOr("zones", []string{}),
		false, // auto start disabled
		req.BoolOr("active", true),
		0,          // max uptime disabled
		"disabled", // max uptime unit
		req.StringOr("icon_url", ""),
		[]model.TemplateCustomField{}, // no custom fields
	)

	templateService := service.GetTemplateService()
	err := templateService.CreateTemplate(template, user)
	if err != nil {
		return nil, err
	}

	// Audit log
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
	templateId := req.StringOr("template_id", "")

	templateService := service.GetTemplateService()
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		return nil, fmt.Errorf("Template not found: %v", err)
	}

	// Apply updates based on provided parameters
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		template.Name = name
	}
	if description, err := req.String("description"); err != mcp.ErrUnknownParameter {
		template.Description = description
	}
	if job, err := req.String("job"); err != mcp.ErrUnknownParameter {
		template.Job = job
	}
	if volumes, err := req.String("volumes"); err != mcp.ErrUnknownParameter {
		template.Volumes = volumes
	}
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
	if computeUnits, err := req.Int("compute_units"); err != mcp.ErrUnknownParameter {
		template.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, err := req.Int("storage_units"); err != mcp.ErrUnknownParameter {
		template.StorageUnits = uint32(storageUnits)
	}
	if active, err := req.Bool("active"); err != mcp.ErrUnknownParameter {
		template.Active = active
	}
	if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
		template.Zones = zones
	}

	// Handle group operations - this is MCP-specific logic
	if action, err := req.String("group_action"); err != mcp.ErrUnknownParameter {
		db := database.GetInstance()
		switch action {
		case "replace":
			if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
				validGroups := []string{}
				for _, groupId := range groups {
					if _, err := db.GetGroup(groupId); err == nil {
						validGroups = append(validGroups, groupId)
					}
				}
				template.Groups = validGroups
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
				newGroups := []string{}
				for _, existing := range template.Groups {
					shouldRemove := false
					for _, groupId := range groups {
						if existing == groupId {
							shouldRemove = true
							break
						}
					}
					if !shouldRemove {
						newGroups = append(newGroups, existing)
					}
				}
				template.Groups = newGroups
			}
		default:
			return nil, fmt.Errorf("Invalid group_action. Must be 'replace', 'add', or 'remove'")
		}
	}

	err = templateService.UpdateTemplate(template, user)
	if err != nil {
		return nil, err
	}

	// Audit log
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
	templateId := req.StringOr("template_id", "")

	templateService := service.GetTemplateService()

	// Get template name for audit log before deletion
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		return nil, err
	}
	templateName := template.Name

	err = templateService.DeleteTemplate(templateId, user)
	if err != nil {
		return nil, err
	}

	// Audit log
	audit.Log(
		user.Username,
		model.AuditActorTypeMCP,
		model.AuditEventTemplateDelete,
		fmt.Sprintf("Deleted template %s", templateName),
		&map[string]interface{}{
			"template_id":   templateId,
			"template_name": templateName,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}
