package driver_badgerdb

import (
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) GetCfgValue(name string) (*model.CfgValue, error) {
	var v = &model.CfgValue{
		Name:  name,
		Value: "",
	}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Configs:%s", name)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			v.Value = string(val)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return v, nil
}

func (db *BadgerDbDriver) SaveCfgValue(cfgValue *model.CfgValue) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry([]byte(fmt.Sprintf("Configs:%s", cfgValue.Name)), []byte(cfgValue.Value))
		if err := txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetCfgValues() ([]*model.CfgValue, error) {
	var cfgValues []*model.CfgValue

	err := db.connection.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte("Configs:")
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			name := string(item.Key()[len("Configs:"):])
			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			cfgValues = append(cfgValues, &model.CfgValue{
				Name:  name,
				Value: string(value),
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return cfgValues, nil
}
