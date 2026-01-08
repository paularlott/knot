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
	executeScriptLocally func(*model.Script, []string) (string, error),
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
				return spaceMCPExecScript(ctx, user, executeScriptLocally, args...)
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
	if len(args) < 1 {
		return &object.Error{Message: "start() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", err)}
	}

	err = containerService.StartSpace(space, template, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to start space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPStop(ctx context.Context, user *model.User, containerService ContainerService, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "stop() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	err = containerService.StopSpace(space)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to stop space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPRestart(ctx context.Context, user *model.User, containerService ContainerService, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "restart() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	err = containerService.RestartSpace(space)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to restart space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPGetField(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "get_field() requires space name and field name"}
	}

	spaceName := args[0].(*object.String).Value
	fieldName := args[1].(*object.String).Value

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	value, err := spaceService.GetSpaceCustomField(spaceId, fieldName, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get field: %v", err)}
	}

	return &object.String{Value: value}
}

func spaceMCPSetField(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	if len(args) < 3 {
		return &object.Error{Message: "set_field() requires space name, field name, and value"}
	}

	spaceName := args[0].(*object.String).Value
	fieldName := args[1].(*object.String).Value
	fieldValue := args[2].(*object.String).Value

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	err = spaceService.SetSpaceCustomField(spaceId, fieldName, fieldValue, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to set field: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPCreate(ctx context.Context, user *model.User, spaceService SpaceService, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "create() requires name and template_name"}
	}

	name := args[0].(*object.String).Value
	templateName := args[1].(*object.String).Value

	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get templates: %v", err)}
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

	space := model.NewSpace(name, description, user.Id, templateId, shell, &[]string{}, "", "", []model.SpaceCustomField{})

	err = spaceService.CreateSpace(space, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to create space: %v", err)}
	}

	return &object.String{Value: space.Id}
}

func spaceMCPDelete(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "delete() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	err = spaceService.DeleteSpace(spaceId, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to delete space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPSetDescription(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "set_description() requires space name and description"}
	}

	spaceName := args[0].(*object.String).Value
	description := args[1].(*object.String).Value

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	space, err := spaceService.GetSpace(spaceId, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	space.Description = description
	err = spaceService.UpdateSpace(space, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to update space: %v", err)}
	}

	return &object.Boolean{Value: true}
}

func spaceMCPGetDescription(ctx context.Context, user *model.User, spaceService SpaceService, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "get_description() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	space, err := spaceService.GetSpace(spaceId, user)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	return &object.String{Value: space.Description}
}

func spaceMCPIsRunning(ctx context.Context, user *model.User, args ...object.Object) object.Object {
	if len(args) < 1 {
		return &object.Error{Message: "is_running() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	return &object.Boolean{Value: space.IsDeployed}
}

func spaceMCPList(ctx context.Context, user *model.User, args ...object.Object) object.Object {
	db := database.GetInstance()
	spaces, err := db.GetSpacesForUser(user.Id)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to list spaces: %v", err)}
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

func spaceMCPExecScript(ctx context.Context, user *model.User, executeScriptLocally func(*model.Script, []string) (string, error), args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "run_script() requires space name and script name"}
	}

	spaceName := args[0].(*object.String).Value
	scriptName := args[1].(*object.String).Value

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return &object.Error{Message: "no permission to execute scripts in this space"}
	}

	scripts, err := db.GetScripts()
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get scripts: %v", err)}
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
		scriptArgs = append(scriptArgs, args[i].(*object.String).Value)
	}

	output, err := executeScriptLocally(script, scriptArgs)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to execute script: %v", err)}
	}

	return &object.String{Value: output}
}

func spaceMCPExecCommand(ctx context.Context, user *model.User, getAgentSession func(string) AgentSession, kwargs map[string]object.Object, args ...object.Object) object.Object {
	if len(args) < 2 {
		return &object.Error{Message: "run() requires space name and command"}
	}

	spaceName := args[0].(*object.String).Value
	command := args[1].(*object.String).Value

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("space not found: %v", err)}
	}

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to get template: %v", err)}
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

	responseChannel, err := session.SendRunCommand(runCmd)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send command to agent: %v", err)}
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
	if len(args) < 4 {
		return &object.Error{Message: "port_forward() requires source_space, local_port, remote_space, and remote_port"}
	}

	sourceSpaceName := args[0].(*object.String).Value
	localPort := uint16(args[1].(*object.Integer).Value)
	remoteSpaceName := args[2].(*object.String).Value
	remotePort := uint16(args[3].(*object.Integer).Value)

	sourceSpaceId, err := resolveSpaceNameMCP(user, sourceSpaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
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

	responseChannel, err := session.SendPortForwardRequest(req)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port forward request to agent: %v", err)}
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
	if len(args) < 1 {
		return &object.Error{Message: "port_list() requires space name"}
	}

	spaceName := args[0].(*object.String).Value
	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
	}

	var session AgentSession
	if getAgentSession != nil {
		session = getAgentSession(spaceId)
	}
	if session == nil {
		return &object.Error{Message: "space is not running or agent session not available"}
	}

	responseChannel, err := session.SendPortListRequest()
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port list request to agent: %v", err)}
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
	if len(args) < 2 {
		return &object.Error{Message: "port_stop() requires space name and local_port"}
	}

	spaceName := args[0].(*object.String).Value
	localPort := uint16(args[1].(*object.Integer).Value)

	spaceId, err := resolveSpaceNameMCP(user, spaceName)
	if err != nil {
		return &object.Error{Message: err.Error()}
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

	responseChannel, err := session.SendPortStopRequest(req)
	if err != nil {
		return &object.Error{Message: fmt.Sprintf("failed to send port stop request to agent: %v", err)}
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
