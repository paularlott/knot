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
	}, "create(name, job, platform='', description='', ...) - Create a new template")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateUpdate(ctx, client, kwargs, args...)
	}, "update(template_id, ...) - Update template properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return templateDelete(ctx, client, args...)
	}, "delete(template_id) - Delete a template")

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

// templateGet returns template by ID
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
	pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: template.Active}}
	pairs["is_managed"] = object.DictPair{Key: &object.String{Value: "is_managed"}, Value: &object.Boolean{Value: template.IsManaged}}
	pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(template.ComputeUnits)}}
	pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(template.StorageUnits)}}
	pairs["usage"] = object.DictPair{Key: &object.String{Value: "usage"}, Value: &object.Integer{Value: int64(template.Usage)}}
	pairs["deployed"] = object.DictPair{Key: &object.String{Value: "deployed"}, Value: &object.Integer{Value: int64(template.Deployed)}}

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
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	job, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("job", err)
	}

	request := &apiclient.TemplateCreateRequest{
		Name:        name,
		Job:         job,
		Description: "",
		Platform:    "",
		Active:      true,
		Volumes:     "",
		Groups:      []string{},
	}

	// Optional parameters via kwargs
	if description, errObj := kwargs.GetString("description", ""); errObj == nil {
		request.Description = description
	}
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil {
		request.Platform = platform
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
		WithVSCodeTunnel:  template.WithVSCodeTunnel,
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
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil {
		request.Platform = platform
	}
	if active, errObj := kwargs.GetBool("active", template.Active); errObj == nil {
		request.Active = active
	}

	_, apiErr = client.UpdateTemplate(ctx, templateId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update template: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
