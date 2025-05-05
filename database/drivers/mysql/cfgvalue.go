package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/database/model"
)

func (db *MySQLDriver) GetCfgValue(name string) (*model.CfgValue, error) {
	var v []*model.CfgValue

	err := db.read("configs", &v, nil, "name = ? LIMIT 1", name)
	if err != nil {
		return nil, err
	}

	if len(v) == 0 {
		return nil, fmt.Errorf("config not found")
	}

	return v[0], nil
}

func (db *MySQLDriver) SaveCfgValue(cfgValue *model.CfgValue) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM configs WHERE name=?)", cfgValue.Name).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		err = db.update("configs", cfgValue, []string{"Value"})
	} else {
		err = db.create("configs", cfgValue)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}
