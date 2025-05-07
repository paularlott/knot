package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveTemplateVar(templateVar *model.TemplateVar) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		templateVar.Value = templateVar.GetValueEncrypted()
		data, err := json.Marshal(templateVar)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("TemplateVars:%s", templateVar.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteTemplateVar(templateVar *model.TemplateVar) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("TemplateVars:%s", templateVar.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
	var templateVar = &model.TemplateVar{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("TemplateVars:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, templateVar)
		})
	})

	if err != nil {
		return nil, err
	}

	templateVar.DecryptSetValue(templateVar.Value)
	return templateVar, err
}

func (db *BadgerDbDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
	var templateVars []*model.TemplateVar

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("TemplateVars:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var templateVar = &model.TemplateVar{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, templateVar)
			})
			if err != nil {
				return err
			}

			templateVar.DecryptSetValue(templateVar.Value)
			templateVars = append(templateVars, templateVar)
		}

		return nil
	})

	// Sort the template vars by name
	sort.Slice(templateVars, func(i, j int) bool {
		return templateVars[i].Name < templateVars[j].Name
	})

	return templateVars, err
}
