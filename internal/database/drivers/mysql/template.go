package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveTemplate(template *model.Template, updateFields []string) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM templates WHERE template_id=?)", template.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if !template.ScheduleEnabled {
		template.Schedule = nil
	}

	// Update
	if doUpdate {
		err = db.update("templates", template, updateFields)
	} else {
		err = db.create("templates", template)
	}
	if err != nil {
		tx.Rollback()
		return err
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
		// Check if any of the spaces are not marked for deletion
		for _, space := range spaces {
			if !space.IsDeleted {
				return fmt.Errorf("template in use")
			}
		}
	}

	_, err = db.connection.Exec("DELETE FROM templates WHERE template_id = ?", template.Id)
	return err
}

func (db *MySQLDriver) GetTemplate(id string) (*model.Template, error) {
	var templates []*model.Template

	err := db.read("templates", &templates, nil, "template_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("template not found")
	}

	return templates[0], nil
}

func (db *MySQLDriver) GetTemplates() ([]*model.Template, error) {
	var templates []*model.Template

	err := db.read("templates", &templates, nil, "1 ORDER BY name")
	return templates, err
}
