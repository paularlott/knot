package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetTemplatesLibrary returns the templates management library for scriptling
func GetTemplatesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.template", "Knot template management functions")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateList(ctx, client)
	}, "list() - List all templates")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateGet(ctx, client, args...)
	}, "get(template_id) - Get template by ID or name")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateCreate(ctx, client, kwargs, args...)
	}, "create(name, ...) - Create a new template")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateUpdate(ctx, client, kwargs, args...)
	}, "update(template_id, ...) - Update template properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateDelete(ctx, client, args...)
	}, "delete(template_id) - Delete a template")

	builder.FunctionWithHelp("get_icons", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateGetIcons(ctx, client)
	}, "get_icons() - Get list of available icons")

	return builder.Build()
}

// templateList returns all templates
func templateList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Templates not available - API client not configured"}
	}

	templates, _, err := client.GetTemplates(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list templates: %v", err)}
	}

	elements := make([]object.Object, len(templates.Templates))
	for i, tmpl := range templates.Templates {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: tmpl.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: tmpl.Name}}
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: tmpl.Description}}
		pairs["platform"] = object.DictPair{Key: &object.String{Value: "platform"}, Value: &object.String{Value: tmpl.Platform}}
		pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: tmpl.Active}}
		pairs["usage"] = object.DictPair{Key: &object.String{Value: "usage"}, Value: &object.Integer{Value: int64(tmpl.Usage)}}
		pairs["deployed"] = object.DictPair{Key: &object.String{Value: "deployed"}, Value: &object.Integer{Value: int64(tmpl.Deployed)}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// templateGet returns template by ID or name
func templateGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	templateId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("template_id", err)
	}

	template, _, apiErr := client.GetTemplate(ctx, templateId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: template.TemplateId}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: template.Name}}
	pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: template.Description}}
	pairs["platform"] = object.DictPair{Key: &object.String{Value: "platform"}, Value: &object.String{Value: template.Platform}}
	pairs["job"] = object.DictPair{Key: &object.String{Value: "job"}, Value: &object.String{Value: template.Job}}
	pairs["volumes"] = object.DictPair{Key: &object.String{Value: "volumes"}, Value: &object.String{Value: template.Volumes}}
	pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: template.Active}}
	pairs["is_managed"] = object.DictPair{Key: &object.String{Value: "is_managed"}, Value: &object.Boolean{Value: template.IsManaged}}
	pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(template.ComputeUnits)}}
	pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(template.StorageUnits)}}
	pairs["usage"] = object.DictPair{Key: &object.String{Value: "usage"}, Value: &object.Integer{Value: int64(template.Usage)}}
	pairs["deployed"] = object.DictPair{Key: &object.String{Value: "deployed"}, Value: &object.Integer{Value: int64(template.Deployed)}}
	pairs["hash"] = object.DictPair{Key: &object.String{Value: "hash"}, Value: &object.String{Value: template.Hash}}
	pairs["with_terminal"] = object.DictPair{Key: &object.String{Value: "with_terminal"}, Value: &object.Boolean{Value: template.WithTerminal}}
	pairs["with_vscode_tunnel"] = object.DictPair{Key: &object.String{Value: "with_vscode_tunnel"}, Value: &object.Boolean{Value: template.WithVSCodeTunnel}}
	pairs["with_code_server"] = object.DictPair{Key: &object.String{Value: "with_code_server"}, Value: &object.Boolean{Value: template.WithCodeServer}}
	pairs["with_ssh"] = object.DictPair{Key: &object.String{Value: "with_ssh"}, Value: &object.Boolean{Value: template.WithSSH}}
	pairs["with_run_command"] = object.DictPair{Key: &object.String{Value: "with_run_command"}, Value: &object.Boolean{Value: template.WithRunCommand}}
	pairs["schedule_enabled"] = object.DictPair{Key: &object.String{Value: "schedule_enabled"}, Value: &object.Boolean{Value: template.ScheduleEnabled}}
	pairs["auto_start"] = object.DictPair{Key: &object.String{Value: "auto_start"}, Value: &object.Boolean{Value: template.AutoStart}}
	pairs["max_uptime"] = object.DictPair{Key: &object.String{Value: "max_uptime"}, Value: &object.Integer{Value: int64(template.MaxUptime)}}
	pairs["max_uptime_unit"] = object.DictPair{Key: &object.String{Value: "max_uptime_unit"}, Value: &object.String{Value: template.MaxUptimeUnit}}
	pairs["icon_url"] = object.DictPair{Key: &object.String{Value: "icon_url"}, Value: &object.String{Value: template.IconURL}}

	// Groups
	groupElements := make([]object.Object, len(template.Groups))
	for i, g := range template.Groups {
		groupElements[i] = &object.String{Value: g}
	}
	pairs["groups"] = object.DictPair{Key: &object.String{Value: "groups"}, Value: &object.List{Elements: groupElements}}

	// Zones
	zoneElements := make([]object.Object, len(template.Zones))
	for i, z := range template.Zones {
		zoneElements[i] = &object.String{Value: z}
	}
	pairs["zones"] = object.DictPair{Key: &object.String{Value: "zones"}, Value: &object.List{Elements: zoneElements}}

	// Schedule
	scheduleElements := make([]object.Object, len(template.Schedule))
	for i, day := range template.Schedule {
		dayPairs := make(map[string]object.DictPair)
		dayPairs["enabled"] = object.DictPair{Key: &object.String{Value: "enabled"}, Value: &object.Boolean{Value: day.Enabled}}
		dayPairs["from"] = object.DictPair{Key: &object.String{Value: "from"}, Value: &object.String{Value: day.From}}
		dayPairs["to"] = object.DictPair{Key: &object.String{Value: "to"}, Value: &object.String{Value: day.To}}
		scheduleElements[i] = &object.Dict{Pairs: dayPairs}
	}
	pairs["schedule"] = object.DictPair{Key: &object.String{Value: "schedule"}, Value: &object.List{Elements: scheduleElements}}

	// Custom fields
	customFieldElements := make([]object.Object, len(template.CustomFields))
	for i, cf := range template.CustomFields {
		cfPairs := make(map[string]object.DictPair)
		cfPairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: cf.Name}}
		cfPairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: cf.Description}}
		customFieldElements[i] = &object.Dict{Pairs: cfPairs}
	}
	pairs["custom_fields"] = object.DictPair{Key: &object.String{Value: "custom_fields"}, Value: &object.List{Elements: customFieldElements}}

	return &object.Dict{Pairs: pairs}
}

// templateDelete deletes a template
func templateDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	templateId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("template_id", err)
	}

	_, apiErr := client.DeleteTemplate(ctx, templateId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete template: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// templateCreate creates a new template
func templateCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	request := &apiclient.TemplateCreateRequest{
		Name:         name,
		Job:          "",
		Description:  "",
		Platform:     "",
		Active:       true,
		Volumes:      "",
		Groups:       []string{},
		Zones:        []string{},
		Schedule:     []apiclient.TemplateDetailsDay{},
		CustomFields: []apiclient.CustomFieldDef{},
	}

	// Optional parameters via kwargs
	if job, errObj := kwargs.GetString("job", ""); errObj == nil {
		request.Job = job
	}
	if description, errObj := kwargs.GetString("description", ""); errObj == nil {
		request.Description = description
	}
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil {
		request.Platform = platform
	}
	if volumes, errObj := kwargs.GetString("volumes", ""); errObj == nil {
		request.Volumes = volumes
	}
	if active, errObj := kwargs.GetBool("active", true); errObj == nil {
		request.Active = active
	}
	if computeUnits, errObj := kwargs.GetInt("compute_units", 0); errObj == nil {
		request.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, errObj := kwargs.GetInt("storage_units", 0); errObj == nil {
		request.StorageUnits = uint32(storageUnits)
	}
	if withTerminal, errObj := kwargs.GetBool("with_terminal", false); errObj == nil {
		request.WithTerminal = withTerminal
	}
	if withVSCodeTunnel, errObj := kwargs.GetBool("with_vscode_tunnel", false); errObj == nil {
		request.WithVSCodeTunnel = withVSCodeTunnel
	}
	if withCodeServer, errObj := kwargs.GetBool("with_code_server", false); errObj == nil {
		request.WithCodeServer = withCodeServer
	}
	if withSSH, errObj := kwargs.GetBool("with_ssh", false); errObj == nil {
		request.WithSSH = withSSH
	}
	if withRunCommand, errObj := kwargs.GetBool("with_run_command", false); errObj == nil {
		request.WithRunCommand = withRunCommand
	}
	if scheduleEnabled, errObj := kwargs.GetBool("schedule_enabled", false); errObj == nil {
		request.ScheduleEnabled = scheduleEnabled
	}
	if iconURL, errObj := kwargs.GetString("icon_url", ""); errObj == nil {
		request.IconURL = iconURL
	}
	if groups, errObj := kwargs.GetList("groups", []object.Object{}); errObj == nil {
		groupStrs := []string{}
		for _, g := range groups {
			if gStr, err := g.AsString(); err == nil {
				groupStrs = append(groupStrs, gStr)
			}
		}
		request.Groups = groupStrs
	}
	if zones, errObj := kwargs.GetList("zones", []object.Object{}); errObj == nil {
		zoneStrs := []string{}
		for _, z := range zones {
			if zStr, err := z.AsString(); err == nil {
				zoneStrs = append(zoneStrs, zStr)
			}
		}
		request.Zones = zoneStrs
	}
	if schedule, errObj := kwargs.GetList("schedule", []object.Object{}); errObj == nil {
		schedDays := []apiclient.TemplateDetailsDay{}
		for _, day := range schedule {
			if dayDict, ok := day.(*object.Dict); ok {
				schedDay := apiclient.TemplateDetailsDay{}
				if enabled, ok := dayDict.Pairs["enabled"]; ok {
					if enabledBool, err := enabled.Value.AsBool(); err == nil {
						schedDay.Enabled = enabledBool
					}
				}
				if from, ok := dayDict.Pairs["from"]; ok {
					if fromStr, err := from.Value.AsString(); err == nil {
						schedDay.From = fromStr
					}
				}
				if to, ok := dayDict.Pairs["to"]; ok {
					if toStr, err := to.Value.AsString(); err == nil {
						schedDay.To = toStr
					}
				}
				schedDays = append(schedDays, schedDay)
			}
		}
		request.Schedule = schedDays
	}
	if customFields, errObj := kwargs.GetList("custom_fields", []object.Object{}); errObj == nil {
		fields := []apiclient.CustomFieldDef{}
		for _, field := range customFields {
			if fieldDict, ok := field.(*object.Dict); ok {
				cf := apiclient.CustomFieldDef{}
				if name, ok := fieldDict.Pairs["name"]; ok {
					if nameStr, err := name.Value.AsString(); err == nil {
						cf.Name = nameStr
					}
				}
				if desc, ok := fieldDict.Pairs["description"]; ok {
					if descStr, err := desc.Value.AsString(); err == nil {
						cf.Description = descStr
					}
				}
				if cf.Name != "" {
					fields = append(fields, cf)
				}
			}
		}
		request.CustomFields = fields
	}

	templateId, _, apiErr := client.CreateTemplate(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create template: %v", apiErr)}
	}

	return &object.String{Value: templateId}
}

// templateUpdate updates a template
func templateUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	templateId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("template_id", err)
	}

	// Get current template to build request
	template, _, apiErr := client.GetTemplate(ctx, templateId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", apiErr)}
	}

	request := &apiclient.TemplateUpdateRequest{
		Name:             template.Name,
		Job:              template.Job,
		Description:      template.Description,
		Volumes:          template.Volumes,
		Groups:           template.Groups,
		Active:           template.Active,
		Platform:         template.Platform,
		WithTerminal:     template.WithTerminal,
		WithVSCodeTunnel: template.WithVSCodeTunnel,
		WithCodeServer:   template.WithCodeServer,
		WithSSH:          template.WithSSH,
		WithRunCommand:   template.WithRunCommand,
		StartupScriptId:  template.StartupScriptId,
		ShutdownScriptId: template.ShutdownScriptId,
		ScheduleEnabled:  template.ScheduleEnabled,
		AutoStart:        template.AutoStart,
		Schedule:         template.Schedule,
		ComputeUnits:     template.ComputeUnits,
		StorageUnits:     template.StorageUnits,
		Zones:            template.Zones,
		MaxUptime:        template.MaxUptime,
		MaxUptimeUnit:    template.MaxUptimeUnit,
		IconURL:          template.IconURL,
		CustomFields:     template.CustomFields,
	}

	// Update with provided kwargs
	if name, errObj := kwargs.GetString("name", ""); errObj == nil && name != "" {
		request.Name = name
	}
	if job, errObj := kwargs.GetString("job", ""); errObj == nil && job != "" {
		request.Job = job
	}
	if description, errObj := kwargs.GetString("description", ""); errObj == nil {
		request.Description = description
	}
	if volumes, errObj := kwargs.GetString("volumes", ""); errObj == nil {
		request.Volumes = volumes
	}
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil && platform != "" {
		request.Platform = platform
	}
	if active, errObj := kwargs.GetBool("active", template.Active); errObj == nil {
		request.Active = active
	}
	if computeUnits, errObj := kwargs.GetInt("compute_units", 0); errObj == nil {
		request.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, errObj := kwargs.GetInt("storage_units", 0); errObj == nil {
		request.StorageUnits = uint32(storageUnits)
	}
	if withTerminal, errObj := kwargs.GetBool("with_terminal", template.WithTerminal); errObj == nil {
		request.WithTerminal = withTerminal
	}
	if withVSCodeTunnel, errObj := kwargs.GetBool("with_vscode_tunnel", template.WithVSCodeTunnel); errObj == nil {
		request.WithVSCodeTunnel = withVSCodeTunnel
	}
	if withCodeServer, errObj := kwargs.GetBool("with_code_server", template.WithCodeServer); errObj == nil {
		request.WithCodeServer = withCodeServer
	}
	if withSSH, errObj := kwargs.GetBool("with_ssh", template.WithSSH); errObj == nil {
		request.WithSSH = withSSH
	}
	if withRunCommand, errObj := kwargs.GetBool("with_run_command", template.WithRunCommand); errObj == nil {
		request.WithRunCommand = withRunCommand
	}
	if scheduleEnabled, errObj := kwargs.GetBool("schedule_enabled", template.ScheduleEnabled); errObj == nil {
		request.ScheduleEnabled = scheduleEnabled
	}
	if iconURL, errObj := kwargs.GetString("icon_url", ""); errObj == nil {
		request.IconURL = iconURL
	}
	if groups, errObj := kwargs.GetList("groups", []object.Object{}); errObj == nil {
		groupStrs := []string{}
		for _, g := range groups {
			if gStr, err := g.AsString(); err == nil {
				groupStrs = append(groupStrs, gStr)
			}
		}
		request.Groups = groupStrs
	}
	if zones, errObj := kwargs.GetList("zones", []object.Object{}); errObj == nil {
		zoneStrs := []string{}
		for _, z := range zones {
			if zStr, err := z.AsString(); err == nil {
				zoneStrs = append(zoneStrs, zStr)
			}
		}
		request.Zones = zoneStrs
	}
	if schedule, errObj := kwargs.GetList("schedule", []object.Object{}); errObj == nil {
		schedDays := []apiclient.TemplateDetailsDay{}
		for _, day := range schedule {
			if dayDict, ok := day.(*object.Dict); ok {
				schedDay := apiclient.TemplateDetailsDay{}
				if enabled, ok := dayDict.Pairs["enabled"]; ok {
					if enabledBool, err := enabled.Value.AsBool(); err == nil {
						schedDay.Enabled = enabledBool
					}
				}
				if from, ok := dayDict.Pairs["from"]; ok {
					if fromStr, err := from.Value.AsString(); err == nil {
						schedDay.From = fromStr
					}
				}
				if to, ok := dayDict.Pairs["to"]; ok {
					if toStr, err := to.Value.AsString(); err == nil {
						schedDay.To = toStr
					}
				}
				schedDays = append(schedDays, schedDay)
			}
		}
		request.Schedule = schedDays
	}
	if customFields, errObj := kwargs.GetList("custom_fields", []object.Object{}); errObj == nil {
		fields := []apiclient.CustomFieldDef{}
		for _, field := range customFields {
			if fieldDict, ok := field.(*object.Dict); ok {
				cf := apiclient.CustomFieldDef{}
				if name, ok := fieldDict.Pairs["name"]; ok {
					if nameStr, err := name.Value.AsString(); err == nil {
						cf.Name = nameStr
					}
				}
				if desc, ok := fieldDict.Pairs["description"]; ok {
					if descStr, err := desc.Value.AsString(); err == nil {
						cf.Description = descStr
					}
				}
				if cf.Name != "" {
					fields = append(fields, cf)
				}
			}
		}
		request.CustomFields = fields
	}

	_, apiErr = client.UpdateTemplate(ctx, templateId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update template: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// templateGetIcons returns list of available icons
func templateGetIcons(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Icons not available - API client not configured"}
	}

	icons, _, err := client.GetIcons(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get icons: %v", err)}
	}

	elements := make([]object.Object, len(icons.Icons))
	for i, icon := range icons.Icons {
		pairs := make(map[string]object.DictPair)
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: icon.Description}}
		pairs["source"] = object.DictPair{Key: &object.String{Value: "source"}, Value: &object.String{Value: icon.Source}}
		pairs["url"] = object.DictPair{Key: &object.String{Value: "url"}, Value: &object.String{Value: icon.URL}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}
