package driver_mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveUser(user *model.User) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Convert roles array to JSON
	roles, err := json.Marshal(user.Roles)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE user_id=?)", user.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		_, err = tx.Exec("UPDATE users SET email=?, password=?, active=?, updated_at=?, last_login_at=?, ssh_public_key=?, roles=?, groups=?, preferred_shell=?, timezone=?, max_spaces=?, compute_units=?, storage_units=?, max_tunnels=?, service_password=?, github_username=?, totp_secret=? WHERE user_id=?",
			user.Email, user.Password, user.Active, time.Now().UTC(), user.LastLoginAt, user.SSHPublicKey, roles, user.Groups, user.PreferredShell, user.Timezone, user.MaxSpaces, user.ComputeUnits, user.StorageUnits, user.MaxTunnels, user.ServicePassword, user.GitHubUsername, user.TOTPSecret, user.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO users (user_id, username, email, password, active, updated_at, created_at, ssh_public_key, preferred_shell, roles, groups, timezone, max_spaces, compute_units, storage_units, max_tunnels, service_password, github_username, totp_secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			user.Id, user.Username, user.Email, user.Password, user.Active, time.Now().UTC(), time.Now().UTC(), user.SSHPublicKey, user.PreferredShell, roles, user.Groups, user.Timezone, user.MaxSpaces, user.ComputeUnits, user.StorageUnits, user.MaxTunnels, user.ServicePassword, user.GitHubUsername, user.TOTPSecret,
		)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteUser(user *model.User) error {
	_, err := db.connection.Exec("DELETE FROM users WHERE user_id = ?", user.Id)
	return err
}

func (db *MySQLDriver) getUsers(where string, args ...interface{}) ([]*model.User, error) {
	var users []*model.User
	var updatedAt string
	var createdAt string
	var lastLoginAt sql.NullString
	var roles string

	if where != "" {
		where = "WHERE " + where
	}

	rows, err := db.connection.Query(fmt.Sprintf("SELECT user_id, username, email, password, active, updated_at, created_at, last_login_at, ssh_public_key, preferred_shell, roles, groups, timezone, max_spaces, compute_units, storage_units, max_tunnels, service_password, github_username, totp_secret FROM users %s ORDER BY username ASC", where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var user = &model.User{}

		err := rows.Scan(&user.Id, &user.Username, &user.Email, &user.Password, &user.Active, &updatedAt, &createdAt, &lastLoginAt, &user.SSHPublicKey, &user.PreferredShell, &roles, &user.Groups, &user.Timezone, &user.MaxSpaces, &user.ComputeUnits, &user.StorageUnits, &user.MaxTunnels, &user.ServicePassword, &user.GitHubUsername, &user.TOTPSecret)
		if err != nil {
			return nil, err
		}

		// Parse roles
		err = json.Unmarshal([]byte(roles), &user.Roles)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		user.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			return nil, err
		}
		user.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}

		if lastLoginAt.Valid {
			parsedTime, err := time.Parse("2006-01-02 15:04:05", lastLoginAt.String)
			if err != nil {
				return nil, err
			}
			user.LastLoginAt = &parsedTime
		} else {
			user.LastLoginAt = nil
		}

		users = append(users, user)
	}

	return users, nil
}

func (db *MySQLDriver) GetUser(id string) (*model.User, error) {
	users, err := db.getUsers("user_id=?", id)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return users[0], nil
}

func (db *MySQLDriver) GetUserByEmail(email string) (*model.User, error) {
	users, err := db.getUsers("email=?", email)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return users[0], nil
}

func (db *MySQLDriver) GetUserByUsername(name string) (*model.User, error) {
	users, err := db.getUsers("username=?", name)
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return users[0], nil
}

func (db *MySQLDriver) GetUsers() ([]*model.User, error) {
	users, err := db.getUsers("")
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (db *MySQLDriver) HasUsers() (bool, error) {
	var count int

	row := db.connection.QueryRow("SELECT COUNT(*) FROM users")
	if row == nil {
		return false, fmt.Errorf("failed to get user count")
	}

	err := row.Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
