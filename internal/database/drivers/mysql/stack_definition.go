package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *MySQLDriver) SaveStackDefinition(def *model.StackDefinition, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM stack_definitions WHERE stack_definition_id=?)", def.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("stack_definitions", def, updateFields)
	} else {
		err = db.create("stack_definitions", def)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *MySQLDriver) DeleteStackDefinition(def *model.StackDefinition) error {
	_, err := db.connection.Exec("DELETE FROM stack_definitions WHERE stack_definition_id = ?", def.Id)
	return err
}

func (db *MySQLDriver) GetStackDefinition(id string) (*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	err := db.read("stack_definitions", &defs, nil, "stack_definition_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("stack definition not found")
	}

	return defs[0], nil
}

func (db *MySQLDriver) GetStackDefinitions() ([]*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	err := db.read("stack_definitions", &defs, nil, "is_deleted = 0 ORDER BY name")
	return defs, err
}

func (db *MySQLDriver) GetStackDefinitionsByUserId(userId string) ([]*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	err := db.read("stack_definitions", &defs, nil, "user_id = ? AND is_deleted = 0 ORDER BY name", userId)
	return defs, err
}

func (db *MySQLDriver) GetStackDefinitionByName(name string, userId string) (*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	err := db.read("stack_definitions", &defs, nil, "name = ? AND user_id = ? AND is_deleted = 0", name, userId)
	if err != nil {
		return nil, err
	}
	if len(defs) == 0 {
		return nil, fmt.Errorf("stack definition not found")
	}

	return defs[0], nil
}
