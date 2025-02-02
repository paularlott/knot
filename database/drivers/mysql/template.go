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
		_, err = tx.Exec("UPDATE templates SET name=?, description=?, job=?, volumes=?, hash=?, updated_user_id=?, updated_at=?, groups=?, local_container=?, is_manual=?, with_terminal=?, with_vscode_tunnel=?, with_code_server=?, with_ssh=?, compute_units=?, storage_units=?, schedule_enabled=?, schedule=?, locations=? WHERE template_id=?",
			template.Name, template.Description, template.Job, template.Volumes, template.Hash, template.UpdatedUserId, time.Now().UTC(), template.Groups, template.LocalContainer, template.IsManual, template.WithTerminal, template.WithVSCodeTunnel, template.WithCodeServer, template.WithSSH, template.ComputeUnits, template.StorageUnits, template.ScheduleEnabled, template.Schedule, template.Locations, template.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO templates (template_id, name, description, job, volumes, hash, created_user_id, created_at, updated_user_id, updated_at, groups, local_container, is_manual, with_terminal, with_vscode_tunnel, with_code_server, with_ssh, compute_units, storage_units, schedule_enabled, schedule, locations) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			template.Id, template.Name, template.Description, template.Job, template.Volumes, template.Hash, template.CreatedUserId, time.Now().UTC(), template.CreatedUserId, time.Now().UTC(), template.Groups, template.LocalContainer, template.IsManual, template.WithTerminal, template.WithVSCodeTunnel, template.WithCodeServer, template.WithSSH, template.ComputeUnits, template.StorageUnits, template.ScheduleEnabled, template.Schedule, template.Locations,
		)
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
		return fmt.Errorf("template in use")
	}

	_, err = db.connection.Exec("DELETE FROM templates WHERE template_id = ?", template.Id)
	return err
}

func (db *MySQLDriver) getTemplates(query string, args ...interface{}) ([]*model.Template, error) {
	var templates []*model.Template

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var template = &model.Template{}
		var createdAt string
		var updatedAt string

		err := rows.Scan(&template.Id, &template.Name, &template.Description, &template.Job, &template.Volumes, &template.Hash, &template.CreatedUserId, &createdAt, &template.UpdatedUserId, &updatedAt, &template.Groups, &template.LocalContainer, &template.IsManual, &template.WithTerminal, &template.WithVSCodeTunnel, &template.WithCodeServer, &template.WithSSH, &template.ComputeUnits, &template.StorageUnits, &template.ScheduleEnabled, &template.Schedule, &template.Locations)
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
	templates, err := db.getTemplates("SELECT template_id, name, description, job, volumes, hash, created_user_id, created_at, updated_user_id, updated_at, groups, local_container, is_manual, with_terminal, with_vscode_tunnel, with_code_server, with_ssh, compute_units, storage_units, schedule_enabled, schedule, locations FROM templates WHERE template_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("template not found")
	}

	return templates[0], nil
}

func (db *MySQLDriver) GetTemplates() ([]*model.Template, error) {
	return db.getTemplates("SELECT template_id, name, description, job, volumes, hash, created_user_id, created_at, updated_user_id, updated_at, groups, local_container, is_manual, with_terminal, with_vscode_tunnel, with_code_server, with_ssh, compute_units, storage_units, schedule_enabled, schedule, locations FROM templates ORDER BY name")
}
