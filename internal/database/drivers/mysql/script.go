package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveScript(script *model.Script, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM scripts WHERE script_id=?)", script.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	if doUpdate {
		err = db.update("scripts", script, updateFields)
	} else {
		err = db.create("scripts", script)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()
	return nil
}

func (db *MySQLDriver) DeleteScript(script *model.Script) error {
	_, err := db.connection.Exec("DELETE FROM scripts WHERE script_id = ?", script.Id)
	return err
}

func (db *MySQLDriver) GetScript(id string) (*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "script_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	return scripts[0], nil
}

func (db *MySQLDriver) GetScripts() ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "1 ORDER BY name")
	return scripts, err
}

func (db *MySQLDriver) GetScriptByName(name string) (*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "name = ?", name)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	return scripts[0], nil
}
