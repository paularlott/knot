package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveSpace(space *model.Space, updateFields []string) error {
	space.NormalizeShares()

	err := db.connection.Update(func(txn *badger.Txn) error {

		// Load the existing space
		existingSpace, _ := db.GetSpace(space.Id)
		if existingSpace != nil {
			existingSpace.NormalizeShares()

			// If user changed then delete the space and add back in with new user
			if existingSpace.UserId != space.UserId {
				db.DeleteSpace(existingSpace)
				existingSpace = nil
			}
		}

		// If new space or name changed check if the new name is unique
		if existingSpace == nil || (space.Name != existingSpace.Name && (len(updateFields) == 0 || util.InArray(updateFields, "Name"))) {
			exists, err := db.keyExists(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(space.Name)))
			if err != nil {
				return err
			} else if exists {
				return fmt.Errorf("space name already used")
			}
		}

		if existingSpace != nil {
			if existingSpace.UserId != space.UserId && (len(updateFields) == 0 || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", existingSpace.UserId, space.Id)))
				if err != nil {
					return err
				}

				err = txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", existingSpace.UserId, strings.ToLower(existingSpace.Name))))
				if err != nil {
					return err
				}

				// Delete alternate names
				for _, altName := range existingSpace.AltNames {
					err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", existingSpace.UserId, strings.ToLower(altName))))
					if err != nil {
						return err
					}

					err = txn.SetEntry(badger.NewEntry([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(altName))), []byte(space.Id)))
					if err != nil {
						return err
					}
				}
			}

			// If existing and shared but changed then delete the old shared information
			for _, sharedUserId := range existingSpace.SharedUserIds() {
				if !space.IsSharedWith(sharedUserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Shares") || util.InArray(updateFields, "SharedWithUserId")) {
					err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", sharedUserId, space.Id)))
					if err != nil {
						return err
					}
				}
			}

			if existingSpace != nil && existingSpace.Name != space.Name && (len(updateFields) == 0 || util.InArray(updateFields, "Name")) {
				err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", existingSpace.UserId, strings.ToLower(existingSpace.Name))))
				if err != nil {
					return err
				}
			}
		}

		// Apply changes from space to existing space if doing partial update
		if existingSpace != nil && len(updateFields) > 0 {
			util.CopyFields(space, existingSpace, updateFields)
			space = existingSpace
			space.NormalizeShares()
		}

		data, err := json.Marshal(space)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Spaces:%s", space.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", space.UserId, space.Id)), []byte(space.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		// If shared with then add space under shared user
		for _, sharedUserId := range space.SharedUserIds() {
			e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", sharedUserId, space.Id)), []byte(space.Id))
			if err = txn.SetEntry(e); err != nil {
				return err
			}
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(space.Name))), []byte(space.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id)), []byte(space.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		// If existing space
		if existingSpace != nil {

			// Delete alternate names that are no longer in the list
			for _, altName := range existingSpace.AltNames {
				found := false
				for _, name := range space.AltNames {
					if altName == name {
						found = true
						break
					}
				}
				if !found {
					err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(altName))))
					if err != nil {
						return err
					}
				}
			}
		}

		// Add alt names
		for _, name := range space.AltNames {
			found := false
			if existingSpace != nil {
				for _, altName := range existingSpace.AltNames {
					if altName == name {
						found = true
						break
					}
				}
			}

			if !found {
				// Check if the name is unique
				exists, err := db.keyExists(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(name)))
				if err != nil {
					return err
				} else if exists {
					return fmt.Errorf("space name already used")
				}

				e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(name))), []byte(space.Id))
				if err = txn.SetEntry(e); err != nil {
					return err
				}
			}
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteSpace(space *model.Space) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Spaces:%s", space.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", space.UserId, space.Id)))
		if err != nil {
			return err
		}

		// If shared with a user then delete
		for _, sharedUserId := range space.SharedUserIds() {
			err := txn.Delete([]byte(fmt.Sprintf("SpacesByUserId:%s:%s", sharedUserId, space.Id)))
			if err != nil {
				return err
			}
		}

		err = txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(space.Name))))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id)))
		if err != nil {
			return err
		}

		// Delete alternate names
		for _, name := range space.AltNames {
			err = txn.Delete([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", space.UserId, strings.ToLower(name))))
			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetSpace(id string) (*model.Space, error) {
	var space = &model.Space{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Spaces:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, space)
		})
	})

	if err != nil {
		return nil, err
	}
	space.NormalizeShares()

	return space, err
}

func (db *BadgerDbDriver) GetSpacesForUser(userId string) ([]*model.Space, error) {
	var spaces []*model.Space

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(fmt.Sprintf("SpacesByUserId:%s:", userId))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var spaceId string
			err := item.Value(func(val []byte) error {
				spaceId = string(val)
				return nil
			})
			if err != nil {
				return err
			}

			space, err := db.GetSpace(spaceId)
			if err != nil {
				return err
			}

			spaces = append(spaces, space)
		}

		return nil
	})

	// Sort the agents by name
	sort.Slice(spaces, func(i, j int) bool {
		return spaces[i].Name < spaces[j].Name
	})

	return spaces, err
}

func (db *BadgerDbDriver) GetSpaceByName(userId string, spaceName string) (*model.Space, error) {
	var space = &model.Space{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("SpacesByUserIdByName:%s:%s", userId, strings.ToLower(spaceName))))
		if err != nil {
			return err
		}

		var spaceId string
		err = item.Value(func(val []byte) error {
			spaceId = string(val)
			return nil
		})
		if err != nil {
			return err
		}

		space, err = db.GetSpace(spaceId)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		// Try getting all the spaces and see if it's a shared space
		spaces, err2 := db.GetSpacesForUser(userId)
		if err2 != nil {
			return nil, err2
		}

		for _, s := range spaces {
			if s.Name == spaceName && s.IsSharedWith(userId) {
				return s, nil
			}
		}

		return nil, err
	}

	return space, err
}

func (db *BadgerDbDriver) GetSpacesByTemplateId(templateId string) ([]*model.Space, error) {
	var spaces []*model.Space

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(fmt.Sprintf("SpacesByTemplateId:%s:", templateId))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var spaceId string
			err := item.Value(func(val []byte) error {
				spaceId = string(val)
				return nil
			})
			if err != nil {
				return err
			}

			space, err := db.GetSpace(spaceId)
			if err != nil {
				return err
			}

			spaces = append(spaces, space)
		}

		return nil
	})

	// Sort the agents by name
	sort.Slice(spaces, func(i, j int) bool {
		return spaces[i].Name < spaces[j].Name
	})

	return spaces, err
}

func (db *BadgerDbDriver) GetSpaces() ([]*model.Space, error) {
	var spaces []*model.Space

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Spaces:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var space = &model.Space{}
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, space)
			})
			if err != nil {
				return err
			}

			spaces = append(spaces, space)
		}

		return nil
	})

	// Sort the agents by name
	sort.Slice(spaces, func(i, j int) bool {
		return spaces[i].Name < spaces[j].Name
	})

	return spaces, err
}
