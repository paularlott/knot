package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveRole(role *model.Role) error {

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
		_, err = tx.Exec("UPDATE roles SET name=?, permissions=?, updated_user_id=?, updated_at=? WHERE role_id=?",
			role.Name, role.Permissions, role.UpdatedUserId, time.Now().UTC(), role.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO roles (role_id, name, permissions, created_user_id, created_at, updated_user_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			role.Id, role.Name, role.Permissions, role.CreatedUserId, time.Now().UTC(), role.CreatedUserId, time.Now().UTC(),
		)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteRole(role *model.Role) error {
	_, err := db.connection.Exec("DELETE FROM roles WHERE role_id = ?", role.Id)
	return err
}

func (db *MySQLDriver) getRoles(query string, args ...interface{}) ([]*model.Role, error) {
	var roles []*model.Role

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var role = &model.Role{}
		var createdAt string
		var updatedAt string

		err := rows.Scan(&role.Id, &role.Name, &role.Permissions, &role.CreatedUserId, &createdAt, &role.UpdatedUserId, &updatedAt)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		role.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		role.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			return nil, err
		}

		roles = append(roles, role)
	}

	return roles, nil
}

func (db *MySQLDriver) GetRole(id string) (*model.Role, error) {
	roles, err := db.getRoles("SELECT role_id, name, permissions, created_user_id, created_at, updated_user_id, updated_at FROM roles WHERE role_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("user role not found")
	}

	return roles[0], nil
}

func (db *MySQLDriver) GetRoles() ([]*model.Role, error) {
	return db.getRoles("SELECT role_id, name, permissions, created_user_id, created_at, updated_user_id, updated_at FROM roles ORDER BY name")
}
