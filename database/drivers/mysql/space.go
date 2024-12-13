package driver_mysql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) getAltNames(id string) ([]string, error) {
	var altNames []string

	rows, err := db.connection.Query("SELECT name FROM spaces WHERE parent_space_id = ? ORDER BY name ASC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		err := rows.Scan(&name)
		if err != nil {
			return nil, err
		}

		altNames = append(altNames, name)
	}

	return altNames, nil
}

func (db *MySQLDriver) SaveSpace(space *model.Space) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// JSON encode volume data
	volumeData, _ := json.Marshal(space.VolumeData)
	volumeSizes, _ := json.Marshal(space.VolumeSizes)

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM spaces WHERE space_id=?)", space.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		_, err = tx.Exec("UPDATE spaces SET name=?, template_id=?, updated_at=?, shell=?, is_deployed=?, is_pending=?, is_deleting=?, volume_data=?, volume_sizes=?, nomad_namespace=?, container_id=?, template_hash=?, location=? WHERE space_id=?",
			space.Name, space.TemplateId, time.Now().UTC(), space.Shell, space.IsDeployed, space.IsPending, space.IsDeleting, volumeData, volumeSizes, space.NomadNamespace, space.ContainerId, space.TemplateHash, space.Location, space.Id,
		)
	} else {
		_, err = tx.Exec("INSERT INTO spaces (space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, volume_sizes, nomad_namespace, container_id, template_hash, location, ssh_host_signer) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			space.Id, space.UserId, space.TemplateId, space.Name, time.Now().UTC(), time.Now().UTC(), space.Shell, space.IsDeployed, space.IsPending, space.IsDeleting, volumeData, volumeSizes, space.NomadNamespace, space.ContainerId, space.TemplateHash, space.Location, space.SSHHostSigner,
		)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	// Get the current list of alt names
	altNames, err := db.getAltNames(space.Id)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Remove any alt names that are no longer in the list
	for _, altName := range altNames {
		found := false
		for _, name := range space.AltNames {
			if altName == name {
				found = true
				break
			}
		}
		if !found {
			_, err = tx.Exec("DELETE FROM spaces WHERE parent_space_id = ? AND name = ?", space.Id, altName)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	// Add any new alt names
	for _, name := range space.AltNames {
		found := false
		for _, altName := range altNames {
			if altName == name {
				found = true

				// Update the location
				_, err = tx.Exec("UPDATE spaces SET location=? WHERE parent_space_id = ? AND name = ?", space.Location, space.Id, name)
				if err != nil {
					tx.Rollback()
					return err
				}

				break
			}
		}
		if !found {
			altId, err := uuid.NewV7()
			if err != nil {
				tx.Rollback()
				return err
			}

			_, err = tx.Exec("INSERT INTO spaces (space_id, parent_space_id, user_id, name, location) VALUES (?, ?, ?, ?, ?)", altId, space.Id, space.UserId, name, space.Location)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteSpace(space *model.Space) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	_, err = db.connection.Exec("DELETE FROM spaces WHERE space_id = ?", space.Id)
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = db.connection.Exec("DELETE FROM spaces WHERE parent_space_id = ?", space.Id)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) getRow(rows *sql.Rows) (*model.Space, string, error) {
	var space = &model.Space{}
	var createdAt sql.NullString
	var updatedAt sql.NullString
	var volumeData []byte
	var volumeSizes []byte
	var parentId string

	err := rows.Scan(&space.Id, &space.UserId, &space.TemplateId, &space.Name, &createdAt, &updatedAt, &space.Shell, &space.IsDeployed, &space.IsPending, &space.IsDeleting, &volumeData, &space.NomadNamespace, &space.ContainerId, &space.TemplateHash, &volumeSizes, &parentId, &space.Location, &space.SSHHostSigner)
	if err != nil {
		return nil, "", err
	}

	if parentId == "" {
		if createdAt.Valid {

			// Parse the createdAt date
			space.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAt.String)
			if err != nil {
				return nil, "", err
			}
		}

		if updatedAt.Valid {
			// Parse the updatedAt date
			space.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAt.String)
			if err != nil {
				return nil, "", err
			}
		}

		// Decode volume data
		err = json.Unmarshal(volumeData, &space.VolumeData)
		if err != nil {
			return nil, "", err
		}

		err = json.Unmarshal(volumeSizes, &space.VolumeSizes)
		if err != nil {
			return nil, "", err
		}
	}

	return space, parentId, nil
}

func (db *MySQLDriver) getSpaces(query string, args ...interface{}) ([]*model.Space, error) {
	var spaces []*model.Space

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		space, _, err := db.getRow(rows)
		if err != nil {
			return nil, err
		}

		spaces = append(spaces, space)
	}

	return spaces, nil
}

func (db *MySQLDriver) GetSpace(id string) (*model.Space, error) {
	spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE space_id = ? AND parent_space_id = ''", id)
	if err != nil {
		return nil, err
	}
	if len(spaces) == 0 {
		return nil, fmt.Errorf("space not found")
	}

	// Load the alt names
	spaces[0].AltNames, err = db.getAltNames(spaces[0].Id)
	if err != nil {
		return nil, err
	}

	return spaces[0], nil
}

func (db *MySQLDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
	spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE user_id = ? AND parent_space_id = '' ORDER BY name ASC", userId)
	if err != nil {
		return nil, err
	}

	return spaces, nil
}

func (db *MySQLDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
	var space *model.Space = nil
	var parentId string
	var err error

	rows, err := db.connection.Query("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE user_id = ? AND name = ?", userId, spaceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		space, parentId, err = db.getRow(rows)
		if err != nil {
			return nil, err
		}

		// If has a parent ID then load the parent space
		if parentId != "" {
			rows2, err := db.connection.Query("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE space_id = ?", parentId)
			if err != nil {
				return nil, err
			}
			defer rows2.Close()

			if rows2.Next() {
				space, _, err = db.getRow(rows2)
				if err != nil {
					return nil, err
				}
			}
		}
	} else {
		return nil, fmt.Errorf("space not found")
	}

	return space, nil
}

func (db *MySQLDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
	spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE template_id = ? AND parent_space_id = '' ORDER BY name ASC", templateId)
	if err != nil {
		return nil, err
	}

	return spaces, nil
}

func (db *MySQLDriver) GetSpaces() ([]*model.Space, error) {
	spaces, err := db.getSpaces("SELECT space_id, user_id, template_id, name, created_at, updated_at, shell, is_deployed, is_pending, is_deleting, volume_data, nomad_namespace, container_id, template_hash, volume_sizes, parent_space_id, location, ssh_host_signer FROM spaces WHERE parent_space_id = '' ORDER BY name ASC")
	if err != nil {
		return nil, err
	}

	return spaces, nil
}
