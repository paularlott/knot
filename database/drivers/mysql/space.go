package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveSpace(space *model.Space) error {

  tx, err := db.connection.Begin()
  if err != nil {
    return err
  }

  // Assume update
  result, err := tx.Exec("UPDATE spaces SET template_id=?, name=?, agent_url=?, updated_at=? WHERE space_id=?",
    space.TemplateId, space.Name, space.AgentURL, time.Now().UTC(), space.Id,
  )
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO spaces (space_id, user_id, template_id, name, agent_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
      space.Id, space.UserId, space.TemplateId, space.Name, space.AgentURL, time.Now().UTC(), time.Now().UTC(),
    )
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteSpace(space *model.Space) error {
  _, err := db.connection.Exec("DELETE FROM spaces WHERE space_id = ?", space.Id)
  return err
}

func (db *MySQLDriver) GetSpace(id string) (*model.Space, error) {
  var space = &model.Space{}
  var createdAt string
  var updatedAt string

  row := db.connection.QueryRow("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at FROM spaces WHERE space_id = ?", id)
  if row == nil {
    return nil, fmt.Errorf("agent not found")
  }

  err := row.Scan(&space.Id, &space.UserId, &space.TemplateId, &space.Name, &space.AgentURL, &createdAt, &updatedAt)
  if err != nil {
    return nil, err
  }

  // Parse the dates
  space.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
  if err != nil {
    return nil, err
  }
  space.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
  if err != nil {
    return nil, err
  }

  return space, nil
}

func (db *MySQLDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
  var spaces []*model.Space

  rows, err := db.connection.Query("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at FROM spaces WHERE user_id = ? ORDER BY name ASC", userId)
  if err != nil {
    return nil, err
  }

  for rows.Next() {
    var space = &model.Space{}
    var createdAt string
    var updatedAt string

    err := rows.Scan(&space.Id, &space.UserId, &space.TemplateId, &space.Name, &space.AgentURL, &createdAt, &updatedAt)
    if err != nil {
      return nil, err
    }

    // Parse the dates
    space.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
    if err != nil {
      return nil, err
    }
    space.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
    if err != nil {
      return nil, err
    }

    spaces = append(spaces, space)
  }

  return spaces, nil
}

func (db *MySQLDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
  var space = &model.Space{}
  var createdAt string
  var updatedAt string

  row := db.connection.QueryRow("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at FROM spaces WHERE userId = ? AND name = ?", userId, spaceName)
  if row == nil {
    return nil, fmt.Errorf("agent not found")
  }

  err := row.Scan(&space.Id, &space.UserId, &space.TemplateId, &space.Name, &space.AgentURL, &createdAt, &updatedAt)
  if err != nil {
    return nil, err
  }

  // Parse the dates
  space.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
  if err != nil {
    return nil, err
  }
  space.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
  if err != nil {
    return nil, err
  }

  return space, nil
}
