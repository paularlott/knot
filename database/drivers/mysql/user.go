package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveUser(user *model.User, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
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
		err = db.update("users", user, updateFields)
		if err != nil {
			tx.Rollback()
			return err
		}
	} else {
		err = db.create("users", user)
		if err != nil {
			tx.Rollback()
			return err
		}
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

func (db *MySQLDriver) GetUser(id string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "user_id=?", id)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *MySQLDriver) GetUserByEmail(email string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "email=?", email)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *MySQLDriver) GetUserByUsername(name string) (*model.User, error) {
	var users []model.User
	err := db.read("users", &users, nil, "username=?", name)
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("user not found")
	}

	return &users[0], nil
}

func (db *MySQLDriver) GetUsers() ([]*model.User, error) {
	var users []*model.User
	err := db.read("users", &users, nil, "1")
	if err != nil {
		fmt.Println(err)
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
