package mcp

import (
	"context"
	"fmt"
	"slices"

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
		req.BoolOr("with_run_command", false),
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
		parseCustomFields(req),
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
	if runCmd, err := req.Bool("with_run_command"); err != mcp.ErrUnknownParameter {
		template.WithRunCommand = runCmd
	}

	// Handle zone operations - this is MCP-specific logic
	if action, err := req.String("zone_action"); err != mcp.ErrUnknownParameter {
		switch action {
		case "replace":
			if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
				template.Zones = zones
			}
		case "add":
			if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
				for _, zone := range zones {
					if !slices.Contains(template.Zones, zone) {
						template.Zones = append(template.Zones, zone)
					}
				}
			}
		case "remove":
			if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
				newZones := []string{}
				for _, existing := range template.Zones {
					if !slices.Contains(zones, existing) {
						newZones = append(newZones, existing)
					}
				}
				template.Zones = newZones
			}
		default:
			return nil, fmt.Errorf("Invalid zone_action. Must be 'replace', 'add', or 'remove'")
		}
	} else {
		// Fallback to old behavior for backward compatibility
		if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
			template.Zones = zones
		}
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
						if !slices.Contains(template.Groups, groupId) {
							template.Groups = append(template.Groups, groupId)
						}
					}
				}
			}
		case "remove":
			if groups, err := req.StringSlice("groups"); err != mcp.ErrUnknownParameter {
				newGroups := []string{}
				for _, existing := range template.Groups {
					if !slices.Contains(groups, existing) {
						newGroups = append(newGroups, existing)
					}
				}
				template.Groups = newGroups
			}
		default:
			return nil, fmt.Errorf("Invalid group_action. Must be 'replace', 'add', or 'remove'")
		}
	}

	// Handle custom field operations - this is MCP-specific logic
	if action, err := req.String("custom_field_action"); err != mcp.ErrUnknownParameter {
		switch action {
		case "replace":
			template.CustomFields = parseCustomFields(req)
		case "add":
			if fields := parseCustomFields(req); len(fields) > 0 {
				for _, newField := range fields {
					// Check if field already exists
					exists := false
					for i, existing := range template.CustomFields {
						if existing.Name == newField.Name {
							// Update existing field
							template.CustomFields[i] = newField
							exists = true
							break
						}
					}
					if !exists {
						template.CustomFields = append(template.CustomFields, newField)
					}
				}
			}
		case "remove":
			if fieldsToRemove := parseCustomFields(req); len(fieldsToRemove) > 0 {
				// Extract field names to remove
				removeNames := make([]string, len(fieldsToRemove))
				for i, field := range fieldsToRemove {
					removeNames[i] = field.Name
				}

				// Filter out fields to remove
				newFields := []model.TemplateCustomField{}
				for _, existing := range template.CustomFields {
					if !slices.Contains(removeNames, existing.Name) {
						newFields = append(newFields, existing)
					}
				}
				template.CustomFields = newFields
			}
		default:
			return nil, fmt.Errorf("Invalid custom_field_action. Must be 'replace', 'add', or 'remove'")
		}
	} else {
		// Fallback to old behavior for backward compatibility
		if fields := parseCustomFields(req); len(fields) > 0 {
			template.CustomFields = fields
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

// parseCustomFields extracts custom fields from the MCP request
// Expects custom_fields as an array of objects with name and description properties
func parseCustomFields(req *mcp.ToolRequest) []model.TemplateCustomField {
	var fields []model.TemplateCustomField

	// Get array of custom field objects
	customFields, err := req.ObjectSlice("custom_fields")
	if err != mcp.ErrUnknownParameter {
		for _, fieldObj := range customFields {
			field := model.TemplateCustomField{}

			// Extract name (required)
			if name, ok := fieldObj["name"].(string); ok && name != "" {
				field.Name = name
			} else {
				continue // Skip fields without valid names
			}

			// Extract description (optional)
			if description, ok := fieldObj["description"].(string); ok {
				field.Description = description
			}

			fields = append(fields, field)
		}
	}

	return fields
}
