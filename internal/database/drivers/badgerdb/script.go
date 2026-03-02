package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveScript(script *model.Script, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		existingScript, _ := db.GetScript(script.Id)

		if existingScript != nil {
			// If name or user_id changed, delete old index
			if (existingScript.Name != script.Name || existingScript.UserId != script.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("ScriptsByName:%s:%s", existingScript.UserId, existingScript.Name)))
				if err != nil {
					return err
				}
			}

			if len(updateFields) > 0 {
				util.CopyFields(script, existingScript, updateFields)
				script = existingScript
			}
		}

		data, err := json.Marshal(script)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Scripts:%s", script.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		// Create composite index with user_id:name
		e = badger.NewEntry([]byte(fmt.Sprintf("ScriptsByName:%s:%s", script.UserId, script.Name)), []byte(script.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteScript(script *model.Script) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Scripts:%s", script.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("ScriptsByName:%s:%s", script.UserId, script.Name)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetScript(id string) (*model.Script, error) {
	var script = &model.Script{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Scripts:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, script)
		})
	})

	if err != nil {
		return nil, err
	}

	return script, err
}

func (db *BadgerDbDriver) GetScripts() ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Scripts:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var script = &model.Script{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, script)
			})
			if err != nil {
				return err
			}

			scripts = append(scripts, script)
		}

		return nil
	})

	sort.Slice(scripts, func(i, j int) bool {
		return scripts[i].Name < scripts[j].Name
	})

	return scripts, err
}

func (db *BadgerDbDriver) GetScriptByName(name string) (*model.Script, error) {
	scripts, err := db.GetScriptsByName(name)
	if err != nil {
		return nil, err
	}
	return scripts[0], nil
}

// GetScriptsByName returns all scripts matching a name (for zone-specific overrides)
func (db *BadgerDbDriver) GetScriptsByName(name string) ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Scripts:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var script = &model.Script{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, script)
			})
			if err != nil {
				return err
			}

			// Filter by name and global (empty user_id)
			if script.Name == name && script.UserId == "" {
				scripts = append(scripts, script)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	// Sort by zone specificity (more zones = higher priority)
	sort.Slice(scripts, func(i, j int) bool {
		zonesI := len(scripts[i].Zones)
		zonesJ := len(scripts[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return scripts[i].CreatedAt.Before(scripts[j].CreatedAt)
	})

	return scripts, nil
}

// GetScriptByNameAndUser gets a script by name for a specific user
// If userId is empty, it searches for global scripts
// This supports the user script override functionality
func (db *BadgerDbDriver) GetScriptByNameAndUser(name string, userId string) (*model.Script, error) {
	scripts, err := db.GetScriptsByNameAndUser(name, userId)
	if err != nil {
		return nil, err
	}
	return scripts[0], nil
}

// GetScriptsByNameAndUser returns all scripts matching a name and user_id (for zone-specific overrides)
func (db *BadgerDbDriver) GetScriptsByNameAndUser(name string, userId string) ([]*model.Script, error) {
	var scripts []*model.Script

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Scripts:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var script = &model.Script{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, script)
			})
			if err != nil {
				return err
			}

			// Filter by name and user_id
			if script.Name == name && script.UserId == userId {
				scripts = append(scripts, script)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(scripts) == 0 {
		return nil, fmt.Errorf("script not found")
	}

	// Sort by zone specificity (more zones = higher priority)
	sort.Slice(scripts, func(i, j int) bool {
		zonesI := len(scripts[i].Zones)
		zonesJ := len(scripts[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return scripts[i].CreatedAt.Before(scripts[j].CreatedAt)
	})

	return scripts, nil
}
