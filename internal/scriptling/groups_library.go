package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetGroupsLibrary returns the groups management library for scriptling
func GetGroupsLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.group", "Knot group management functions")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return groupList(ctx, client)
	}, "list() - List all groups")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return groupGet(ctx, client, args...)
	}, "get(group_id) - Get group by ID")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return groupCreate(ctx, client, kwargs, args...)
	}, "create(name, ...) - Create a new group")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return groupUpdate(ctx, client, kwargs, args...)
	}, "update(group_id, ...) - Update group properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return groupDelete(ctx, client, args...)
	}, "delete(group_id) - Delete a group")

	return builder.Build()
}

// groupList returns all groups
func groupList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Groups not available - API client not configured"}
	}

	groups, _, err := client.GetGroups(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list groups: %v", err)}
	}

	elements := make([]object.Object, len(groups.Groups))
	for i, group := range groups.Groups {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: group.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: group.Name}}
		pairs["max_spaces"] = object.DictPair{Key: &object.String{Value: "max_spaces"}, Value: &object.Integer{Value: int64(group.MaxSpaces)}}
		pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(group.ComputeUnits)}}
		pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(group.StorageUnits)}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// groupGet returns group by ID
func groupGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	groupId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("group_id", err)
	}

	group, _, apiErr := client.GetGroup(ctx, groupId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get group: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: group.Id}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: group.Name}}
	pairs["max_spaces"] = object.DictPair{Key: &object.String{Value: "max_spaces"}, Value: &object.Integer{Value: int64(group.MaxSpaces)}}
	pairs["compute_units"] = object.DictPair{Key: &object.String{Value: "compute_units"}, Value: &object.Integer{Value: int64(group.ComputeUnits)}}
	pairs["storage_units"] = object.DictPair{Key: &object.String{Value: "storage_units"}, Value: &object.Integer{Value: int64(group.StorageUnits)}}
	pairs["max_tunnels"] = object.DictPair{Key: &object.String{Value: "max_tunnels"}, Value: &object.Integer{Value: int64(group.MaxTunnels)}}

	return &object.Dict{Pairs: pairs}
}

// groupCreate creates a new group
func groupCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	request := &apiclient.GroupRequest{
		Name: name,
	}

	// Optional parameters
	if maxSpaces, errObj := kwargs.GetInt("max_spaces", 0); errObj == nil {
		request.MaxSpaces = uint32(maxSpaces)
	}
	if computeUnits, errObj := kwargs.GetInt("compute_units", 0); errObj == nil {
		request.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, errObj := kwargs.GetInt("storage_units", 0); errObj == nil {
		request.StorageUnits = uint32(storageUnits)
	}
	if maxTunnels, errObj := kwargs.GetInt("max_tunnels", 0); errObj == nil {
		request.MaxTunnels = uint32(maxTunnels)
	}

	groupId, _, apiErr := client.CreateGroup(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create group: %v", apiErr)}
	}

	return &object.String{Value: groupId}
}

// groupUpdate updates a group
func groupUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	groupId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("group_id", err)
	}

	// Get current group to build request
	group, _, apiErr := client.GetGroup(ctx, groupId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get group: %v", apiErr)}
	}

	request := &apiclient.GroupRequest{
		Name:         group.Name,
		MaxSpaces:    group.MaxSpaces,
		ComputeUnits: group.ComputeUnits,
		StorageUnits: group.StorageUnits,
		MaxTunnels:   group.MaxTunnels,
	}

	// Update with provided kwargs
	if name, errObj := kwargs.GetString("name", ""); errObj == nil && name != "" {
		request.Name = name
	}
	if maxSpaces, errObj := kwargs.GetInt("max_spaces", int64(group.MaxSpaces)); errObj == nil {
		request.MaxSpaces = uint32(maxSpaces)
	}
	if computeUnits, errObj := kwargs.GetInt("compute_units", int64(group.ComputeUnits)); errObj == nil {
		request.ComputeUnits = uint32(computeUnits)
	}
	if storageUnits, errObj := kwargs.GetInt("storage_units", int64(group.StorageUnits)); errObj == nil {
		request.StorageUnits = uint32(storageUnits)
	}

	_, apiErr = client.UpdateGroup(ctx, groupId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update group: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// groupDelete deletes a group
func groupDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	groupId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("group_id", err)
	}

	_, apiErr := client.DeleteGroup(ctx, groupId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete group: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
