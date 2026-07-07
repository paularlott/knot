package service

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

func ResolveCommandByName(name string, userId string) (*model.Command, error) {
	db := database.GetInstance()
	cfg := config.GetServerConfig()

	if userId != "" {
		commands, err := db.GetCommandsByNameAndUser(name, userId)
		if err == nil {
			for _, command := range commands {
				if !command.IsDeleted && command.IsValidForZone(cfg.Zone) {
					return command, nil
				}
			}
		}
	}

	commands, err := db.GetCommandsByName(name)
	if err != nil {
		return nil, fmt.Errorf("command not found")
	}

	for _, command := range commands {
		if command.IsDeleted {
			continue
		}

		if !command.IsValidForZone(cfg.Zone) {
			continue
		}

		return command, nil
	}

	return nil, fmt.Errorf("command not found")
}

func CanUserAccessCommand(user *model.User, command *model.Command) bool {
	if command.IsUserCommand() {
		if command.UserId == user.Id {
			return user.HasPermission(model.PermissionManageOwnSlashCommands)
		}
		return false
	}

	if !user.HasPermission(model.PermissionManageGlobalSlashCommands) {
		if len(command.Groups) > 0 && !user.HasAnyGroup(&command.Groups) {
			return false
		}
	}

	return true
}
