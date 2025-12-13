package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/scriptling/object"
)

// GetSpacesLibrary returns the spaces helper library for scriptling (local/remote environments)
func GetSpacesLibrary(client *apiclient.ApiClient, userId string) *object.Library {
	functions := map[string]*object.Builtin{
		"start": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceStart(ctx, client, userId, args...)
			},
			HelpText: "start(name) - Start a space by name",
		},
		"stop": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceStop(ctx, client, userId, args...)
			},
			HelpText: "stop(name) - Stop a space by name",
		},
		"restart": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceRestart(ctx, client, userId, args...)
			},
			HelpText: "restart(name) - Restart a space by name",
		},
		"get_field": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceGetField(ctx, client, userId, args...)
			},
			HelpText: "get_field(name, field) - Get a custom field value from a space",
		},
		"set_field": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceSetField(ctx, client, userId, args...)
			},
			HelpText: "set_field(name, field, value) - Set a custom field value on a space",
		},
		"create": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceCreate(ctx, client, userId, kwargs, args...)
			},
			HelpText: "create(name, template_name, description='', shell='bash') - Create a new space",
		},
		"delete": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceDelete(ctx, client, userId, args...)
			},
			HelpText: "delete(name) - Delete a space by name",
		},
		"set_description": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceSetDescription(ctx, client, userId, args...)
			},
			HelpText: "set_description(name, description) - Set the description of a space",
		},
		"get_description": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceGetDescription(ctx, client, userId, args...)
			},
			HelpText: "get_description(name) - Get the description of a space",
		},
		"is_running": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceIsRunning(ctx, client, userId, args...)
			},
			HelpText: "is_running(name) - Check if a space is running",
		},
		"list": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceList(ctx, client, userId, args...)
			},
			HelpText: "list() - List all spaces for the current user",
		},
		"exec_script": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceExecScript(ctx, client, userId, args...)
			},
			HelpText: "exec_script(space_name, script_name, *args) - Execute a script in a space",
		},
		"exec_command": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceExecCommand(ctx, client, userId, kwargs, args...)
			},
			HelpText: "exec_command(space_name, command, args=[], timeout=30, workdir='') - Execute a command in a space",
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
	if len(args) < 1 {
		return &object.Error{Message: "start() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	_, err = client.StartSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to start space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceStop(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "stop() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	_, err = client.StopSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceRestart(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "restart() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	_, err = client.RestartSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to restart space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceGetField(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "get_field() requires space name and field name"}
	}

	spaceName := args[0].(*object.String).Value
	fieldName := args[1].(*object.String).Value

	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	response, _, err := client.GetSpaceCustomField(ctx, spaceId, fieldName)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get field: %v", err)}
	}

	return &object.String{Value: response.Value}
}

func spaceSetField(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 3 {
		return &object.Error{Message: "set_field() requires space name, field name, and value"}
	}

	spaceName := args[0].(*object.String).Value
	fieldName := args[1].(*object.String).Value
	fieldValue := args[2].(*object.String).Value

	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	_, err = client.SetSpaceCustomField(ctx, spaceId, fieldName, fieldValue)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to set field: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceCreate(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "create() requires name and template_name"}
	}

	name := args[0].(*object.String).Value
	templateName := args[1].(*object.String).Value

	template, err := client.GetTemplateByName(ctx, templateName)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", err)}
	}
	templateId := template.TemplateId

	description := ""
	if len(args) > 2 {
		description = args[2].(*object.String).Value
	}
	if desc, ok := kwargs["description"]; ok {
		description = desc.(*object.String).Value
	}

	shell := "bash"
	if len(args) > 3 {
		shell = args[3].(*object.String).Value
	}
	if sh, ok := kwargs["shell"]; ok {
		shell = sh.(*object.String).Value
	}

	request := &apiclient.SpaceRequest{
		Name:        name,
		Description: description,
		TemplateId:  templateId,
		Shell:       shell,
		UserId:      userId,
	}

	spaceId, _, err := client.CreateSpace(ctx, request)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create space: %v", err)}
	}

	return &object.String{Value: spaceId}
}

func spaceDelete(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "delete() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	_, err = client.DeleteSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceSetDescription(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "set_description() requires space name and description"}
	}

	spaceName := args[0].(*object.String).Value
	description := args[1].(*object.String).Value

	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	space, _, err := client.GetSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", err)}
	}

	request := &apiclient.SpaceRequest{
		Name:        space.Name,
		Description: description,
		TemplateId:  space.TemplateId,
		Shell:       space.Shell,
	}

	_, err = client.UpdateSpace(ctx, spaceId, request)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceGetDescription(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "get_description() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	space, _, err := client.GetSpace(ctx, spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", err)}
	}

	return &object.String{Value: space.Description}
}

func spaceIsRunning(ctx context.Context, client *apiclient.ApiClient, userId string, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "is_running() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	spaces, _, err := client.GetSpaces(ctx, userId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get spaces: %v", err)}
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
	if len(args) < 2 {
		return &object.Error{Message: "exec_script() requires space name and script name"}
	}

	spaceName := args[0].(*object.String).Value
	scriptName := args[1].(*object.String).Value

	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	script, err := client.GetScriptByName(ctx, scriptName)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get script: %v", err)}
	}

	scriptArgs := make([]string, 0, len(args)-2)
	for i := 2; i < len(args); i++ {
		scriptArgs = append(scriptArgs, args[i].(*object.String).Value)
	}

	output, err := client.ExecuteScript(ctx, spaceId, script.Id, scriptArgs)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute script: %v", err)}
	}

	return &object.String{Value: output}
}

func spaceExecCommand(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "exec_command() requires space name and command"}
	}

	spaceName := args[0].(*object.String).Value
	command := args[1].(*object.String).Value

	spaceId, err := resolveSpaceName(ctx, client, userId, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	cmdArgs := make([]string, 0)
	if len(args) > 2 {
		for i := 2; i < len(args); i++ {
			cmdArgs = append(cmdArgs, args[i].(*object.String).Value)
		}
	}
	if argsObj, ok := kwargs["args"]; ok {
		if argsList, ok := argsObj.(*object.List); ok {
			cmdArgs = make([]string, len(argsList.Elements))
			for i, elem := range argsList.Elements {
				cmdArgs[i] = elem.(*object.String).Value
			}
		}
	}

	timeout := 30
	if timeoutObj, ok := kwargs["timeout"]; ok {
		timeout = int(timeoutObj.(*object.Integer).Value)
	}

	workdir := ""
	if workdirObj, ok := kwargs["workdir"]; ok {
		workdir = workdirObj.(*object.String).Value
	}

	request := &apiclient.RunCommandRequest{
		Command: command,
		Args:    cmdArgs,
		Timeout: timeout,
		Workdir: workdir,
	}

	output, err := client.RunCommand(ctx, spaceId, request)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute command: %v", err)}
	}

	return &object.String{Value: output}
}
