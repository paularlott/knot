package driver_mysql

import (
	"encoding/json"
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

  // JSON encode volume data
  volumeData, _ := json.Marshal(space.VolumeData)

  // Assume update
  result, err := tx.Exec("UPDATE spaces SET name=?, template_id=?, agent_url=?, updated_at=?, shell=?, is_deployed=?, volume_data=?, nomad_namespace=?, nomad_job_id=?, template_hash=? WHERE space_id=?",
    space.Name, space.TemplateId, space.AgentURL, time.Now().UTC(), space.Shell, space.IsDeployed, volumeData, space.NomadNamespace, space.NomadJobId, space.TemplateHash, space.Id,
  )
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO spaces (space_id, user_id, template_id, name, agent_url, created_at, updated_at, shell, is_deployed, volume_data, nomad_namespace, nomad_job_id, template_hash) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
      space.Id, space.UserId, space.TemplateId, space.Name, space.AgentURL, time.Now().UTC(), time.Now().UTC(), space.Shell, space.IsDeployed, volumeData, space.NomadNamespace, space.NomadJobId, space.TemplateHash,
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

func (db *MySQLDriver) getSpaces(query string, args ...interface{}) ([]*model.Space, error) {
  var spaces []*model.Space

  rows, err := db.connection.Query(query, args ...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  for rows.Next() {
    var space = &model.Space{}
    var createdAt string
    var updatedAt string
    var volumeData []byte

    err := rows.Scan(&space.Id, &space.UserId, &space.TemplateId, &space.Name, &space.AgentURL, &createdAt, &updatedAt, &space.Shell, &space.IsDeployed, &volumeData, &space.NomadNamespace, &space.NomadJobId, &space.TemplateHash)
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

    // Decode volume data
    err = json.Unmarshal(volumeData, &space.VolumeData)
    if err != nil {
      return nil, err
    }

    spaces = append(spaces, space)
  }

  return spaces, nil
}

func (db *MySQLDriver) GetSpace(id string) (*model.Space, error) {
  spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at, shell, is_deployed, volume_data, nomad_namespace, nomad_job_id, template_hash FROM spaces WHERE space_id = ?", id)
  if err != nil {
    return nil, err
  }
  if len(spaces) == 0 {
    return nil, fmt.Errorf("space not found")
  }

  return spaces[0], nil
}

func (db *MySQLDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
  spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at, shell, is_deployed, volume_data, nomad_namespace, nomad_job_id, template_hash FROM spaces WHERE user_id = ? ORDER BY name ASC", userId)
  if err != nil {
    return nil, err
  }

  return spaces, nil
}

func (db *MySQLDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
  spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at, shell, is_deployed, volume_data, nomad_namespace, nomad_job_id, template_hash FROM spaces WHERE user_id = ? AND name = ?", userId, spaceName)
  if err != nil {
    return nil, err
  }
  if len(spaces) == 0 {
    return nil, fmt.Errorf("space not found")
  }

  return spaces[0], nil
}

func (db *MySQLDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
  spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, agent_url, created_at, updated_at, shell, is_deployed, volume_data, nomad_namespace, nomad_job_id, template_hash FROM spaces WHERE template_id = ? ORDER BY name ASC", templateId)
  if err != nil {
    return nil, err
  }

  return spaces, nil
}
