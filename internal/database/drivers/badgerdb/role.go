package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveRole(role *model.Role) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(role)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Roles:%s", role.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteRole(role *model.Role) error {

	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Roles:%s", role.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetRole(id string) (*model.Role, error) {
	var role = &model.Role{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Roles:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, role)
		})
	})

	if err != nil {
		return nil, err
	}

	return role, err
}

func (db *BadgerDbDriver) GetRoles() ([]*model.Role, error) {
	var roles []*model.Role

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Roles:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var role = &model.Role{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, role)
			})
			if err != nil {
				return err
			}

			roles = append(roles, role)
		}

		return nil
	})

	// Sort the roles by name
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})

	return roles, err
}
