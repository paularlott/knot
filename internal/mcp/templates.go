package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

type Template struct {
	ID              string                        `json:"id"`
	Name            string                        `json:"name"`
	Description     string                        `json:"description"`
	Platform        string                        `json:"platform"`
	Groups          []string                      `json:"groups"`
	ComputeUnits    uint32                        `json:"compute_units"`
	StorageUnits    uint32                        `json:"storage_units"`
	ScheduleEnabled bool                          `json:"schedule_enabled"`
	IsManaged       bool                          `json:"is_managed"`
	Schedule        string                        `json:"schedule"`
	Zones           []string                      `json:"zones"`
	CustomFields    []model.TemplateCustomField   `json:"custom_fields"`
	MaxUptime       uint32                        `json:"max_uptime"`
	MaxUptimeUnit   string                        `json:"max_uptime_unit"`
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
		var groupsList []string
		for _, group := range template.Groups {
			if grp, ok := groupMap[group]; ok {
				groupsList = append(groupsList, fmt.Sprintf("%s (ID: %s)", grp.Name, grp.Id))
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