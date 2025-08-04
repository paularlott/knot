package mcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/mcp"
)

type TemplateVar struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Zones      []string `json:"zones"`
	Local      bool     `json:"local"`
	Protected  bool     `json:"protected"`
	Restricted bool     `json:"restricted"`
	IsManaged  bool     `json:"is_managed"`
}

func listTemplateVars(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageVariables) {
		return nil, fmt.Errorf("No permission to manage variables")
	}

	templateVars, err := database.GetInstance().GetTemplateVars()
	if err != nil {
		return nil, fmt.Errorf("Failed to get template variables: %v", err)
	}

	var result []TemplateVar
	for _, variable := range templateVars {
		if variable.IsDeleted {
			continue
		}

		result = append(result, TemplateVar{
			ID:         variable.Id,
			Name:       variable.Name,
			Zones:      variable.Zones,
			Local:      variable.Local,
			Protected:  variable.Protected,
			Restricted: variable.Restricted,
			IsManaged:  variable.IsManaged,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func cleanZones(zones []string) ([]string, error) {
	zoneSet := make(map[string]struct{})
	cleanZones := make([]string, 0, len(zones))
	for _, zone := range zones {
		zone = strings.Trim(zone, " \r\n")
		if zone == "" {
			continue
		}
		if len(zone) > 64 {
			return nil, fmt.Errorf("zone '%s' exceeds maximum length of 64", zone)
		}
		if _, exists := zoneSet[zone]; !exists {
			zoneSet[zone] = struct{}{}
			cleanZones = append(cleanZones, zone)
		}
	}
	cleanZones = slices.Clip(cleanZones)
	return cleanZones, nil
}

func createTemplateVar(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageVariables) {
		return nil, fmt.Errorf("No permission to manage variables")
	}

	name := req.StringOr("name", "")
	if !validate.Required(name) || !validate.VarName(name) {
		return nil, fmt.Errorf("Invalid template variable name given")
	}

	value := req.StringOr("value", "")
	if !validate.MaxLength(value, 10*1024*1024) {
		return nil, fmt.Errorf("Value must be less than 10MB")
	}

	var zones []string
	if z, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
		zones = z
	}

	cleanedZones, err := cleanZones(zones)
	if err != nil {
		return nil, fmt.Errorf("Invalid zones: %v", err)
	}

	local := req.BoolOr("local", false)
	protected := req.BoolOr("protected", false)
	restricted := req.BoolOr("restricted", false)

	templateVar := model.NewTemplateVar(name, cleanedZones, local, value, protected, restricted, user.Id)

	err = database.GetInstance().SaveTemplateVar(templateVar)
	if err != nil {
		return nil, fmt.Errorf("Failed to save template variable: %v", err)
	}

	service.GetTransport().GossipTemplateVar(templateVar)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVarCreate,
		fmt.Sprintf("Created variable %s", templateVar.Name),
		&map[string]interface{}{
			"var_id":   templateVar.Id,
			"var_name": templateVar.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
		"id":     templateVar.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateTemplateVar(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageVariables) {
		return nil, fmt.Errorf("No permission to manage variables")
	}

	templateVarId := req.StringOr("templatevar_id", "")
	if !validate.UUID(templateVarId) {
		return nil, fmt.Errorf("Invalid variable ID")
	}

	db := database.GetInstance()
	templateVar, err := db.GetTemplateVar(templateVarId)
	if err != nil {
		return nil, fmt.Errorf("Template variable not found: %v", err)
	}

	// Update name if provided
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		if !validate.Required(name) || !validate.VarName(name) {
			return nil, fmt.Errorf("Invalid template variable name given")
		}
		templateVar.Name = name
	}

	// Update value if provided
	if value, err := req.String("value"); err != mcp.ErrUnknownParameter {
		if !validate.MaxLength(value, 10*1024*1024) {
			return nil, fmt.Errorf("Value must be less than 10MB")
		}
		templateVar.Value = value
	}

	// Update zones if provided
	if zones, err := req.StringSlice("zones"); err != mcp.ErrUnknownParameter {
		cleanedZones, err := cleanZones(zones)
		if err != nil {
			return nil, fmt.Errorf("Invalid zones: %v", err)
		}
		templateVar.Zones = cleanedZones
	}

	// Update flags if provided
	if local, err := req.Bool("local"); err != mcp.ErrUnknownParameter {
		templateVar.Local = local
	}
	if protected, err := req.Bool("protected"); err != mcp.ErrUnknownParameter {
		templateVar.Protected = protected
	}
	if restricted, err := req.Bool("restricted"); err != mcp.ErrUnknownParameter {
		templateVar.Restricted = restricted
	}

	templateVar.UpdatedUserId = user.Id
	templateVar.UpdatedAt = hlc.Now()

	err = db.SaveTemplateVar(templateVar)
	if err != nil {
		return nil, fmt.Errorf("Failed to save template variable: %v", err)
	}

	service.GetTransport().GossipTemplateVar(templateVar)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVarUpdate,
		fmt.Sprintf("Updated variable %s", templateVar.Name),
		&map[string]interface{}{
			"var_id":   templateVar.Id,
			"var_name": templateVar.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteTemplateVar(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageVariables) {
		return nil, fmt.Errorf("No permission to manage variables")
	}

	templateVarId := req.StringOr("templatevar_id", "")
	if !validate.UUID(templateVarId) {
		return nil, fmt.Errorf("Invalid variable ID")
	}

	db := database.GetInstance()
	templateVar, err := db.GetTemplateVar(templateVarId)
	if err != nil {
		return nil, fmt.Errorf("Template variable not found: %v", err)
	}

	templateVar.IsDeleted = true
	templateVar.UpdatedAt = hlc.Now()
	templateVar.UpdatedUserId = user.Id
	err = db.SaveTemplateVar(templateVar)
	if err != nil {
		return nil, fmt.Errorf("Failed to delete template variable: %v", err)
	}

	service.GetTransport().GossipTemplateVar(templateVar)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventVarDelete,
		fmt.Sprintf("Deleted variable %s", templateVar.Name),
		&map[string]interface{}{
			"var_id":   templateVar.Id,
			"var_name": templateVar.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}