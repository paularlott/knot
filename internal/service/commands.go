package service

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

type CommandService struct{}

type CommandListOptions struct {
	FilterUserId         string
	User                 *model.User
	IncludeDeleted       bool
	CheckZoneRestriction bool
}

var commandService *CommandService

func GetCommandService() *CommandService {
	if commandService == nil {
		commandService = &CommandService{}
	}
	return commandService
}

func (s *CommandService) ListCommands(opts CommandListOptions) ([]*model.Command, error) {
	db := database.GetInstance()
	commands, err := db.GetCommands()
	if err != nil {
		return nil, fmt.Errorf("failed to get commands: %v", err)
	}

	cfg := config.GetServerConfig()
	var result []*model.Command

	for _, command := range commands {
		if command.IsDeleted && !opts.IncludeDeleted {
			continue
		}

		isUserCommands := opts.FilterUserId != ""

		if isUserCommands {
			if command.UserId != opts.FilterUserId {
				continue
			}
		} else {
			if command.UserId != "" {
				continue
			}

			if opts.User != nil && !opts.User.HasPermission(model.PermissionManageGlobalSlashCommands) {
				if len(command.Groups) > 0 && !opts.User.HasAnyGroup(&command.Groups) {
					continue
				}
			}
		}

		if opts.CheckZoneRestriction && !command.IsValidForZone(cfg.Zone) {
			continue
		}

		result = append(result, command)
	}

	return result, nil
}
