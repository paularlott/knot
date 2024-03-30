package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveVolume(volume *model.Volume) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Assume update
	result, err := tx.Exec("UPDATE volumes SET name=?, definition=?, updated_user_id=?, updated_at=?, active=?, location=? WHERE volume_id=?",
		volume.Name, volume.Definition, volume.UpdatedUserId, time.Now().UTC(), volume.Active, volume.Location, volume.Id,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	// If no rows were updated then do an insert
	if rows, _ := result.RowsAffected(); rows == 0 {
		_, err = tx.Exec("INSERT INTO volumes (volume_id, name, definition, created_user_id, created_at, updated_user_id, updated_at, active, location) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			volume.Id, volume.Name, volume.Definition, volume.CreatedUserId, time.Now().UTC(), volume.CreatedUserId, time.Now().UTC(), volume.Active, volume.Location,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteVolume(volume *model.Volume) error {
	_, err := db.connection.Exec("DELETE FROM volumes WHERE volume_id = ?", volume.Id)
	return err
}

func (db *MySQLDriver) getVolumes(query string, args ...interface{}) ([]*model.Volume, error) {
	var volumes []*model.Volume

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var volume = &model.Volume{}
		var createdAt string
		var updatedAt string

		err := rows.Scan(&volume.Id, &volume.Name, &volume.Definition, &volume.Active, &volume.CreatedUserId, &createdAt, &volume.UpdatedUserId, &updatedAt, &volume.Location)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		volume.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt)
		if err != nil {
			return nil, err
		}
		volume.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt)
		if err != nil {
			return nil, err
		}

		volumes = append(volumes, volume)
	}

	return volumes, nil
}

func (db *MySQLDriver) GetVolume(id string) (*model.Volume, error) {
	templates, err := db.getVolumes("SELECT volume_id, name, definition, active, created_user_id, created_at, updated_user_id, updated_at, location FROM volumes WHERE volume_id = ?", id)
	if err != nil {
		return nil, err
	}
	if len(templates) == 0 {
		return nil, fmt.Errorf("volume not found")
	}

	return templates[0], nil
}

func (db *MySQLDriver) GetVolumes() ([]*model.Volume, error) {
	return db.getVolumes("SELECT volume_id, name, definition, active, created_user_id, created_at, updated_user_id, updated_at, location FROM volumes ORDER BY name")
}
