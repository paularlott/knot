package api

import (
	"fmt"
	"net/http"

	"github.com/paularlott/gossip/hlc"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	knotwebchat "github.com/paularlott/knot/internal/webchat"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetCommands(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	filterUserId := r.URL.Query().Get("user_id")
	allZones := r.URL.Query().Get("all_zones") == "true"

	if filterUserId != "" {
		if filterUserId == user.Id {
			if !user.HasPermission(model.PermissionManageOwnSlashCommands) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.CommandList{Count: 0, Commands: []apiclient.CommandInfo{}})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusOK, w, r, apiclient.CommandList{Count: 0, Commands: []apiclient.CommandInfo{}})
				return
			}
		}
	} else {
		canSeeGlobals := user.HasPermission(model.PermissionManageGlobalSlashCommands)
		canSeeOwn := user.HasPermission(model.PermissionManageOwnSlashCommands)

		if !canSeeGlobals && !canSeeOwn {
			rest.WriteResponse(http.StatusOK, w, r, apiclient.CommandList{Count: 0, Commands: []apiclient.CommandInfo{}})
			return
		}

		if canSeeOwn && !canSeeGlobals {
			filterUserId = user.Id
		}
	}

	commandService := service.GetCommandService()
	commands, err := commandService.ListCommands(service.CommandListOptions{
		FilterUserId:         filterUserId,
		User:                 user,
		IncludeDeleted:       false,
		CheckZoneRestriction: !allZones,
	})
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	response := apiclient.CommandList{
		Count:    0,
		Commands: []apiclient.CommandInfo{},
	}

	seenCommands := make(map[string]bool)

	for _, command := range commands {
		response.Commands = append(response.Commands, apiclient.CommandInfo{
			Id:           command.Id,
			UserId:       command.UserId,
			Name:         command.Name,
			Description:  command.Description,
			ArgumentHint: command.ArgumentHint,
			AllowedTools: command.AllowedTools,
			Groups:       command.Groups,
			Zones:        command.Zones,
			Active:       command.Active,
			IsManaged:    command.IsManaged,
		})
		seenCommands[command.Id] = true
		response.Count++
	}

	if filterUserId == "" && user.HasPermission(model.PermissionManageOwnSlashCommands) {
		ownCommands, err := commandService.ListCommands(service.CommandListOptions{
			FilterUserId:         user.Id,
			User:                 user,
			IncludeDeleted:       false,
			CheckZoneRestriction: !allZones,
		})
		if err == nil {
			for _, command := range ownCommands {
				if !seenCommands[command.Id] {
					response.Commands = append(response.Commands, apiclient.CommandInfo{
						Id:           command.Id,
						UserId:       command.UserId,
						Name:         command.Name,
						Description:  command.Description,
						ArgumentHint: command.ArgumentHint,
						AllowedTools: command.AllowedTools,
						Groups:       command.Groups,
						Zones:        command.Zones,
						Active:       command.Active,
						IsManaged:    command.IsManaged,
					})
					seenCommands[command.Id] = true
					response.Count++
				}
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func HandleGetCommand(w http.ResponseWriter, r *http.Request) {
	commandIdOrName := r.PathValue("command_id")
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	var command *model.Command
	var err error

	if validate.UUID(commandIdOrName) {
		command, err = db.GetCommand(commandIdOrName)
		if err != nil || command.IsDeleted {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
			return
		}

		if command.IsUserCommand() {
			if command.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
				return
			}
			if command.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to view this command"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				if len(command.Groups) > 0 && !user.HasAnyGroup(&command.Groups) {
					rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
					return
				}
			}
		}
	} else {
		command, err = service.ResolveCommandByName(commandIdOrName, user.Id)
		if err != nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
			return
		}

		if !service.CanUserAccessCommand(user, command) {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, apiclient.CommandDetails{
		Id:           command.Id,
		UserId:       command.UserId,
		Name:         command.Name,
		Description:  command.Description,
		ArgumentHint: command.ArgumentHint,
		AllowedTools: command.AllowedTools,
		Body:         command.Body,
		Groups:       command.Groups,
		Zones:        command.Zones,
		Active:       command.Active,
		IsManaged:    command.IsManaged,
	})
}

func HandleCreateCommand(w http.ResponseWriter, r *http.Request) {
	request := apiclient.CommandCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if len(request.Content) > 1*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Command content exceeds 1MB limit"})
		return
	}

	fm, err := util.ParseCommandFrontmatter(request.Content)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Invalid frontmatter: %v", err)})
		return
	}

	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	ownerUserId := request.UserId
	if ownerUserId == "current" {
		ownerUserId = user.Id
	}
	isUserCommand := ownerUserId != ""

	if !cfg.LeafNode {
		if isUserCommand {
			if ownerUserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create commands for other users"})
				return
			}
			if ownerUserId == user.Id && !user.HasPermission(model.PermissionManageOwnSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create own commands"})
				return
			}
			if len(request.Groups) > 0 {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User commands cannot have groups"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create global commands"})
				return
			}
		}
	} else {
		if isUserCommand && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User commands cannot have groups"})
			return
		}
	}

	// Extract body (everything after frontmatter)
	_, body := splitFrontmatter(request.Content)

	command := model.NewCommand(
		fm.Name,
		fm.Description,
		fm.ArgumentHint,
		fm.AllowedTools,
		body,
		request.Groups,
		request.Zones,
		ownerUserId,
		user.Id,
	)
	command.Active = request.Active

	err = db.SaveCommand(command, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipCommand(command)
	sse.PublishSlashCommandsChanged(command.Id)
	knotwebchat.BroadcastCommandEvent()

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSlashCommandCreate,
		fmt.Sprintf("Created slash command %s", command.Name),
		&map[string]interface{}{
			"agent":            r.UserAgent(),
			"IP":               r.RemoteAddr,
			"X-Forwarded-For":  r.Header.Get("X-Forwarded-For"),
			"command_id":       command.Id,
			"command_name":     command.Name,
			"is_user_command":  isUserCommand,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.CommandCreateResponse{
		Status: true,
		Id:     command.Id,
	})
}

func HandleUpdateCommand(w http.ResponseWriter, r *http.Request) {
	commandIdOrName := r.PathValue("command_id")
	user := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	var command *model.Command
	var err error

	if validate.UUID(commandIdOrName) {
		command, err = db.GetCommand(commandIdOrName)
	} else {
		command, err = service.ResolveCommandByName(commandIdOrName, user.Id)
	}

	if err != nil || command.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
		return
	}

	request := apiclient.CommandUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if len(request.Content) > 1*1024*1024 {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Command content exceeds 1MB limit"})
		return
	}

	fm, err := util.ParseCommandFrontmatter(request.Content)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("Invalid frontmatter: %v", err)})
		return
	}

	cfg := config.GetServerConfig()

	if command.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot edit managed command"})
		return
	}

	if !cfg.LeafNode {
		if command.IsUserCommand() {
			if command.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this command"})
				return
			}
			if command.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit own commands"})
				return
			}
			if len(request.Groups) > 0 {
				rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User commands cannot have groups"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit global commands"})
				return
			}
		}
	} else {
		if command.IsUserCommand() && len(request.Groups) > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "User commands cannot have groups"})
			return
		}
	}

	// Extract body (everything after frontmatter)
	_, body := splitFrontmatter(request.Content)

	command.Name = fm.Name
	command.Description = fm.Description
	command.ArgumentHint = fm.ArgumentHint
	command.AllowedTools = fm.AllowedTools
	command.Body = body
	command.Groups = request.Groups
	command.Zones = request.Zones
	command.Active = request.Active
	command.UpdatedUserId = user.Id
	command.UpdatedAt = hlc.Now()

	err = db.SaveCommand(command, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipCommand(command)
	sse.PublishSlashCommandsChanged(command.Id)
	knotwebchat.BroadcastCommandEvent()

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSlashCommandUpdate,
		fmt.Sprintf("Updated slash command %s", command.Name),
		&map[string]interface{}{
			"agent":            r.UserAgent(),
			"IP":               r.RemoteAddr,
			"X-Forwarded-For":  r.Header.Get("X-Forwarded-For"),
			"command_id":       command.Id,
			"command_name":     command.Name,
			"is_user_command":  command.IsUserCommand(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteCommand(w http.ResponseWriter, r *http.Request) {
	commandIdOrName := r.PathValue("command_id")
	user := r.Context().Value("user").(*model.User)
	cfg := config.GetServerConfig()
	db := database.GetInstance()

	var command *model.Command
	var err error

	if validate.UUID(commandIdOrName) {
		command, err = db.GetCommand(commandIdOrName)
	} else {
		command, err = service.ResolveCommandByName(commandIdOrName, user.Id)
	}

	if err != nil || command.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Command not found"})
		return
	}

	if command.IsManaged {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Cannot delete managed command"})
		return
	}

	if !cfg.LeafNode {
		if command.IsUserCommand() {
			if command.UserId != user.Id && !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this command"})
				return
			}
			if command.UserId == user.Id && !user.HasPermission(model.PermissionManageOwnSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete own commands"})
				return
			}
		} else {
			if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
				rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete global commands"})
				return
			}
		}
	}

	commandName := command.Name
	commandId := command.Id
	command.Name = command.Id
	command.IsDeleted = true
	command.UpdatedUserId = user.Id
	command.UpdatedAt = hlc.Now()

	err = db.SaveCommand(command, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipCommand(command)
	sse.PublishSlashCommandsDeleted(command.Id)
	knotwebchat.BroadcastCommandEvent()

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventSlashCommandDelete,
		fmt.Sprintf("Deleted slash command %s", commandName),
		&map[string]interface{}{
			"agent":            r.UserAgent(),
			"IP":               r.RemoteAddr,
			"X-Forwarded-For":  r.Header.Get("X-Forwarded-For"),
			"command_id":       commandId,
			"command_name":     commandName,
			"is_user_command":  command.IsUserCommand(),
		},
	)

	w.WriteHeader(http.StatusOK)
}

// splitFrontmatter separates the YAML frontmatter block from the markdown
// body. If no frontmatter is present, returns ("", content). Mirrors the
// webchat command-file parser so the body stored in the DB matches what the
// LLM sees when a /command is expanded.
func splitFrontmatter(content string) (frontmatter string, body string) {
	trimmed := content
	for len(trimmed) > 0 && (trimmed[0] == '\n' || trimmed[0] == '\r' || trimmed[0] == ' ') {
		trimmed = trimmed[1:]
	}

	if len(trimmed) < 3 || trimmed[:3] != "---" {
		return "", content
	}

	// Find the closing delimiter
	idx := -1
	lines := splitLines(trimmed)
	if len(lines) > 0 && lines[0] == "---" {
		for i := 1; i < len(lines); i++ {
			if lines[i] == "---" {
				idx = i
				break
			}
		}
	}

	if idx == -1 {
		return "", content
	}

	fmEnd := 0
	for i := 0; i <= idx; i++ {
		fmEnd += len(lines[i])
		if i < idx {
			fmEnd++
		}
	}

	if fmEnd+1 > len(trimmed) {
		return trimmed, ""
	}
	return trimmed[:fmEnd], trimmed[fmEnd+1:]
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		line := s[start:]
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		lines = append(lines, line)
	}
	return lines
}
