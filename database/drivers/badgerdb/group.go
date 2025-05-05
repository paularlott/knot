package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveGroup(group *model.Group) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(group)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Groups:%s", group.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteGroup(group *model.Group) error {

	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Groups:%s", group.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetGroup(id string) (*model.Group, error) {
	var group = &model.Group{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Groups:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, group)
		})
	})

	if err != nil {
		return nil, err
	}

	return group, err
}

func (db *BadgerDbDriver) GetGroups() ([]*model.Group, error) {
	var groups []*model.Group

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Groups:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var group = &model.Group{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, group)
			})
			if err != nil {
				return err
			}

			groups = append(groups, group)
		}

		return nil
	})

	// Sort the groups by name
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	return groups, err
}
