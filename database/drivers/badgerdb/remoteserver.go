package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/database/model"
)

func (db *BadgerDbDriver) SaveRemoteServer(server *model.RemoteServer) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		// Calculate the expiration time
		server.ExpiresAfter = time.Now().UTC().Add(model.REMOTE_SERVER_TIMEOUT)

		data, err := json.Marshal(server)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("RemoteServer:%s", server.Id)), data).WithTTL(model.REMOTE_SERVER_TIMEOUT)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteRemoteServer(server *model.RemoteServer) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("RemoteServer:%s", server.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetRemoteServer(id string) (*model.RemoteServer, error) {
	var server = &model.RemoteServer{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("RemoteServer:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, server)
		})
	})

	if err != nil {
		return nil, err
	}

	return server, err
}

func (db *BadgerDbDriver) GetRemoteServers() ([]*model.RemoteServer, error) {
	var servers []*model.RemoteServer

	err := db.connection.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10

		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("RemoteServer:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var server = &model.RemoteServer{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, server)
			})
			if err != nil {
				return err
			}

			servers = append(servers, server)
		}

		return nil
	})

	return servers, err
}
