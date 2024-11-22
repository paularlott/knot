package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveTemplateVar(templateVar *model.TemplateVar) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	val := templateVar.GetValueEncrypted()

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM templatevars WHERE templatevar_id=?)", templateVar.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		_, err = tx.Exec("UPDATE templatevars SET name=?, location=?, local=?, value=?, protected=?, restricted=?, updated_user_id=?, updated_at=? WHERE templatevar_id=?",
			templateVar.Name, templateVar.Location, templateVar.Local, val, templateVar.Protected, templateVar.Restricted, templateVar.UpdatedUserId, time.Now().UTC(), templateVar.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO templatevars (templatevar_id, name, location, local, value, protected, restricted, created_user_id, created_at, updated_user_id, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			templateVar.Id, templateVar.Name, templateVar.Location, templateVar.Local, val, templateVar.Protected, templateVar.Restricted, templateVar.CreatedUserId, time.Now().UTC(), templateVar.CreatedUserId, time.Now().UTC(),
		)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteTemplateVar(templateVar *model.TemplateVar) error {
	_, err := db.connection.Exec("DELETE FROM templatevars WHERE templatevar_id = ?", templateVar.Id)
	return err
}

func (db *MySQLDriver) getTemplateVars(query string, args ...interface{}) ([]*model.TemplateVar, error) {
	var templateVars []*model.TemplateVar

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var templateVar = &model.TemplateVar{}
		var createdAt string
		var updatedAt string

		err := rows.Scan(&templateVar.Id, &templateVar.Name, &templateVar.Location, &templateVar.Local, &templateVar.Value, &templateVar.Protected, &templateVar.Restricted, &templateVar.CreatedUserId, &createdAt, &templateVar.UpdatedUserId, &updatedAt)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		templateVar.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		templateVar.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			return nil, err
		}

		templateVar.DecryptSetValue(templateVar.Value)
		templateVars = append(templateVars, templateVar)
	}

	return templateVars, nil
}

func (db *MySQLDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
	templateVars, err := db.getTemplateVars("SELECT templatevar_id, name, location, local, value, protected, restricted, created_user_id, created_at, updated_user_id, updated_at FROM templatevars WHERE templatevar_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(templateVars) == 0 {
		return nil, fmt.Errorf("template value not found")
	}

	return templateVars[0], nil
}

func (db *MySQLDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
	return db.getTemplateVars("SELECT templatevar_id, name, location, local, value, protected, restricted, created_user_id, created_at, updated_user_id, updated_at FROM templatevars ORDER BY name")
}
