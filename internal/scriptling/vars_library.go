package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetVarsLibrary returns the template variables management library for scriptling
func GetVarsLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.vars", "Knot template variable management functions")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return varList(ctx, client)
	}, "list() - List all template variables")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return varGet(ctx, client, args...)
	}, "get(var_id) - Get variable value")

	builder.FunctionWithHelp("set", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return varSet(ctx, client, kwargs, args...)
	}, "set(var_id, value) - Set variable value (updates existing)")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return varCreate(ctx, client, kwargs, args...)
	}, "create(name, value) - Create a new variable")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return varDelete(ctx, client, args...)
	}, "delete(var_id) - Delete a variable")

	return builder.Build()
}

// varList returns all template variables
func varList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Variables not available - API client not configured"}
	}

	vars, _, err := client.GetTemplateVars(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list variables: %v", err)}
	}

	elements := make([]object.Object, len(vars.TemplateVar))
	for i, v := range vars.TemplateVar {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: v.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: v.Name}}
		pairs["local"] = object.DictPair{Key: &object.String{Value: "local"}, Value: &object.Boolean{Value: v.Local}}
		pairs["protected"] = object.DictPair{Key: &object.String{Value: "protected"}, Value: &object.Boolean{Value: v.Protected}}
		pairs["restricted"] = object.DictPair{Key: &object.String{Value: "restricted"}, Value: &object.Boolean{Value: v.Restricted}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// varGet returns variable value
func varGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	varId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("var_id", err)
	}

	v, _, apiErr := client.GetTemplateVar(ctx, varId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get variable: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: v.Id}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: v.Name}}
	pairs["value"] = object.DictPair{Key: &object.String{Value: "value"}, Value: &object.String{Value: v.Value}}
	pairs["local"] = object.DictPair{Key: &object.String{Value: "local"}, Value: &object.Boolean{Value: v.Local}}
	pairs["protected"] = object.DictPair{Key: &object.String{Value: "protected"}, Value: &object.Boolean{Value: v.Protected}}
	pairs["restricted"] = object.DictPair{Key: &object.String{Value: "restricted"}, Value: &object.Boolean{Value: v.Restricted}}

	return &object.Dict{Pairs: pairs}
}

// varSet sets variable value
func varSet(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	varId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("var_id", err)
	}

	value, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("value", err)
	}

	// Get current variable to preserve other properties
	currentVar, _, apiErr := client.GetTemplateVar(ctx, varId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get variable: %v", apiErr)}
	}

	request := &apiclient.TemplateVarValue{
		Id:         currentVar.Id,
		Name:       currentVar.Name,
		Zones:      currentVar.Zones,
		Local:      currentVar.Local,
		Value:      value,
		Protected:  currentVar.Protected,
		Restricted: currentVar.Restricted,
		IsManaged:  currentVar.IsManaged,
	}

	_, apiErr = client.UpdateTemplateVar(ctx, varId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to set variable: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// varDelete deletes a variable
func varDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	varId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("var_id", err)
	}

	_, apiErr := client.DeleteTemplateVar(ctx, varId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete variable: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// varCreate creates a new variable
func varCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	value, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("value", err)
	}

	request := &apiclient.TemplateVarValue{
		Name:      name,
		Value:     value,
		Local:     false,
		Protected: false,
		Zones:     []string{},
	}

	// Optional parameters via kwargs
	if local, errObj := kwargs.GetBool("local", false); errObj == nil {
		request.Local = local
	}
	if protected, errObj := kwargs.GetBool("protected", false); errObj == nil {
		request.Protected = protected
	}

	varId, _, apiErr := client.CreateTemplateVar(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create variable: %v", apiErr)}
	}

	return &object.String{Value: varId}
}
