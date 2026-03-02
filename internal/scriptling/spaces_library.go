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
		_, err := client.StartSpace(context.Background(), name)
		return err == nil, err
	}, "start(name) - Start a space by name or ID")

	builder.FunctionWithHelp("stop", func(name string) (bool, error) {
		_, err := client.StopSpace(context.Background(), name)
		return err == nil, err
	}, "stop(name) - Stop a space by name or ID")

	builder.FunctionWithHelp("restart", func(name string) (bool, error) {
		_, err := client.RestartSpace(context.Background(), name)
		return err == nil, err
	}, "restart(name) - Restart a space by name or ID")

	builder.FunctionWithHelp("get_field", func(spaceName, fieldName string) (string, error) {
		response, _, err := client.GetSpaceCustomField(context.Background(), spaceName, fieldName)
		if err != nil {
			return "", err
		}
		return response.Value, nil
	}, "get_field(name, field) - Get a custom field value from a space")

	builder.FunctionWithHelp("set_field", func(spaceName, fieldName, fieldValue string) (bool, error) {
		_, err := client.SetSpaceCustomField(context.Background(), spaceName, fieldName, fieldValue)
		return err == nil, err
	}, "set_field(name, field, value) - Set a custom field value on a space")

	builder.FunctionWithHelp("create", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceCreate(ctx, client, userId, kwargs, args...)
	}, "create(name, template_name, description='', shell='bash') - Create a new space")

	builder.FunctionWithHelp("delete", func(name string) (bool, error) {
		_, err := client.DeleteSpace(context.Background(), name)
		return err == nil, err
	}, "delete(name) - Delete a space by name or ID")

	builder.FunctionWithHelp("set_description", func(spaceName, description string) (bool, error) {
		space, _, err := client.GetSpace(context.Background(), spaceName)
		if err != nil {
			return false, err
		}
		request := &apiclient.SpaceRequest{
			Name:        space.Name,
			Description: description,
			TemplateId:  space.TemplateId,
			Shell:       space.Shell,
		}
		_, err = client.UpdateSpace(context.Background(), spaceName, request)
		return err == nil, err
	}, "set_description(name, description) - Set the description of a space")

	builder.FunctionWithHelp("get_description", func(spaceName string) (string, error) {
		space, _, err := client.GetSpace(context.Background(), spaceName)
		if err != nil {
			return "", err
		}
		return space.Description, nil
	}, "get_description(name) - Get the description of a space")

	builder.FunctionWithHelp("is_running", func(spaceName string) (bool, error) {
		space, _, err := client.GetSpace(context.Background(), spaceName)
		if err != nil {
			return false, err
		}
		return space.IsDeployed, nil
	}, "is_running(name) - Check if a space is running")

	builder.FunctionWithHelp("get", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceGet(ctx, client, args...)
	}, "get(name) - Get space details as a dict")

	builder.FunctionWithHelp("update", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceUpdate(ctx, client, kwargs, args...)
	}, "update(name, description='', shell='') - Update space properties")

	builder.FunctionWithHelp("transfer", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceTransfer(ctx, client, args...)
	}, "transfer(name, user_id) - Transfer space to another user (user_id can be username, email, or UUID)")

	builder.FunctionWithHelp("share", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceShare(ctx, client, args...)
	}, "share(name, user_id) - Share space with another user (user_id can be username, email, or UUID)")

	builder.FunctionWithHelp("unshare", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceUnshare(ctx, client, args...)
	}, "unshare(name) - Remove space share")

	builder.FunctionWithHelp("list", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceList(ctx, client, userId)
	}, "list() - List all spaces for the current user")

	builder.FunctionWithHelp("run_script", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		return spaceExecScript(ctx, client, userId, kwargs, args...)
	}, "run_script(space_name, script_name, args=[]) - Execute a script in a space, returns {output: str, exit_code: int}")

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

	builder.FunctionWithHelp("read_file", func(spaceName, filePath string) (string, error) {
		space, _, err := client.GetSpace(context.Background(), spaceName)
		if err != nil {
			return "", err
		}
		return client.ReadSpaceFile(context.Background(), space.SpaceId, filePath)
	}, "read_file(space_name, file_path) - Read file contents from a running space")

	builder.FunctionWithHelp("write_file", func(spaceName, filePath, content string) (bool, error) {
		space, _, err := client.GetSpace(context.Background(), spaceName)
		if err != nil {
			return false, err
		}
		err = client.WriteSpaceFile(context.Background(), space.SpaceId, filePath, content)
		return err == nil, err
	}, "write_file(space_name, file_path, content) - Write content to a file in a running space")

	return builder.Build()
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

func spaceExecScript(ctx context.Context, client *apiclient.ApiClient, userId string, kwargs object.Kwargs, args ...object.Object) object.Object {
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

	// Check for args kwarg first (for backward compatibility with args= syntax)
	argsList, err := kwargs.GetList("args", []object.Object{})
	if err != nil {
		return err
	}

	var scriptArgs []string

	// If args kwarg was provided, use it
	if len(argsList) > 0 {
		scriptArgs = make([]string, len(argsList))
		for i, elem := range argsList {
			arg, err := elem.AsString()
			if err != nil {
				return errors.ParameterError(fmt.Sprintf("args[%d]", i), err)
			}
			scriptArgs[i] = arg
		}
	} else if len(args) > 2 {
		// Otherwise, collect extra positional arguments (from *args unpacking)
		scriptArgs = make([]string, len(args)-2)
		for i := 2; i < len(args); i++ {
			arg, err := args[i].AsString()
			if err != nil {
				return errors.ParameterError(fmt.Sprintf("arg[%d]", i-2), err)
			}
			scriptArgs[i-2] = arg
		}
	} else {
		scriptArgs = []string{}
	}

	output, exitCode, apiErr := client.ExecuteScriptByName(ctx, spaceName, scriptName, scriptArgs)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("script execution failed: %v", apiErr)}
	}

	// Return dict with output and exit_code
	pairs := make(map[string]object.DictPair)
	pairs["output"] = object.DictPair{Key: &object.String{Value: "output"}, Value: &object.String{Value: output}}
	pairs["exit_code"] = object.DictPair{Key: &object.String{Value: "exit_code"}, Value: &object.Integer{Value: int64(exitCode)}}
	return &object.Dict{Pairs: pairs}
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

	output, apiErr := client.RunCommand(ctx, spaceName, request)
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

	request := &apiclient.PortForwardRequest{
		LocalPort:  localPort,
		Space:      remoteSpaceName,
		RemotePort: remotePort,
	}

	_, apiErr := client.ForwardPort(ctx, sourceSpaceName, request)
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

	response, _, apiErr := client.ListPorts(ctx, spaceName)
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

	request := &apiclient.PortStopRequest{
		LocalPort: localPort,
	}

	_, apiErr := client.StopPort(ctx, spaceName, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop port: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// spaceGet returns space details as a dict
func spaceGet(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	space, _, apiErr := client.GetSpace(ctx, spaceName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", apiErr)}
	}

	pairs := make(map[string]object.DictPair)
	pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: space.SpaceId}}
	pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: space.Name}}
	pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: space.Description}}
	pairs["template_id"] = object.DictPair{Key: &object.String{Value: "template_id"}, Value: &object.String{Value: space.TemplateId}}
	pairs["template_name"] = object.DictPair{Key: &object.String{Value: "template_name"}, Value: &object.String{Value: space.TemplateName}}
	pairs["user_id"] = object.DictPair{Key: &object.String{Value: "user_id"}, Value: &object.String{Value: space.UserId}}
	pairs["username"] = object.DictPair{Key: &object.String{Value: "username"}, Value: &object.String{Value: space.Username}}
	pairs["shared_user_id"] = object.DictPair{Key: &object.String{Value: "shared_user_id"}, Value: &object.String{Value: space.SharedUserId}}
	pairs["shared_username"] = object.DictPair{Key: &object.String{Value: "shared_username"}, Value: &object.String{Value: space.SharedUsername}}
	pairs["shell"] = object.DictPair{Key: &object.String{Value: "shell"}, Value: &object.String{Value: space.Shell}}
	pairs["platform"] = object.DictPair{Key: &object.String{Value: "platform"}, Value: &object.String{Value: space.Platform}}
	pairs["zone"] = object.DictPair{Key: &object.String{Value: "zone"}, Value: &object.String{Value: space.Zone}}
	pairs["is_running"] = object.DictPair{Key: &object.String{Value: "is_running"}, Value: &object.Boolean{Value: space.IsDeployed}}
	pairs["is_pending"] = object.DictPair{Key: &object.String{Value: "is_pending"}, Value: &object.Boolean{Value: space.IsPending}}
	pairs["is_deleting"] = object.DictPair{Key: &object.String{Value: "is_deleting"}, Value: &object.Boolean{Value: space.IsDeleting}}
	pairs["node_hostname"] = object.DictPair{Key: &object.String{Value: "node_hostname"}, Value: &object.String{Value: space.NodeHostname}}
	pairs["created_at"] = object.DictPair{Key: &object.String{Value: "created_at"}, Value: &object.String{Value: space.CreatedAt.Format("2006-01-02T15:04:05Z")}}

	return &object.Dict{Pairs: pairs}
}

// spaceUpdate updates space properties
func spaceUpdate(ctx context.Context, client *apiclient.ApiClient, kwargs object.Kwargs, args ...object.Object) object.Object {
	if err := errors.MinArgs(args, 1); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	// Get current space to build request
	space, _, apiErr := client.GetSpace(ctx, spaceName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get space: %v", apiErr)}
	}

	description, err := kwargs.GetString("description", space.Description)
	if err != nil {
		return err
	}

	shell, err := kwargs.GetString("shell", space.Shell)
	if err != nil {
		return err
	}

	request := &apiclient.SpaceRequest{
		Name:        space.Name,
		Description: description,
		TemplateId:  space.TemplateId,
		Shell:       shell,
	}

	_, apiErr = client.UpdateSpace(ctx, spaceName, request)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// spaceTransfer transfers a space to another user
func spaceTransfer(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 2); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	userId, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	_, apiErr := client.TransferSpace(ctx, spaceName, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to transfer space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// spaceShare shares a space with another user
func spaceShare(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 2); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	userId, err := args[1].AsString()
	if err != nil {
		return errors.ParameterError("user_id", err)
	}

	_, apiErr := client.AddShare(ctx, spaceName, userId)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to share space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}

// spaceUnshare removes a space share
func spaceUnshare(ctx context.Context, client *apiclient.ApiClient, args ...object.Object) object.Object {
	if err := errors.ExactArgs(args, 1); err != nil {
		return err
	}

	spaceName, err := args[0].AsString()
	if err != nil {
		return errors.ParameterError("name", err)
	}

	_, apiErr := client.RemoveShare(ctx, spaceName)
	if apiErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to unshare space: %v", apiErr)}
	}

	return &object.Boolean{Value: true}
}
