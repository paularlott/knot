package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveSpace(space *model.Space) error {
  err := db.connection.Update(func(txn *badger.Txn) error {
    space.UpdatedAt = time.Now().UTC()
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

    e = badger.NewEntry([]byte(fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id)), []byte(space.Id))
    if err = txn.SetEntry(e); err != nil {
      return err
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

    err = txn.Delete([]byte(fmt.Sprintf("SpacesByTemplateId:%s:%s", space.TemplateId, space.Id)))
    if err != nil {
      return err
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
