package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SaveCommand(command *model.Command, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM commands WHERE command_id=?)", command.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("commands", command, updateFields)
	} else {
		err = db.create("commands", command)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *PostgresDriver) DeleteCommand(command *model.Command) error {
	_, err := db.connection.Exec("DELETE FROM commands WHERE command_id = ?", command.Id)
	return err
}

func (db *PostgresDriver) GetCommand(id string) (*model.Command, error) {
	var commands []*model.Command

	err := db.read("commands", &commands, nil, "command_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	return commands[0], nil
}

func (db *PostgresDriver) GetCommands() ([]*model.Command, error) {
	var commands []*model.Command

	err := db.read("commands", &commands, nil, "TRUE ORDER BY name")
	return commands, err
}

func (db *PostgresDriver) GetCommandsByName(name string) ([]*model.Command, error) {
	var commands []*model.Command

	err := db.read("commands", &commands, nil, "name = ? ORDER BY "+zonesLength+" DESC, created_at", name)
	if err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	return commands, nil
}

func (db *PostgresDriver) GetCommandsByNameAndUser(name string, userId string) ([]*model.Command, error) {
	var commands []*model.Command

	err := db.read("commands", &commands, nil, "name = ? AND user_id = ? ORDER BY "+zonesLength+" DESC, created_at", name, userId)
	if err != nil {
		return nil, err
	}
	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	return commands, nil
}
