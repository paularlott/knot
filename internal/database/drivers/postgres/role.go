package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SaveRole(role *model.Role) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM roles WHERE role_id=?)", role.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		err = db.update("roles", role, nil)
	} else {
		err = db.create("roles", role)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *PostgresDriver) DeleteRole(role *model.Role) error {
	_, err := db.connection.Exec("DELETE FROM roles WHERE role_id = ?", role.Id)
	return err
}

func (db *PostgresDriver) GetRole(id string) (*model.Role, error) {
	var roles []*model.Role

	err := db.read("roles", &roles, nil, "role_id = ?", id)
	if err != nil {
		return nil, err
	}

	if len(roles) == 0 {
		return nil, fmt.Errorf("user role not found")
	}

	return roles[0], nil
}

func (db *PostgresDriver) GetRoles() ([]*model.Role, error) {
	var roles []*model.Role

	err := db.read("roles", &roles, nil, "TRUE ORDER BY name")
	if err != nil {
		return nil, err
	}

	return roles, nil
}
