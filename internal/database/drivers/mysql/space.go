package driver_mysql

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/util"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func (db *MySQLDriver) getAltNames(id string) ([]string, error) {
	var altNames []string
	var spaces []*model.Space
	err := db.read("spaces", &spaces, []string{"Name"}, "parent_space_id = ? ORDER BY name ASC", id)
	if err != nil {
		return nil, err
	}

	for _, space := range spaces {
		altNames = append(altNames, space.Name)
	}
	return altNames, nil
}

func (db *MySQLDriver) SaveSpace(space *model.Space, updateFields []string) error {
	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	var existingSpaces []model.Space
	err = db.read("spaces", &existingSpaces, []string{"Name"}, "space_id = ?", space.Id)
	if err != nil {
		tx.Rollback()
		return err
	}

	var doUpdate = len(existingSpaces) > 0

	// Update the owner of alt names
	if doUpdate && (len(updateFields) == 0 || util.InArray(updateFields, "UserId")) {
		_, err = tx.Exec("UPDATE spaces SET user_id=? WHERE parent_space_id = ?", space.UserId, space.Id)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// If saving the name then check if the name has changed, and if it has is it still unique
	if len(updateFields) == 0 || util.InArray(updateFields, "Name") {
		if !doUpdate || existingSpaces[0].Name != space.Name {
			// Check if the space name already in use by another space
			var exists bool
			err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM spaces WHERE user_id=? AND name=?)", space.UserId, space.Name).Scan(&exists)
			if err != nil {
				tx.Rollback()
				return err
			}

			if exists {
				tx.Rollback()
				return fmt.Errorf("space name already used")
			}
		}
	}

	if doUpdate {
		// Update the update time
		if len(updateFields) > 0 && !util.InArray(updateFields, "UpdatedAt") {
			updateFields = append(updateFields, "UpdatedAt")
		}

		err = db.update("spaces", space, updateFields)
		if err != nil {
			tx.Rollback()
			return err
		}
	} else {
		err = db.create("spaces", space)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	// If updating all or explicitly updating alt names
	if len(updateFields) == 0 || util.InArray(updateFields, "AltNames") {

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
		now := time.Now().UTC()
		for _, name := range space.AltNames {
			found := false
			for _, altName := range altNames {
				if altName == name {
					found = true

					// Update the location
					_, err = tx.Exec("UPDATE spaces SET location=?,updated_at=? WHERE parent_space_id = ? AND name = ?", now, space.Location, space.Id, name)
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

				_, err = tx.Exec("INSERT INTO spaces (space_id, parent_space_id, user_id, name, location, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)", altId, space.Id, space.UserId, name, space.Location, now, now)
				if err != nil {
					tx.Rollback()
					return err
				}
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

func (db *MySQLDriver) GetSpace(id string) (*model.Space, error) {
	var spaces []model.Space
	err := db.read("spaces", &spaces, nil, "space_id = ? AND parent_space_id = ''", id)
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

	return &spaces[0], nil
}

func (db *MySQLDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
	var spaces []*model.Space
	err := db.read("spaces", &spaces, nil, "(user_id = ? || shared_with_user_id = ?) AND parent_space_id = '' ORDER BY name ASC", userId, userId)
	if err != nil {
		return nil, err
	}

	return spaces, nil
}

func (db *MySQLDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
	var spaces []model.Space
	err := db.read("spaces", &spaces, nil, "(user_id = ? || shared_with_user_id = ?) AND name = ? ORDER BY shared_with_user_id ASC LIMIT 1", userId, userId, spaceName)
	if err != nil {
		return nil, err
	}

	if len(spaces) == 0 {
		return nil, fmt.Errorf("space not found")
	}

	// If has a parent ID then load the parent space
	if spaces[0].ParentSpaceId != "" {
		return db.GetSpace(spaces[0].ParentSpaceId)
	}

	return &spaces[0], nil
}

func (db *MySQLDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
	var spaces []*model.Space
	err := db.read("spaces", &spaces, nil, "template_id = ? AND parent_space_id = '' ORDER BY name ASC", templateId)
	if err != nil {
		return nil, err
	}

	return spaces, nil
}

func (db *MySQLDriver) GetSpaces() ([]*model.Space, error) {
	var spaces []*model.Space
	err := db.read("spaces", &spaces, nil, "parent_space_id = '' ORDER BY name ASC")
	if err != nil {
		return nil, err
	}

	return spaces, nil
}
