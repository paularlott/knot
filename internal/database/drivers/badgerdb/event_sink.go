package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveEventSink(sink *model.EventSink, updateFields []string) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		existing, _ := db.GetEventSink(sink.Id)

		if existing != nil {
			if len(updateFields) > 0 {
				util.CopyFields(sink, existing, updateFields)
				sink = existing
			}
		}

		data, err := json.Marshal(sink)
		if err != nil {
			return err
		}

		return txn.Set([]byte(fmt.Sprintf("EventSinks:%s", sink.Id)), data)
	})
}

func (db *BadgerDbDriver) DeleteEventSink(sink *model.EventSink) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(fmt.Sprintf("EventSinks:%s", sink.Id)))
	})
}

func (db *BadgerDbDriver) GetEventSink(id string) (*model.EventSink, error) {
	sink := &model.EventSink{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("EventSinks:%s", id)))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, sink)
		})
	})

	if err != nil {
		return nil, err
	}

	return sink, nil
}

func (db *BadgerDbDriver) GetEventSinks() ([]*model.EventSink, error) {
	var sinks []*model.EventSink

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("EventSinks:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			sink := &model.EventSink{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, sink)
			})
			if err != nil {
				return err
			}

			sinks = append(sinks, sink)
		}

		return nil
	})

	sort.Slice(sinks, func(i, j int) bool {
		return sinks[i].Name < sinks[j].Name
	})

	return sinks, err
}
