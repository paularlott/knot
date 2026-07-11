package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveCommand(command *model.Command, updateFields []string) error {
	existingCommand, _ := db.GetCommand(command.Id)

	if existingCommand != nil {
		if (existingCommand.Name != command.Name || existingCommand.UserId != command.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
			db.connection.Del(context.Background(), fmt.Sprintf("%sCommandsByName:%s:%s", db.prefix, existingCommand.UserId, existingCommand.Name))
		}

		if len(updateFields) > 0 {
			util.CopyFields(command, existingCommand, updateFields)
			command = existingCommand
		}
	}

	data, err := json.Marshal(command)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sCommands:%s", db.prefix, command.Id), data, 0).Err()
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sCommandsByName:%s:%s", db.prefix, command.UserId, command.Name), command.Id, 0).Err()
}

func (db *RedisDbDriver) DeleteCommand(command *model.Command) error {
	db.connection.Del(context.Background(), fmt.Sprintf("%sCommandsByName:%s:%s", db.prefix, command.UserId, command.Name))
	return db.connection.Del(context.Background(), fmt.Sprintf("%sCommands:%s", db.prefix, command.Id)).Err()
}

func (db *RedisDbDriver) GetCommand(id string) (*model.Command, error) {
	var command = &model.Command{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sCommands:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &command)
	if err != nil {
		return nil, err
	}

	return command, nil
}

func (db *RedisDbDriver) GetCommands() ([]*model.Command, error) {
	var commands []*model.Command

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sCommands:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		command, err := db.GetCommand(iter.Val()[len(fmt.Sprintf("%sCommands:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		commands = append(commands, command)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return commands, nil
}

func (db *RedisDbDriver) GetCommandsByName(name string) ([]*model.Command, error) {
	commands, err := db.GetCommands()
	if err != nil {
		return nil, err
	}

	var result []*model.Command
	for _, command := range commands {
		if command.Name == name && command.UserId == "" {
			result = append(result, command)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (db *RedisDbDriver) GetCommandsByNameAndUser(name string, userId string) ([]*model.Command, error) {
	commands, err := db.GetCommands()
	if err != nil {
		return nil, err
	}

	var result []*model.Command
	for _, command := range commands {
		if command.Name == name && command.UserId == userId {
			result = append(result, command)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}
