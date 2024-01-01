package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveTemplate(template *model.Template) error {

  tx, err := db.connection.Begin()
  if err != nil {
    return err
  }

  // Assume update
  result, err := tx.Exec("UPDATE templates SET name=?, job=?, updated_user_id=?, updated_at=? WHERE template_id=?",
    template.Name, template.Job, template.UpdatedUserId, time.Now().UTC(), template.Id,
  )
  if err != nil {
    tx.Rollback()
    return err
  }

  // If no rows were updated then do an insert
  if rows, _ := result.RowsAffected(); rows == 0 {
    _, err = tx.Exec("INSERT INTO templates (template_id, name, job, created_user_id, created_at, updated_user_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
      template.Id, template.Name, template.Job, template.CreatedUserId, time.Now().UTC(), template.CreatedUserId, time.Now().UTC(),
    )
    if err != nil {
      tx.Rollback()
      return err
    }
  }

  tx.Commit()

  return nil
}

func (db *MySQLDriver) DeleteTemplate(template *model.Template) error {

  // Test if the space in in use
  spaces, err := db.GetSpacesByTemplateId(template.Id)
  if err != nil {
    return err
  }

  if len(spaces) > 0 {
    return fmt.Errorf("template in use")
  }

  _, err = db.connection.Exec("DELETE FROM templates WHERE template_id = ?", template.Id)
  return err
}

func (db *MySQLDriver) getTemplates(query string, args ...interface{}) ([]*model.Template, error) {
  var templates []*model.Template

  rows, err := db.connection.Query(query, args ...)
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  for rows.Next() {
    var template = &model.Template{}
    var createdAt string
    var updatedAt string

    err := rows.Scan(&template.Id, &template.Name, &template.Job, &template.CreatedUserId, &createdAt, &template.UpdatedUserId, &updatedAt)
    if err != nil {
      return nil, err
    }

    // Parse the dates
    template.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
    if err != nil {
      return nil, err
    }
    template.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
    if err != nil {
      return nil, err
    }

    templates = append(templates, template)
  }

  return templates, nil
}

func (db *MySQLDriver) GetTemplate(id string) (*model.Template, error) {
  templates, err := db.getTemplates("SELECT template_id, name, job, created_user_id, created_at, updated_user_id, updated_at FROM templates WHERE template_id = ?", id)
  if err != nil {
    return nil, err
  }
  if len(templates) == 0 {
    return nil, fmt.Errorf("template not found")
  }

  return templates[0], nil
}

func (db *MySQLDriver) GetTemplates() ([]*model.Template, error) {
  return db.getTemplates("SELECT template_id, name, job, created_user_id, created_at, updated_user_id, updated_at FROM templates ORDER BY name")
}

func (db *MySQLDriver) GetTemplateOptionList() (map[string]string, error) {
  var optionList = make(map[string]string)
  optionList[""] = "None (Manual Deploy)"

  rows, err := db.connection.Query("SELECT template_id, name FROM templates ORDER BY name")
  if err != nil {
    return nil, err
  }
  defer rows.Close()

  for rows.Next() {
    var id string
    var name string

    err := rows.Scan(&id, &name)
    if err != nil {
      return nil, err
    }

    optionList[id] = name
  }

  return optionList, nil
}
