package driver_mysql

import (
	"database/sql"
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

  // Assume update
  result, err := tx.Exec("UPDATE users SET username=?, email=?, password=?, active=?, updated_at=?, last_login_at=?, is_admin=? WHERE user_id=?", user.Username, user.Email, user.Password, user.Active, time.Now().UTC(), user.LastLoginAt, user.IsAdmin, user.Id)
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO users (user_id, username, email, password, active, updated_at, created_at, is_admin) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", user.Id, user.Username, user.Email, user.Password, user.Active, time.Now().UTC(), time.Now().UTC(), user.IsAdmin)
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteUser(user *model.User) error {
  _, err := db.connection.Exec("DELETE FROM users WHERE user_id = ?", user.Id)
  return err
}

func (db *MySQLDriver) getUser(by string, value string) (*model.User, error) {
  var user model.User
  var updatedAt string
  var createdAt string
  var lastLoginAt sql.NullString

  row := db.connection.QueryRow(fmt.Sprintf("SELECT user_id, username, email, password, active, updated_at, created_at, last_login_at, is_admin FROM users where %s = ?", by), value)
  if row == nil {
    return nil, fmt.Errorf("user not found")
  }

  err := row.Scan(&user.Id, &user.Username, &user.Email, &user.Password, &user.Active, &updatedAt, &createdAt, &lastLoginAt, &user.IsAdmin)
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
    user.LastLoginAt, err = time.Parse("2006-01-02 15:04:05", lastLoginAt.String)
    if err != nil {
      return nil, err
    }
  }

  return &user, nil
}

func (db *MySQLDriver) GetUser(id string) (*model.User, error) {
  return db.getUser("user_id", id)
}

func (db *MySQLDriver) GetUserByEmail(email string) (*model.User, error) {
  return db.getUser("email", email)
}

func (db *MySQLDriver) GetUsers() ([]*model.User, error) {
  var users []*model.User

  rows, err := db.connection.Query("SELECT user_id, username, email, password, active, updated_at, created_at, last_login_at, is_admin FROM users ORDER BY username ASC")
  if err != nil {
    return nil, err
  }

  for rows.Next() {
    var user model.User
    var updatedAt string
    var createdAt string
    var lastLoginAt sql.NullString

    err = rows.Scan(&user.Id, &user.Username, &user.Email, &user.Password, &user.Active, &updatedAt, &createdAt, &lastLoginAt, &user.IsAdmin)
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
      user.LastLoginAt, err = time.Parse("2006-01-02 15:04:05", lastLoginAt.String)
      if err != nil {
        return nil, err
      }
    }

    users = append(users, &user)
  }

  return users, nil
}

func (db *MySQLDriver) GetUserCount() (int, error) {
  var count int

  row := db.connection.QueryRow("SELECT COUNT(*) FROM users")
  if row == nil {
    return 0, fmt.Errorf("failed to get user count")
  }

  err := row.Scan(&count)
  if err != nil {
    return 0, err
  }

  return count, nil
}
