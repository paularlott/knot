package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

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
		_, err = tx.Exec("UPDATE groups SET name=?, max_spaces=?, compute_units=?, storage_units=?, updated_user_id=?, updated_at=? WHERE group_id=?",
			group.Name, group.MaxSpaces, group.ComputeUnits, group.StorageUnits, group.UpdatedUserId, time.Now().UTC(), group.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO groups (group_id, name, max_spaces, compute_units, storage_units, created_user_id, created_at, updated_user_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			group.Id, group.Name, group.MaxSpaces, group.ComputeUnits, group.StorageUnits, group.CreatedUserId, time.Now().UTC(), group.CreatedUserId, time.Now().UTC(),
		)
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

func (db *MySQLDriver) getGroups(query string, args ...interface{}) ([]*model.Group, error) {
	var groups []*model.Group

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var group = &model.Group{}
		var createdAt string
		var updatedAt string

		err := rows.Scan(&group.Id, &group.Name, &group.MaxSpaces, &group.ComputeUnits, &group.StorageUnits, &group.CreatedUserId, &createdAt, &group.UpdatedUserId, &updatedAt)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		group.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		group.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	return groups, nil
}

func (db *MySQLDriver) GetGroup(id string) (*model.Group, error) {
	groups, err := db.getGroups("SELECT group_id, name, max_spaces, compute_units, storage_units, created_user_id, created_at, updated_user_id, updated_at FROM groups WHERE group_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("user group not found")
	}

	return groups[0], nil
}

func (db *MySQLDriver) GetGroups() ([]*model.Group, error) {
	return db.getGroups("SELECT group_id, name, max_spaces, compute_units, storage_units, created_user_id, created_at, updated_user_id, updated_at FROM groups ORDER BY name")
}
