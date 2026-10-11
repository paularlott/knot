package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SaveTemplate(template *model.Template, updateFields []string) error {

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
		err = db.updateWithExecutor(tx, "templates", template, updateFields)
	} else {
		err = db.createWithExecutor(tx, "templates", template)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (db *PostgresDriver) DeleteTemplate(template *model.Template) error {

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

func (db *PostgresDriver) GetTemplate(id string) (*model.Template, error) {
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

func (db *PostgresDriver) GetTemplates() ([]*model.Template, error) {
	var templates []*model.Template

	err := db.read("templates", &templates, nil, "TRUE ORDER BY name")
	return templates, err
}

func (db *PostgresDriver) GetTemplateByName(name string) (*model.Template, error) {
	var templates []*model.Template

	err := db.read("templates", &templates, nil, "name = ?", name)
	if err != nil {
		return nil, err
	}

	if len(templates) == 0 {
		return nil, fmt.Errorf("template not found")
	}

	return templates[0], nil
}
