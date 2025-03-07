package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveVolume(volume *model.Volume) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		volume.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(volume)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Volumes:%s", volume.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteVolume(volume *model.Volume) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Volumes:%s", volume.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetVolume(id string) (*model.Volume, error) {
	var volume = &model.Volume{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Volumes:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, volume)
		})
	})

	if err != nil {
		return nil, err
	}

	return volume, err
}

func (db *BadgerDbDriver) GetVolumes() ([]*model.Volume, error) {
	var volumes []*model.Volume

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Volumes:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var volume = &model.Volume{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, volume)
			})
			if err != nil {
				return err
			}

			volumes = append(volumes, volume)
		}

		return nil
	})

	// Sort the volumes by name
	sort.Slice(volumes, func(i, j int) bool {
		return volumes[i].Name < volumes[j].Name
	})

	return volumes, err
}
