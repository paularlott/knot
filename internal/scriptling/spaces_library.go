package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/object"
)

// GetSpacesLibrary returns the spaces helper library for scriptling (local/remote environments)
func GetSpacesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	functions := map[string]*object.Builtin{
		"start": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceStart(ctx, client, userId, args...)
			},
			HelpText: "start(name) - Start a space by name",
		},
		"stop": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceStop(ctx, client, userId, args...)
			},
			HelpText: "stop(name) - Stop a space by name",
		},
		"restart": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceRestart(ctx, client, userId, args...)
			},
			HelpText: "restart(name) - Restart a space by name",
		},
		"get_field": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceGetField(ctx, client, userId, args...)
			},
			HelpText: "get_field(name, field) - Get a custom field value from a space",
		},
		"set_field": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceSetField(ctx, client, userId, args...)
			},
			HelpText: "set_field(name, field, value) - Set a custom field value on a space",
		},
		"create": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceCreate(ctx, client, userId, kwargs, args...)
			},
			HelpText: "create(name, template_name, description='', shell='bash') - Create a new space",
		},
		"delete": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceDelete(ctx, client, userId, args...)
			},
			HelpText: "delete(name) - Delete a space by name",
		},
		"set_description": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceSetDescription(ctx, client, userId, args...)
			},
			HelpText: "set_description(name, description) - Set the description of a space",
		},
		"get_description": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceGetDescription(ctx, client, userId, args...)
			},
			HelpText: "get_description(name) - Get the description of a space",
		},
		"is_running": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceIsRunning(ctx, client, userId, args...)
			},
			HelpText: "is_running(name) - Check if a space is running",
		},
		"list": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceList(ctx, client, userId, args...)
			},
			HelpText: "list() - List all spaces for the current user",
		},
		"run_script": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceExecScript(ctx, client, userId, args...)
			},
			HelpText: "run_script(space_name, script_name, *args) - Execute a script in a space",
		},
		"run": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spaceExecCommand(ctx, client, userId, kwargs, args...)
			},
			HelpText: "run(space_name, command, args=[], timeout=30, workdir='') - Execute a command in a space",
		},
		"port_forward": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spacePortForward(ctx, client, userId, args...)
			},
			HelpText: "port_forward(source_space, local_port, remote_space, remote_port) - Forward a local port to a remote space port",
		},
		"port_list": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spacePortList(ctx, client, userId, args...)
			},
			HelpText: "port_list(space) - List active port forwards for a space",
		},
		"port_stop": {
			Fn: func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
				return spacePortStop(ctx, client, userId, args...)
			},
			HelpText: "port_stop(space, local_port) - Stop a port forward",
		},
	}

	return object.NewLibrary(functions, nil, "Space management functions")
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

func spaceStart(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "start")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	_, apiErr := client.StartSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to start space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceStop(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "stop")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	_, apiErr := client.StopSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceRestart(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "restart")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	_, apiErr := client.RestartSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to restart space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceGetField(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	fieldName, fieldErr := scriptling.GetString(args, 1, "field name")
	if fieldErr != nil {
		return fieldErr
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	response, _, apiErr := client.GetSpaceCustomField(ctx, spaceId, fieldName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get field: %v", apiErr)}
	}

	return &object.String{Value: response.Value}
}

func spaceSetField(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	fieldName, fieldErr := scriptling.GetString(args, 1, "field name")
	if fieldErr != nil {
		return fieldErr
	}

	fieldValue, valueErr := scriptling.GetString(args, 2, "field value")
	if valueErr != nil {
		return valueErr
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	_, apiErr := client.SetSpaceCustomField(ctx, spaceId, fieldName, fieldValue)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to set field: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	name, err := scriptling.GetString(args, 0, "name")
	if err != nil {
		return err
	}

	templateName, templateErr := scriptling.GetString(args, 1, "template_name")
	if templateErr != nil {
		return templateErr
	}

	template, apiErr := client.GetTemplateByName(ctx, templateName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", apiErr)}
	}
	templateId := template.TemplateId

	description := ""
	if len(args) > 2 {
		description, err = scriptling.GetString(args, 2, "description")
		if err != nil {
			// Optional arg - if wrong type, return error
			return err
		}
	}
	desc, getErr := kwargs.GetString("description", "")
	if getErr != nil {
		return &object.Error{Message: getErr.Error()}
	}
	description = desc

	shell := "bash"
	if len(args) > 3 {
		shell, err = scriptling.GetString(args, 3, "shell")
		if err != nil {
			// Optional arg - if wrong type, return error
			return err
		}
	}
	sh, getErr := kwargs.GetString("shell", "bash")
	if getErr != nil {
		return &object.Error{Message: getErr.Error()}
	}
	shell = sh

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

func spaceDelete(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "delete")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	_, apiErr := client.DeleteSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceSetDescription(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	description, descErr := scriptling.GetString(args, 1, "description")
	if descErr != nil {
		return descErr
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	space, _, apiErr := client.GetSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", apiErr)}
	}

	request := &apiclient.SpaceRequest{
		Name:        space.Name,
		Description: description,
		TemplateId:  space.TemplateId,
		Shell:       space.Shell,
	}

	_, apiErr = client.UpdateSpace(ctx, spaceId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceGetDescription(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	space, _, apiErr := client.GetSpace(ctx, spaceId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", apiErr)}
	}

	return &object.String{Value: space.Description}
}

func spaceIsRunning(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	spaces, _, apiErr := client.GetSpaces(ctx, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get spaces: %v", apiErr)}
	}

	for _, space := range spaces.Spaces {
		if space.Id == spaceId {
			return &object.Boolean{Value: space.IsDeployed}
		}
	}

	return &object.Boolean{Value: false}
}

func spaceList(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
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
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	scriptName, scriptErr := scriptling.GetString(args, 1, "script name")
	if scriptErr != nil {
		return scriptErr
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	scriptArgs := make([]string, 0, len(args)-2)
	for i := 2; i < len(args); i++ {
		arg, argErr := scriptling.GetString(args, i, "script arg")
		if argErr != nil {
			return argErr
		}
		scriptArgs = append(scriptArgs, arg)
	}

	output, apiErr := client.ExecuteScriptByName(ctx, spaceId, scriptName, scriptArgs)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute script: %v", apiErr)}
	}

	return &object.String{Value: output}
}

func spaceExecCommand(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	command, cmdErr := scriptling.GetString(args, 1, "command")
	if cmdErr != nil {
		return cmdErr
	}

	spaceId, resolveErr := resolveSpaceName(ctx, client, userId, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	cmdArgs := make([]string, 0)
	if len(args) > 2 {
		for i := 2; i < len(args); i++ {
			arg, argErr := scriptling.GetString(args, i, "command arg")
			if argErr != nil {
				return argErr
			}
			cmdArgs = append(cmdArgs, arg)
		}
	}
	argsList, getErr := kwargs.GetList("args", []object.Object{})
	if getErr != nil {
		return &object.Error{Message: getErr.Error()}
	}
	if len(argsList) > 0 {
		cmdArgs = make([]string, len(argsList))
		for i, elem := range argsList {
			arg, ok := elem.AsString()
			if !ok {
				return &object.Error{Message: fmt.Sprintf("args[%d]: must be a string", i)}
			}
			cmdArgs[i] = arg
		}
	}

	timeout := 30
	timeoutVal, getErr := kwargs.GetInt("timeout", 30)
	if getErr != nil {
		return &object.Error{Message: getErr.Error()}
	}
	timeout = int(timeoutVal)

	workdir := ""
	workdirVal, getErr := kwargs.GetString("workdir", "")
	if getErr != nil {
		return &object.Error{Message: getErr.Error()}
	}
	workdir = workdirVal

	request := &apiclient.RunCommandRequest{
		Command: command,
		Args:    cmdArgs,
		Timeout: timeout,
		Workdir: workdir,
	}

	output, apiErr := client.RunCommand(ctx, spaceId, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute command: %v", apiErr)}
	}

	return &object.String{Value: output}
}

func spacePortForward(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	sourceSpaceName, err := scriptling.GetString(args, 0, "source_space")
	if err != nil {
		return err
	}

	localPort, portErr := GetIntAsUint16(args, 1, "local_port")
	if portErr != nil {
		return portErr
	}

	remoteSpaceName, spaceErr := scriptling.GetString(args, 2, "remote_space")
	if spaceErr != nil {
		return spaceErr
	}

	remotePort, remotePortErr := GetIntAsUint16(args, 3, "remote_port")
	if remotePortErr != nil {
		return remotePortErr
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
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
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
	spaceName, err := scriptling.GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	localPort, portErr := GetIntAsUint16(args, 1, "local_port")
	if portErr != nil {
		return portErr
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
