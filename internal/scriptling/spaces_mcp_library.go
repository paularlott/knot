package scriptling

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/scriptling/object"
)

// SpaceService interface to avoid import cycle
type SpaceService interface {
	GetSpace(spaceId string, user *model.User) (*model.Space, error)
	GetSpaceCustomField(spaceId string, fieldName string, user *model.User) (string, error)
	SetSpaceCustomField(spaceId string, fieldName string, fieldValue string, user *model.User) error
	CreateSpace(space *model.Space, user *model.User) error
	UpdateSpace(space *model.Space, user *model.User) error
	DeleteSpace(spaceId string, user *model.User) error
}

// ContainerService interface to avoid import cycle
type ContainerService interface {
	StartSpace(space *model.Space, template *model.Template, user *model.User) error
	StopSpace(space *model.Space) error
	RestartSpace(space *model.Space) error
}

// AgentSession interface to avoid import cycle
type AgentSession interface {
	SendRunCommand(runCmd *msg.RunCommandMessage) (chan *msg.RunCommandResponse, error)
	SendExecuteScript(execMsg *msg.ExecuteScriptMessage) (chan *msg.ExecuteScriptResponse, error)
	SendPortForwardRequest(req *msg.PortForwardRequest) (chan *msg.PortForwardResponse, error)
	SendPortListRequest() (chan *msg.PortListResponse, error)
	SendPortStopRequest(req *msg.PortStopRequest) (chan *msg.PortStopResponse, error)
}

// GetSpacesMCPLibrary returns the spaces helper library for scriptling (MCP environment)
func GetSpacesMCPLibrary(
	user *model.User,
	spaceService SpaceService,
	containerService ContainerService,
	getAgentSession func(string) AgentSession,
) *object.Library {
	if getAgentSession == nil {
		getAgentSession = func(spaceId string) AgentSession { return nil }
	}
	functions := map[string]*object.Builtin{
		"start": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPStart(ctx, user, containerService, args...)
			},
			HelpText: "start(name) - Start a space by name",
		},
		"stop": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPStop(ctx, user, containerService, args...)
			},
			HelpText: "stop(name) - Stop a space by name",
		},
		"restart": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPRestart(ctx, user, containerService, args...)
			},
			HelpText: "restart(name) - Restart a space by name",
		},
		"get_field": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPGetField(ctx, user, spaceService, args...)
			},
			HelpText: "get_field(name, field) - Get a custom field value from a space",
		},
		"set_field": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPSetField(ctx, user, spaceService, args...)
			},
			HelpText: "set_field(name, field, value) - Set a custom field value on a space",
		},
		"create": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPCreate(ctx, user, spaceService, kwargs, args...)
			},
			HelpText: "create(name, template_name, description='', shell='bash') - Create a new space",
		},
		"delete": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPDelete(ctx, user, spaceService, args...)
			},
			HelpText: "delete(name) - Delete a space by name",
		},
		"set_description": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPSetDescription(ctx, user, spaceService, args...)
			},
			HelpText: "set_description(name, description) - Set the description of a space",
		},
		"get_description": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPGetDescription(ctx, user, spaceService, args...)
			},
			HelpText: "get_description(name) - Get the description of a space",
		},
		"is_running": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPIsRunning(ctx, user, args...)
			},
			HelpText: "is_running(name) - Check if a space is running",
		},
		"list": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPList(ctx, user, args...)
			},
			HelpText: "list() - List all spaces for the current user",
		},
		"run_script": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPExecScript(ctx, user, getAgentSession, args...)
			},
			HelpText: "run_script(space_name, script_name, *args) - Execute a script in a space",
		},
		"run": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPExecCommand(ctx, user, getAgentSession, kwargs, args...)
			},
			HelpText: "run(space_name, command, args=[], timeout=30, workdir='') - Execute a command in a space",
		},
		"port_forward": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPPortForward(ctx, user, getAgentSession, args...)
			},
			HelpText: "port_forward(source_space, local_port, remote_space, remote_port) - Forward a local port to a remote space port",
		},
		"port_list": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPPortList(ctx, user, getAgentSession, args...)
			},
			HelpText: "port_list(space) - List active port forwards for a space",
		},
		"port_stop": {
			Fn: func(ctx context.Context, kwargs map[string]object.Object, args ...object.Object) object.Object {
				return spaceMCPPortStop(ctx, user, getAgentSession, args...)
			},
			HelpText: "port_stop(space, local_port) - Stop a port forward",
		},
	}

	return object.NewLibrary(functions, nil, "Space management functions")
}

func resolveSpaceNameMCP(user *model.User, spaceName string) (string, error) {
	db := database.GetInstance()
	space, err := db.GetSpaceByName(user.Id, spaceName)
	if err != nil || space == nil {
		return "", fmt.Errorf("space not found: %s", spaceName)
	}
	return space.Id, nil
}

func spaceMCPStart(ctx context.Context, user *model.User, containerService ContainerService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "start")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	template, templateErr := db.GetTemplate(space.TemplateId)
	if templateErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", templateErr)}
	}

	svcErr := containerService.StartSpace(space, template, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to start space: %v", svcErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPStop(ctx context.Context, user *model.User, containerService ContainerService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "stop")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	svcErr := containerService.StopSpace(space)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop space: %v", svcErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPRestart(ctx context.Context, user *model.User, containerService ContainerService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "restart")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	svcErr := containerService.RestartSpace(space)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to restart space: %v", svcErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPGetField(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	fieldName, fieldErr := GetString(args, 1, "field name")
	if fieldErr != nil {
		return fieldErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	value, svcErr := spaceService.GetSpaceCustomField(spaceId, fieldName, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get field: %v", svcErr)}
	}

	return &object.String{Value: value}
}

func spaceMCPSetField(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	fieldName, fieldErr := GetString(args, 1, "field name")
	if fieldErr != nil {
		return fieldErr
	}

	fieldValue, valueErr := GetString(args, 2, "field value")
	if valueErr != nil {
		return valueErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	svcErr := spaceService.SetSpaceCustomField(spaceId, fieldName, fieldValue, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to set field: %v", svcErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPCreate(ctx context.Context, user *model.User, spaceService SpaceService, kwargs map[string]object.Object, args ...object.Object) object.Object {
	name, err := GetString(args, 0, "name")
	if err != nil {
		return err
	}

	templateName, templateErr := GetString(args, 1, "template_name")
	if templateErr != nil {
		return templateErr
	}

	db := database.GetInstance()
	templates, dbErr := db.GetTemplates()
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get templates: %v", dbErr)}
	}

	var templateId string
	for _, t := range templates {
		if t.Name == templateName && !t.IsDeleted && t.Active {
			templateId = t.Id
			break
		}
	}

	if templateId == "" {
		return &object.Error{Message: fmt.Sprintf("template not found: %s", templateName)}
	}

	description := ""
	if len(args) > 2 {
		description, err = GetString(args, 2, "description")
		if err != nil {
			return err
		}
	}
	if desc, found, kwErr := GetStringFromKwargs(kwargs, "description"); found {
		if kwErr != nil {
			return kwErr
		}
		description = desc
	}

	shell := "bash"
	if len(args) > 3 {
		shell, err = GetString(args, 3, "shell")
		if err != nil {
			return err
		}
	}
	if sh, found, kwErr := GetStringFromKwargs(kwargs, "shell"); found {
		if kwErr != nil {
			return kwErr
		}
		shell = sh
	}

	space := model.NewSpace(name, description, user.Id, templateId, shell, &[]string{}, "", "", []model.SpaceCustomField{})

	svcErr := spaceService.CreateSpace(space, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create space: %v", svcErr)}
	}

	return &object.String{Value: space.Id}
}

func spaceMCPDelete(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "delete")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	svcErr := spaceService.DeleteSpace(spaceId, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete space: %v", svcErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPSetDescription(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	description, descErr := GetString(args, 1, "description")
	if descErr != nil {
		return descErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	space, svcErr := spaceService.GetSpace(spaceId, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", svcErr)}
	}

	space.Description = description
	updateErr := spaceService.UpdateSpace(space, user)
	if updateErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update space: %v", updateErr)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPGetDescription(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	space, svcErr := spaceService.GetSpace(spaceId, user)
	if svcErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", svcErr)}
	}

	return &object.String{Value: space.Description}
}

func spaceMCPIsRunning(ctx context.Context, user *model.User, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	return &object.Boolean{Value: space.IsDeployed}
}

func spaceMCPList(ctx context.Context, user *model.User, args ...object.Object) object.Object {
	db := database.GetInstance()
	spaces, dbErr := db.GetSpacesForUser(user.Id)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list spaces: %v", dbErr)}
	}

	elements := make([]object.Object, 0, len(spaces))
	for _, space := range spaces {
		if space.IsDeleted {
			continue
		}
		pairs := make(map[string]object.DictPair)
		pairs["name"] = object.DictPair{Key: &object.String{Value: "name"}, Value: &object.String{Value: space.Name}}
		pairs["id"] = object.DictPair{Key: &object.String{Value: "id"}, Value: &object.String{Value: space.Id}}
		pairs["is_running"] = object.DictPair{Key: &object.String{Value: "is_running"}, Value: &object.Boolean{Value: space.IsDeployed}}
		pairs["description"] = object.DictPair{Key: &object.String{Value: "description"}, Value: &object.String{Value: space.Description}}
		elements = append(elements, &object.Dict{Pairs: pairs})
	}

	return &object.List{Elements: elements}
}

func spaceMCPExecScript(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	scriptName, scriptErr := GetString(args, 1, "script name")
	if scriptErr != nil {
		return scriptErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return &object.Error{Message: "no permission to execute scripts in this space"}
	}

	scripts, scriptsErr := db.GetScripts()
	if scriptsErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get scripts: %v", scriptsErr)}
	}

	var script *model.Script
	for _, s := range scripts {
		if s.Name == scriptName && !s.IsDeleted && s.Active {
			script = s
			break
		}
	}

	if script == nil {
		return &object.Error{Message: fmt.Sprintf("script not found: %s", scriptName)}
	}

	scriptArgs := make([]string, 0, len(args)-2)
	for i := 2; i < len(args); i++ {
		arg, argErr := GetString(args, i, "script arg")
		if argErr != nil {
			return argErr
		}
		scriptArgs = append(scriptArgs, arg)
	}

	// Get agent session for the space
	session := getAgentSession(spaceId)
	if session == nil {
		return &object.Error{Message: "space agent is not connected - cannot execute script"}
	}

	// Execute script via agent
	timeout := script.Timeout
	if timeout == 0 {
		timeout = 60
	}

	execMsg := &msg.ExecuteScriptMessage{
		Content:      script.Content,
		Arguments:    scriptArgs,
		Timeout:      timeout,
		IsSystemCall: false,
	}

	respChan, sendErr := session.SendExecuteScript(execMsg)
	if sendErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send script to agent: %v", sendErr)}
	}

	resp := <-respChan
	if !resp.Success {
		return &object.Error{Message: fmt.Sprintf("script execution failed: %s", resp.Error)}
	}

	return &object.String{Value: resp.Output}
}

func spaceMCPExecCommand(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, kwargs map[string]object.Object, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	command, cmdErr := GetString(args, 1, "command")
	if cmdErr != nil {
		return cmdErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	db := database.GetInstance()
	space, dbErr := db.GetSpace(spaceId)
	if dbErr != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", dbErr)}
	}

	template, templateErr := db.GetTemplate(space.TemplateId)
	if templateErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", templateErr)}
	}

	if !template.WithRunCommand {
		return &object.Error{Message: "running commands are not allowed in this space"}
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return &object.Error{Message: "no permission to run commands in this space"}
	}

	if !space.IsDeployed {
		return &object.Error{Message: "space is not running"}
	}

	cmdArgs := make([]string, 0)
	if len(args) > 2 {
		for i := 2; i < len(args); i++ {
			arg, argErr := GetString(args, i, "command arg")
			if argErr != nil {
				return argErr
			}
			cmdArgs = append(cmdArgs, arg)
		}
	}
	if argsList, found, kwErr := GetListFromKwargs(kwargs, "args"); found {
		if kwErr != nil {
			return kwErr
		}
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
	if timeoutVal, found, kwErr := GetIntFromKwargs(kwargs, "timeout"); found {
		if kwErr != nil {
			return kwErr
		}
		timeout = int(timeoutVal)
	}

	workdir := ""
	if workdirVal, found, kwErr := GetStringFromKwargs(kwargs, "workdir"); found {
		if kwErr != nil {
			return kwErr
		}
		workdir = workdirVal
	}

	var session AgentSession
	if getAgentSession != nil {
		session = getAgentSession(spaceId)
	}
	if session == nil {
		return &object.Error{Message: "space is not running or agent session not available"}
	}

	runCmd := &msg.RunCommandMessage{
		Command: command,
		Args:    cmdArgs,
		Timeout: timeout,
		Workdir: workdir,
	}

	responseChannel, sendErr := session.SendRunCommand(runCmd)
	if sendErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send command to agent: %v", sendErr)}
	}

	response := <-responseChannel
	if response == nil {
		return &object.Error{Message: "no response from agent"}
	}

	if !response.Success {
		return &object.Error{Message: response.Error}
	}

	return &object.String{Value: string(response.Output)}
}

func spaceMCPPortForward(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, args ...object.Object) object.Object {
	sourceSpaceName, err := GetString(args, 0, "source_space")
	if err != nil {
		return err
	}

	localPort, portErr := GetIntAsUint16(args, 1, "local_port")
	if portErr != nil {
		return portErr
	}

	remoteSpaceName, spaceErr := GetString(args, 2, "remote_space")
	if spaceErr != nil {
		return spaceErr
	}

	remotePort, remotePortErr := GetIntAsUint16(args, 3, "remote_port")
	if remotePortErr != nil {
		return remotePortErr
	}

	sourceSpaceId, resolveErr := resolveSpaceNameMCP(user, sourceSpaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	var session AgentSession
	if getAgentSession != nil {
		session = getAgentSession(sourceSpaceId)
	}
	if session == nil {
		return &object.Error{Message: "space is not running or agent session not available"}
	}

	req := &msg.PortForwardRequest{
		LocalPort:  localPort,
		Space:      remoteSpaceName,
		RemotePort: remotePort,
	}

	responseChannel, sendErr := session.SendPortForwardRequest(req)
	if sendErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port forward request to agent: %v", sendErr)}
	}

	response := <-responseChannel
	if response == nil {
		return &object.Error{Message: "no response from agent"}
	}

	if !response.Success {
		return &object.Error{Message: response.Error}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPPortList(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	var session AgentSession
	if getAgentSession != nil {
		session = getAgentSession(spaceId)
	}
	if session == nil {
		return &object.Error{Message: "space is not running or agent session not available"}
	}

	responseChannel, sendErr := session.SendPortListRequest()
	if sendErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port list request to agent: %v", sendErr)}
	}

	response := <-responseChannel
	if response == nil {
		return &object.Error{Message: "no response from agent"}
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

func spaceMCPPortStop(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, args ...object.Object) object.Object {
	spaceName, err := GetString(args, 0, "space name")
	if err != nil {
		return err
	}

	localPort, portErr := GetIntAsUint16(args, 1, "local_port")
	if portErr != nil {
		return portErr
	}

	spaceId, resolveErr := resolveSpaceNameMCP(user, spaceName)
	if resolveErr != nil {
		return &object.Error{Message: resolveErr.Error()}
	}

	var session AgentSession
	if getAgentSession != nil {
		session = getAgentSession(spaceId)
	}
	if session == nil {
		return &object.Error{Message: "space is not running or agent session not available"}
	}

	req := &msg.PortStopRequest{
		LocalPort: localPort,
	}

	responseChannel, sendErr := session.SendPortStopRequest(req)
	if sendErr != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port stop request to agent: %v", sendErr)}
	}

	response := <-responseChannel
	if response == nil {
		return &object.Error{Message: "no response from agent"}
	}

	if !response.Success {
		return &object.Error{Message: response.Error}
	}

	return &object.Boolean{Value: true}
}
