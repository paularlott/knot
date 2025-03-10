package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveVolume(volume *model.Volume, updateFields []string) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM volumes WHERE volume_id=?)", volume.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		now := time.Now().UTC()
		volume.UpdatedAt = now
		err = db.update("volumes", volume, updateFields)
	} else {
		err = db.create("volumes", volume)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteVolume(volume *model.Volume) error {
	_, err := db.connection.Exec("DELETE FROM volumes WHERE volume_id = ?", volume.Id)
	return err
}

func (db *MySQLDriver) GetVolume(id string) (*model.Volume, error) {
	var volumes []*model.Volume

	err := db.read("volumes", &volumes, nil, "volume_id = ?", id)
	if err != nil {
		return nil, err
	}

	if len(volumes) == 0 {
		return nil, fmt.Errorf("volume not found")
	}

	return volumes[0], nil
}

func (db *MySQLDriver) GetVolumes() ([]*model.Volume, error) {
	var volumes []*model.Volume

	err := db.read("volumes", &volumes, nil, "1 ORDER BY name")
	if err != nil {
		return nil, err
	}

	return volumes, nil
}
