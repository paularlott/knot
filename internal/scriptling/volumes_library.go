package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetVolumesLibrary returns the volumes management library for scriptling
func GetVolumesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.volume", "Knot volume management functions")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeList(ctx, client)
	}, "list() - List all volumes")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeGet(ctx, client, args...)
	}, "get(volume_id) - Get volume by ID or name")

	builder.FunctionWithHelp("start", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeStart(ctx, client, args...)
	}, "start(volume_id) - Start a volume")

	builder.FunctionWithHelp("stop", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeStop(ctx, client, args...)
	}, "stop(volume_id) - Stop a volume")

	builder.FunctionWithHelp("is_running", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeIsRunning(ctx, client, args...)
	}, "is_running(volume_id) - Check if volume is running")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeCreate(ctx, client, kwargs, args...)
	}, "create(name, definition, platform='') - Create a new volume")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeUpdate(ctx, client, kwargs, args...)
	}, "update(volume_id, ...) - Update volume properties")

	builder.FunctionWithHelp("delete", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return volumeDelete(ctx, client, args...)
	}, "delete(volume_id) - Delete a volume")

	return builder.Build()
}

// volumeList returns all volumes
func volumeList(ctx context.Context, client *apiclient.ApiClient) object.Object {
	if client == nil {
		return &object.Error{Message: "Volumes not available - API client not configured"}
	}

	volumes, _, err := client.GetVolumes(ctx)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list volumes: %v", err)}
	}

	elements := make([]object.Object, len(volumes.Volumes))
	for i, vol := range volumes.Volumes {
		pairs := make(map[string]object.DictPair)
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: vol.Id}}
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: vol.Name}}
		pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: vol.Active}}
		pairs["zone"] = object.DictPair{Key: &object.String{Value: "zone"}, Value: &object.String{Value: vol.Zone}}
		pairs["platform"] = object.DictPair{Key: &object.String{Value: "platform"}, Value: &object.String{Value: vol.Platform}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

// volumeGet returns volume by ID
func volumeGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	volume, _, apiErr := client.GetVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get volume: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: volume.VolumeId}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: volume.Name}}
	pairs["definition"] = object.DictPair{Key: &object.String{Value: "definition"}, Value: &object.String{Value: volume.Definition}}
	pairs["active"] = object.DictPair{Key: &object.String{Value: "active"}, Value: &object.Boolean{Value: volume.Active}}
	pairs["zone"] = object.DictPair{Key: &object.String{Value: "zone"}, Value: &object.String{Value: volume.Zone}}
	pairs["platform"] = object.DictPair{Key: &object.String{Value: "platform"}, Value: &object.String{Value: volume.Platform}}

	return &object.Dict{Pairs: pairs}
}

// volumeStart starts a volume
func volumeStart(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	_, _, apiErr := client.StartVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to start volume: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// volumeStop stops a volume
func volumeStop(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	_, apiErr := client.StopVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop volume: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// volumeIsRunning checks if volume is running
func volumeIsRunning(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	volume, _, apiErr := client.GetVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get volume: %v", apiErr)}
	}

	return &object.Boolean{Value: volume.Active}
}

// volumeDelete deletes a volume
func volumeDelete(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	_, apiErr := client.DeleteVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete volume: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// volumeCreate creates a new volume
func volumeCreate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	definition, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("definition", err)
	}

	request := &apiclient.VolumeCreateRequest{
		Name:       name,
		Definition: definition,
		Platform:   "",
	}

	// Optional platform via kwargs
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil {
		request.Platform = platform
	}

	response, _, apiErr := client.CreateVolume(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create volume: %v", apiErr)}
	}

	return &object.String{Value: response.VolumeId}
}

// volumeUpdate updates a volume
func volumeUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	volumeId, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("volume_id", err)
	}

	// Get current volume to build request
	volume, _, apiErr := client.GetVolume(ctx, volumeId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get volume: %v", apiErr)}
	}

	request := &apiclient.VolumeUpdateRequest{
		Name:       volume.Name,
		Definition: volume.Definition,
		Platform:   volume.Platform,
	}

	// Update with provided kwargs
	if name, errObj := kwargs.GetString("name", ""); errObj == nil && name != "" {
		request.Name = name
	}
	if definition, errObj := kwargs.GetString("definition", ""); errObj == nil && definition != "" {
		request.Definition = definition
	}
	if platform, errObj := kwargs.GetString("platform", ""); errObj == nil {
		request.Platform = platform
	}

	_, apiErr = client.UpdateVolume(ctx, volumeId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update volume: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
