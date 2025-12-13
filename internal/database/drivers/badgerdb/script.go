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

		if existingScript != nil && len(updateFields) > 0 {
			util.CopyFields(script, existingScript, updateFields)
			script = existingScript
		}

		data, err := json.Marshal(script)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Scripts:%s", script.Id)), data)
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
	scripts, err := db.GetScripts()
	if err != nil {
		return nil, err
	}

	for _, script := range scripts {
		if script.Name == name {
			return script, nil
		}
	}

	return nil, fmt.Errorf("script not found")
}
