package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/errors"
	"github.com/paularlott/scriptling/object"
)

// GetSpacesLibrary returns the spaces helper library for scriptling (local/remote environments)
func GetSpacesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	builder := object.NewLibraryBuilder("knot.space", "Knot space management functions")

	builder.FunctionWithHelp("start", func(name string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, name)
		if err != nil {
			return false, err
		}
		_, err = client.StartSpace(context.Background(), spaceId)
		return err == nil, err
	}, "start(name) - Start a space by name")

	builder.FunctionWithHelp("stop", func(name string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, name)
		if err != nil {
			return false, err
		}
		_, err = client.StopSpace(context.Background(), spaceId)
		return err == nil, err
	}, "stop(name) - Stop a space by name")

	builder.FunctionWithHelp("restart", func(name string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, name)
		if err != nil {
			return false, err
		}
		_, err = client.RestartSpace(context.Background(), spaceId)
		return err == nil, err
	}, "restart(name) - Restart a space by name")

	builder.FunctionWithHelp("get_field", func(spaceName, fieldName string) (string, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, spaceName)
		if err != nil {
			return "", err
		}
		response, _, err := client.GetSpaceCustomField(context.Background(), spaceId, fieldName)
		if err != nil {
			return "", err
		}
		return response.Value, nil
	}, "get_field(name, field) - Get a custom field value from a space")

	builder.FunctionWithHelp("set_field", func(spaceName, fieldName, fieldValue string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, spaceName)
		if err != nil {
			return false, err
		}
		_, err = client.SetSpaceCustomField(context.Background(), spaceId, fieldName, fieldValue)
		return err == nil, err
	}, "set_field(name, field, value) - Set a custom field value on a space")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceCreate(ctx, client, userId, kwargs, args...)
	}, "create(name, template_name, description='', shell='bash') - Create a new space")

	builder.FunctionWithHelp("delete", func(name string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, name)
		if err != nil {
			return false, err
		}
		_, err = client.DeleteSpace(context.Background(), spaceId)
		return err == nil, err
	}, "delete(name) - Delete a space by name")

	builder.FunctionWithHelp("set_description", func(spaceName, description string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, spaceName)
		if err != nil {
			return false, err
		}
		space, _, err := client.GetSpace(context.Background(), spaceId)
		if err != nil {
			return false, err
		}
		request := &apiclient.SpaceRequest{
			Name:        space.Name,
			Description: description,
			TemplateId:  space.TemplateId,
			Shell:       space.Shell,
		}
		_, err = client.UpdateSpace(context.Background(), spaceId, request)
		return err == nil, err
	}, "set_description(name, description) - Set the description of a space")

	builder.FunctionWithHelp("get_description", func(spaceName string) (string, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, spaceName)
		if err != nil {
			return "", err
		}
		space, _, err := client.GetSpace(context.Background(), spaceId)
		if err != nil {
			return "", err
		}
		return space.Description, nil
	}, "get_description(name) - Get the description of a space")

	builder.FunctionWithHelp("is_running", func(spaceName string) (bool, error) {
		spaceId, err := resolveSpaceName(context.Background(), client, userId, spaceName)
		if err != nil {
			return false, err
		}
		spaces, _, err := client.GetSpaces(context.Background(), userId)
		if err != nil {
			return false, err
		}
		for _, space := range spaces.Spaces {
			if space.Id == spaceId {
				return space.IsDeployed, nil
			}
		}
		return false, nil
	}, "is_running(name) - Check if a space is running")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceList(ctx, client, userId)
	}, "list() - List all spaces for the current user")

	builder.FunctionWithHelp("run_script", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceExecScript(ctx, client, userId, args...)
	}, "run_script(space_name, script_name, *args) - Execute a script in a space")

	builder.FunctionWithHelp("run", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceExecCommand(ctx, client, userId, kwargs, args...)
	}, "run(space_name, command, args=[], timeout=30, workdir='') - Execute a command in a space")

	builder.FunctionWithHelp("port_forward", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spacePortForward(ctx, client, userId, args...)
	}, "port_forward(source_space, local_port, remote_space, remote_port) - Forward a local port to a remote space port")

	builder.FunctionWithHelp("port_list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spacePortList(ctx, client, userId, args...)
	}, "port_list(space) - List active port forwards for a space")

	builder.FunctionWithHelp("port_stop", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spacePortStop(ctx, client, userId, args...)
	}, "port_stop(space, local_port) - Stop a port forward")

	return builder.Build()
}

func resolveSpaceName(ctx context.Context, client *apiclient.ApiClient, userId string, spaceName string) (string, error) {
	spaces, _, err := client.GetSpaces(ctx, userId)
	if err != nil {
		return "", err
	}

	for _, space := range spaces.Spaces {
		if space.Name == spaceName {
			return space.Id, nil
		}
	}

	return "", fmt.Errorf("space not found: %s", spaceName)
}

func spaceCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	name, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	templateName, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("template_name", err)
	}

	template, apiErr := client.GetTemplateByName(ctx, templateName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", apiErr)}
	}
	templateId := template.TemplateId

	description, err := kwargs.GetString("description", "")
	if err != nil {
		return err
	}

	shell, err := kwargs.GetString("shell", "bash")
	if err != nil {
		return err
	}

	request := &apiclient.SpaceRequest{
		Name:        name,
		Description: description,
		TemplateId:  templateId,
		Shell:       shell,
		UserId:      userId,
	}

	spaceId, _, apiErr := client.CreateSpace(ctx, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create space: %v", apiErr)}
	}

	return &object.String{Value: spaceId}
}

func spaceList(ctx context.Context, client *apiclient.ApiClient, userId string) object.Object {
	spaces, _, err := client.GetSpaces(ctx, userId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list spaces: %v", err)}
	}

	elements := make([]object.Object, len(spaces.Spaces))
	for i, space := range spaces.Spaces {
		pairs := make(map[string]object.DictPair)
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: space.Name}}
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: space.Id}}
		pairs["is_running"] = object.DictPair{Key: &object.String{Value: "is_running"}, Value: &object.Boolean{Value: space.IsDeployed}}
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: space.Description}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

func spaceExecScript(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("space_name", err)
	}

	scriptName, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("script_name", err)
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	scriptArgs := make([]string, 0, len(args)-2)
	for i := 2; i < len(args); i++ {
		arg, err := args[i].AsString()
		if err != nil {
			return errors.ParameterError(fmt.Sprintf("arg[%d]", i-2), err)
		}
		scriptArgs = append(scriptArgs, arg)
	}

	output, _, apiErr := client.ExecuteScriptByName(ctx, spaceId, scriptName, scriptArgs)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute script: %v", apiErr)}
	}

	return &object.String{Value: output}
}

func spaceExecCommand(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 2); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("space_name", err)
	}

	command, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("command", err)
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	cmdArgs := make([]string, 0)
	argsList, err := kwargs.GetList("args", []object.Object{})
	if err != nil {
		return err
	}
	if len(argsList) > 0 {
		cmdArgs = make([]string, len(argsList))
		for i, elem := range argsList {
			arg, err := elem.AsString()
			if err != nil {
				return errors.ParameterError(fmt.Sprintf("args[%d]", i), err)
			}
			cmdArgs[i] = arg
		}
	}

	timeout, err := kwargs.GetInt("timeout", 30)
	if err != nil {
		return err
	}

	workdir, err := kwargs.GetString("workdir", "")
	if err != nil {
		return err
	}

	request := &apiclient.RunCommandRequest{
		Command: command,
		Args:    cmdArgs,
		Timeout: int(timeout),
		Workdir: workdir,
	}

	output, apiErr := client.RunCommand(ctx, spaceId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute command: %v", apiErr)}
	}

	return &object.String{Value: output}
}

func spacePortForward(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 4); err != nil {
		return err
	}

	sourceSpaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("source_space", err)
	}

	localPort, err := GetIntAsUint16(args, 1, "local_port")
	if err != nil {
		return err
	}

	remoteSpaceName, err := args[2].AsString()
	if err != nil {
		return errors.ParameterError("remote_space", err)
	}

	remotePort, err := GetIntAsUint16(args, 3, "remote_port")
	if err != nil {
		return err
	}

	sourceSpaceId, resolveErr := resolveSpaceName(ctx, client, userId, sourceSpaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	request := &apiclient.PortForwardRequest{
		LocalPort:  localPort,
		Space:      remoteSpaceName,
		RemotePort: remotePort,
	}

	_, apiErr := client.ForwardPort(ctx, sourceSpaceId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to forward port: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spacePortList(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("space_name", err)
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	response, _, apiErr := client.ListPorts(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list ports: %v", apiErr)}
	}

	elements := make([]object.Object, len(response.Forwards))
	for i, forward := range response.Forwards {
		pairs := make(map[string]object.DictPair)
		pairs["local_port"] = object.DictPair{Key: &object.String{Value: "local_port"}, Value: &object.Integer{Value: int64(forward.LocalPort)}}
		pairs["space"] = object.DictPair{Key: &object.String{Value: "space"}, Value: &object.String{Value: forward.Space}}
		pairs["remote_port"] = object.DictPair{Key: &object.String{Value: "remote_port"}, Value: &object.Integer{Value: int64(forward.RemotePort)}}
		elements[i] = &object.Dict{Pairs: pairs}
	}

	return &object.List{Elements: elements}
}

func spacePortStop(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 2); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("space_name", err)
	}

	localPort, err := GetIntAsUint16(args, 1, "local_port")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	request := &apiclient.PortStopRequest{
		LocalPort: localPort,
	}

	_, apiErr := client.StopPort(ctx, spaceId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop port: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
