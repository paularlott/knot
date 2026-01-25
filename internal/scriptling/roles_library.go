package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetRolesLibrary returns the roles management library for scriptling
func GetRolesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.role", "Knot role management functions")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return roleList(ctx, client)
	}, "list() - List all roles")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return roleGet(ctx, client, args...)
	}, "get(role_id) - Get role by ID")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return roleCreate(ctx, client, kwargs, args...)
	}, "create(name, permissions=[], ...) - Create a new role")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return roleUpdate(ctx, client, kwargs, args...)
	}, "update(role_id, ...) - Update role properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return roleDelete(ctx, client, args...)
	}, "delete(role_id) - Delete a role")

	return builder.Build()
}

// roleList returns all roles
func roleList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Roles not available - API client not configured"}
	}

	roles, _, err := client.GetRoles(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list roles: %v", err)}
	}

	elements := make([]object.Object, len(roles.Roles))
	for i, role := range roles.Roles {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: role.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: role.Name}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// roleGet returns role by ID
func roleGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	roleId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("role_id", err)
	}

	role, _, apiErr := client.GetRole(ctx, roleId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get role: %v", apiErr)}
	}

	// Convert permissions to list of integers
	permElements := make([]object.Object, len(role.Permissions))
	for i, perm := range role.Permissions {
		permElements[i] = &object.Integer{Value: int64(perm)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: role.Id}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: role.Name}}
	pairs["permissions"] = object.DictPair{Key: &object.String{Value: "permissions"}, Value: &object.List{Elements: permElements}}

	return &object.Dict{Pairs: pairs}
}

// roleCreate creates a new role
func roleCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	request := &apiclient.RoleRequest{
		Name:        name,
		Permissions: []uint16{},
	}

	// Get permissions from kwargs or args
	if len(args) > 1 {
		// Second argument could be a list of permissions
		if permList, err := args[1].AsList(); err == nil {
			permissions := make([]uint16, len(permList))
			for i, perm := range permList {
				if permInt, err := perm.AsInt(); err == nil {
					permissions[i] = uint16(permInt)
				}
			}
			request.Permissions = permissions
		}
	} else {
		// Try to get from kwargs
		if permVal := kwargs.Get("permissions"); permVal != nil {
			if permList, err := permVal.AsList(); err == nil {
				permissions := make([]uint16, len(permList))
				for i, perm := range permList {
					if permInt, err := perm.AsInt(); err == nil {
						permissions[i] = uint16(permInt)
					}
				}
				request.Permissions = permissions
			}
		}
	}

	roleId, _, apiErr := client.CreateRole(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create role: %v", apiErr)}
	}

	return &object.String{Value: roleId}
}

// roleUpdate updates a role
func roleUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	roleId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("role_id", err)
	}

	// Get current role to build request
	role, _, apiErr := client.GetRole(ctx, roleId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get role: %v", apiErr)}
	}

	request := &apiclient.RoleRequest{
		Name:        role.Name,
		Permissions: role.Permissions,
	}

	// Update with provided kwargs
	if name, errObj := kwargs.GetString("name", ""); errObj == nil && name != "" {
		request.Name = name
	}

	// Check for permissions in kwargs or args
	if len(args) > 1 {
		if permList, err := args[1].AsList(); err == nil {
			permissions := make([]uint16, len(permList))
			for i, perm := range permList {
				if permInt, err := perm.AsInt(); err == nil {
					permissions[i] = uint16(permInt)
				}
			}
			request.Permissions = permissions
		}
	} else if permVal := kwargs.Get("permissions"); permVal != nil {
		if permList, err := permVal.AsList(); err == nil {
			permissions := make([]uint16, len(permList))
			for i, perm := range permList {
				if permInt, err := perm.AsInt(); err == nil {
					permissions[i] = uint16(permInt)
				}
			}
			request.Permissions = permissions
		}
	}

	_, apiErr = client.UpdateRole(ctx, roleId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update role: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// roleDelete deletes a role
func roleDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	roleId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("role_id", err)
	}

	_, apiErr := client.DeleteRole(ctx, roleId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete role: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
