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

	// Clone the templateVar to avoid modifying the original
	templateVarClone := *templateVar
	templateVarClone.Value = templateVar.GetValueEncrypted()

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM templatevars WHERE templatevar_id=?)", templateVar.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		now := time.Now().UTC()
		templateVarClone.UpdatedAt = now
		err = db.update("templatevars", &templateVarClone, nil)
	} else {
		err = db.create("templatevars", &templateVarClone)
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

func (db *MySQLDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
	var templateVars []*model.TemplateVar

	err := db.read("templatevars", &templateVars, nil, "templatevar_id = ?", id)
	if err != nil {
		return nil, err
	}

	if len(templateVars) == 0 {
		return nil, fmt.Errorf("template value not found")
	}

	// Decrypt the value
	templateVars[0].DecryptSetValue(templateVars[0].Value)

	return templateVars[0], nil
}

func (db *MySQLDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
	var templateVars []*model.TemplateVar

	err := db.read("templatevars", &templateVars, nil, "1 ORDER BY name")
	if err != nil {
		return nil, err
	}

	// Decrypt the values
	for _, templateVar := range templateVars {
		templateVar.DecryptSetValue(templateVar.Value)
	}

	return templateVars, nil
}
