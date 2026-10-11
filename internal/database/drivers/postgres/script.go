package driver_postgres

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *PostgresDriver) SaveScript(script *model.Script, updateFields []string) error {
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

func (db *PostgresDriver) DeleteScript(script *model.Script) error {
	_, err := db.connection.Exec("DELETE FROM scripts WHERE script_id = ?", script.Id)
	return err
}

func (db *PostgresDriver) GetScript(id string) (*model.Script, error) {
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

func (db *PostgresDriver) GetScripts() ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "TRUE ORDER BY name")
	return scripts, err
}

func (db *PostgresDriver) GetScriptByName(name string) (*model.Script, error) {
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

// GetScriptsByName returns all scripts matching a name (for zone-specific overrides)
func (db *PostgresDriver) GetScriptsByName(name string) ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "name = ? ORDER BY "+zonesLength+" DESC, created_at", name)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	return scripts, nil
}

// GetScriptByNameAndUser gets a script by name for a specific user
// If userId is empty, it searches for global scripts
// This supports the user script override functionality
func (db *PostgresDriver) GetScriptByNameAndUser(name string, userId string) (*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "name = ? AND user_id = ?", name, userId)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	return scripts[0], nil
}

// GetScriptsByNameAndUser returns all scripts matching a name and user_id (for zone-specific overrides)
func (db *PostgresDriver) GetScriptsByNameAndUser(name string, userId string) ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.read("scripts", &scripts, nil, "name = ? AND user_id = ? ORDER BY "+zonesLength+" DESC, created_at", name, userId)
	if err != nil {
		return nil, err
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	return scripts, nil
}
