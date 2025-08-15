package api_utils

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

// GetTemplateDetails returns detailed template information with permission checks
func GetTemplateDetails(templateId string, user *model.User) (*apiclient.TemplateDetails, error) {
	if templateId == "" {
		return nil, fmt.Errorf("template_id is required")
	}

	templateService := service.GetTemplateService()
	template, err := templateService.GetTemplate(templateId)
	if err != nil {
		return nil, fmt.Errorf("Template not found: %v", err)
	}

	// Check if user has permission to view this template
	if !user.HasPermission(model.PermissionManageTemplates) {
		// For non-admin users, check if they can use this template
		templates, err := templateService.ListTemplates(service.TemplateListOptions{
			User:                 user,
			IncludeInactive:      false,
			IncludeDeleted:       false,
			CheckPermissions:     true,
			CheckZoneRestriction: true,
		})
		if err != nil {
			return nil, fmt.Errorf("Failed to check template permissions: %v", err)
		}

		// Check if this template is in the user's accessible templates
		hasAccess := false
		for _, t := range templates {
			if t.Id == templateId {
				hasAccess = true
				break
			}
		}
		if !hasAccess {
			return nil, fmt.Errorf("No permission to access this template")
		}
	}

	// Get template usage
	total, deployed, err := templateService.GetTemplateUsage(templateId)
	if err != nil {
		return nil, fmt.Errorf("Failed to get template usage: %v", err)
	}

	data := &apiclient.TemplateDetails{
		Name:             template.Name,
		Description:      template.Description,
		Job:              template.Job,
		Volumes:          template.Volumes,
		Usage:            total,
		Hash:             template.Hash,
		Deployed:         deployed,
		Groups:           template.Groups,
		Zones:            template.Zones,
		Platform:         template.Platform,
		IsManaged:        template.IsManaged,
		WithTerminal:     template.WithTerminal,
		WithVSCodeTunnel: template.WithVSCodeTunnel,
		WithCodeServer:   template.WithCodeServer,
		WithSSH:          template.WithSSH,
		WithRunCommand:   template.WithRunCommand,
		ScheduleEnabled:  template.ScheduleEnabled,
		AutoStart:        template.AutoStart,
		Schedule:         make([]apiclient.TemplateDetailsDay, 7),
		ComputeUnits:     template.ComputeUnits,
		StorageUnits:     template.StorageUnits,
		Active:           template.Active,
		MaxUptime:        template.MaxUptime,
		MaxUptimeUnit:    template.MaxUptimeUnit,
		IconURL:          template.IconURL,
		CustomFields:     make([]apiclient.CustomFieldDef, len(template.CustomFields)),
	}

	// Handle schedule
	if len(template.Schedule) != 7 {
		for i := 0; i < 7; i++ {
			data.Schedule[i] = apiclient.TemplateDetailsDay{
				Enabled: false,
				From:    "12:00am",
				To:      "11:59pm",
			}
		}
	} else {
		for i, day := range template.Schedule {
			data.Schedule[i] = apiclient.TemplateDetailsDay{
				Enabled: day.Enabled,
				From:    day.From,
				To:      day.To,
			}
		}
	}

	// Handle custom fields
	for i, field := range template.CustomFields {
		data.CustomFields[i] = apiclient.CustomFieldDef{
			Name:        field.Name,
			Description: field.Description,
		}
	}

	return data, nil
}
