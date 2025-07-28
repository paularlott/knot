package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveGroup(group *model.Group) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE group_id=?)", group.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		err = db.update("groups", group, nil)
	} else {
		err = db.create("groups", group)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteGroup(group *model.Group) error {
	_, err := db.connection.Exec("DELETE FROM groups WHERE group_id = ?", group.Id)
	return err
}

func (db *MySQLDriver) GetGroup(id string) (*model.Group, error) {
	var groups []*model.Group

	err := db.read("groups", &groups, nil, "group_id = ?", id)
	if err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return nil, fmt.Errorf("user group not found")
	}

	return groups[0], nil
}

func (db *MySQLDriver) GetGroups() ([]*model.Group, error) {
	var groups []*model.Group

	err := db.read("groups", &groups, nil, "1 ORDER BY name")
	if err != nil {
		return nil, err
	}

	return groups, nil
}
