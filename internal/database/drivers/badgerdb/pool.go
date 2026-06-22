package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SavePoolDefinition(pool *model.PoolDefinition, updateFields []string) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		existingPool, _ := db.GetPoolDefinition(pool.Id)
		if existingPool != nil {
			if (existingPool.Name != pool.Name || existingPool.CreatedUserId != pool.CreatedUserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "CreatedUserId")) {
				if err := txn.Delete([]byte(fmt.Sprintf("PoolsByName:%s:%s", existingPool.CreatedUserId, existingPool.Name))); err != nil {
					return err
				}
			}
			if len(updateFields) > 0 {
				util.CopyFields(pool, existingPool, updateFields)
				pool = existingPool
			}
		}

		data, err := json.Marshal(pool)
		if err != nil {
			return err
		}
		if err = txn.SetEntry(badger.NewEntry([]byte(fmt.Sprintf("Pools:%s", pool.Id)), data)); err != nil {
			return err
		}
		return txn.SetEntry(badger.NewEntry([]byte(fmt.Sprintf("PoolsByName:%s:%s", pool.CreatedUserId, pool.Name)), []byte(pool.Id)))
	})
}

func (db *BadgerDbDriver) DeletePoolDefinition(pool *model.PoolDefinition) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		if err := txn.Delete([]byte(fmt.Sprintf("Pools:%s", pool.Id))); err != nil {
			return err
		}
		return txn.Delete([]byte(fmt.Sprintf("PoolsByName:%s:%s", pool.CreatedUserId, pool.Name)))
	})
}

func (db *BadgerDbDriver) GetPoolDefinition(id string) (*model.PoolDefinition, error) {
	pool := &model.PoolDefinition{}
	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Pools:%s", id)))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, pool)
		})
	})
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (db *BadgerDbDriver) GetPoolDefinitionByName(userId, name string) (*model.PoolDefinition, error) {
	var id string
	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("PoolsByName:%s:%s", userId, name)))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			id = string(val)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	pool, err := db.GetPoolDefinition(id)
	if err != nil || pool.IsDeleted {
		return nil, fmt.Errorf("pool not found")
	}
	return pool, nil
}

func (db *BadgerDbDriver) GetPoolDefinitions() ([]*model.PoolDefinition, error) {
	var pools []*model.PoolDefinition
	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte("Pools:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			pool := &model.PoolDefinition{}
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, pool)
			}); err != nil {
				return err
			}
			pools = append(pools, pool)
		}
		return nil
	})
	sort.Slice(pools, func(i, j int) bool { return pools[i].Name < pools[j].Name })
	return pools, err
}
