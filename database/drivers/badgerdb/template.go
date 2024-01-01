package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveTemplate(template *model.Template) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    // Load the existing space
    existingSpace, _ := db.GetTemplate(template.Id)
    if existingSpace == nil {
      template.CreatedAt = time.Now().UTC()
    }

    template.UpdatedUserId = template.CreatedUserId
    template.UpdatedAt = time.Now().UTC()
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

func (db *BadgerDbDriver) GetTemplateOptionList() (map[string]string, error) {
  templates, err := db.GetTemplates()
  if err != nil {
    return nil, err
  }

  // Build the option list, id => name
  var optionList = make(map[string]string)
  optionList[""] = "None (Manual Deploy)"
  for _, template := range templates {
    optionList[template.Id] = template.Name
  }

  return optionList, nil
}
