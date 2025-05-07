package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveTemplate(template *model.Template, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		// Load the existing template
		existingTemplate, _ := db.GetTemplate(template.Id)

		// Apply changes from new to existing if doing partial update
		if existingTemplate != nil && len(updateFields) > 0 {
			util.CopyFields(template, existingTemplate, updateFields)
			template = existingTemplate
		}

		data, err := json.Marshal(template)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Templates:%s", template.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteTemplate(template *model.Template) error {

	// Test if the space in in use
	spaces, err := db.GetSpacesByTemplateId(template.Id)
	if err != nil {
		return err
	}

	if len(spaces) > 0 {
		return fmt.Errorf("template in use")
	}

	err = db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Templates:%s", template.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetTemplate(id string) (*model.Template, error) {
	var template = &model.Template{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Templates:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, template)
		})
	})

	if err != nil {
		return nil, err
	}

	return template, err
}

func (db *BadgerDbDriver) GetTemplates() ([]*model.Template, error) {
	var templates []*model.Template

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Templates:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var template = &model.Template{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, template)
			})
			if err != nil {
				return err
			}

			templates = append(templates, template)
		}

		return nil
	})

	// Sort the templates by name
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})

	return templates, err
}
