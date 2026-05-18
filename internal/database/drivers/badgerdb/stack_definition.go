package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveStackDefinition(def *model.StackDefinition, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		existingDef, _ := db.GetStackDefinition(def.Id)

		if existingDef != nil {
			if (existingDef.Name != def.Name || existingDef.UserId != def.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("StackDefsByName:%s:%s", existingDef.UserId, existingDef.Name)))
				if err != nil {
					return err
				}
			}

			if len(updateFields) > 0 {
				util.CopyFields(def, existingDef, updateFields)
				def = existingDef
			}
		}

		data, err := json.Marshal(def)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("StackDefs:%s", def.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("StackDefsByName:%s:%s", def.UserId, def.Name)), []byte(def.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteStackDefinition(def *model.StackDefinition) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("StackDefs:%s", def.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("StackDefsByName:%s:%s", def.UserId, def.Name)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetStackDefinition(id string) (*model.StackDefinition, error) {
	var def = &model.StackDefinition{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("StackDefs:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, def)
		})
	})

	if err != nil {
		return nil, err
	}

	return def, err
}

func (db *BadgerDbDriver) GetStackDefinitions() ([]*model.StackDefinition, error) {
	var defs []*model.StackDefinition

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("StackDefs:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var def = &model.StackDefinition{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, def)
			})
			if err != nil {
				return err
			}

			if !def.IsDeleted {
				defs = append(defs, def)
			}
		}

		return nil
	})

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Name < defs[j].Name
	})

	return defs, err
}

func (db *BadgerDbDriver) GetStackDefinitionsByUserId(userId string) ([]*model.StackDefinition, error) {
	defs, err := db.GetStackDefinitions()
	if err != nil {
		return nil, err
	}

	var result []*model.StackDefinition
	for _, def := range defs {
		if def.UserId == userId {
			result = append(result, def)
		}
	}

	return result, nil
}

func (db *BadgerDbDriver) GetStackDefinitionByName(name string, userId string) (*model.StackDefinition, error) {
	defs, err := db.GetStackDefinitions()
	if err != nil {
		return nil, err
	}

	for _, def := range defs {
		if def.Name == name && def.UserId == userId {
			return def, nil
		}
	}

	return nil, fmt.Errorf("stack definition not found")
}
