package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveMCPServer(server *model.MCPServer, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		existing, _ := db.GetMCPServer(server.Id)

		if existing != nil {
			if (existing.Namespace != server.Namespace || existing.UserId != server.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Namespace") || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("MCPServersByUser:%s:%s", existing.UserId, existing.Namespace)))
				if err != nil {
					return err
				}
			}

			if len(updateFields) > 0 {
				util.CopyFields(server, existing, updateFields)
				server = existing
			}
		}

		data, err := json.Marshal(server)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("MCPServers:%s", server.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("MCPServersByUser:%s:%s", server.UserId, server.Namespace)), []byte(server.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteMCPServer(server *model.MCPServer) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("MCPServers:%s", server.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("MCPServersByUser:%s:%s", server.UserId, server.Namespace)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetMCPServer(id string) (*model.MCPServer, error) {
	var server = &model.MCPServer{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("MCPServers:%s", id)))
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

func (db *BadgerDbDriver) GetMCPServers() ([]*model.MCPServer, error) {
	var servers []*model.MCPServer

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("MCPServers:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var server = &model.MCPServer{}

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

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Namespace < servers[j].Namespace
	})

	return servers, err
}

func (db *BadgerDbDriver) GetMCPServersByUser(userId string) ([]*model.MCPServer, error) {
	all, err := db.GetMCPServers()
	if err != nil {
		return nil, err
	}

	var result []*model.MCPServer
	for _, s := range all {
		if s.UserId == userId {
			result = append(result, s)
		}
	}

	return result, nil
}
