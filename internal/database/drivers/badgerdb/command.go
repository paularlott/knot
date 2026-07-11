package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *BadgerDbDriver) SaveCommand(command *model.Command, updateFields []string) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		existingCommand, _ := db.GetCommand(command.Id)

		if existingCommand != nil {
			if (existingCommand.Name != command.Name || existingCommand.UserId != command.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
				err := txn.Delete([]byte(fmt.Sprintf("CommandsByName:%s:%s", existingCommand.UserId, existingCommand.Name)))
				if err != nil {
					return err
				}
			}

			if len(updateFields) > 0 {
				util.CopyFields(command, existingCommand, updateFields)
				command = existingCommand
			}
		}

		data, err := json.Marshal(command)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Commands:%s", command.Id)), data)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("CommandsByName:%s:%s", command.UserId, command.Name)), []byte(command.Id))
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteCommand(command *model.Command) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Commands:%s", command.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("CommandsByName:%s:%s", command.UserId, command.Name)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetCommand(id string) (*model.Command, error) {
	var command = &model.Command{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Commands:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, command)
		})
	})

	if err != nil {
		return nil, err
	}

	return command, err
}

func (db *BadgerDbDriver) GetCommands() ([]*model.Command, error) {
	var commands []*model.Command

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Commands:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var command = &model.Command{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, command)
			})
			if err != nil {
				return err
			}

			commands = append(commands, command)
		}

		return nil
	})

	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})

	return commands, err
}

func (db *BadgerDbDriver) GetCommandsByName(name string) ([]*model.Command, error) {
	var commands []*model.Command

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Commands:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var command = &model.Command{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, command)
			})
			if err != nil {
				return err
			}

			if command.Name == name && command.UserId == "" {
				commands = append(commands, command)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	sort.Slice(commands, func(i, j int) bool {
		zonesI := len(commands[i].Zones)
		zonesJ := len(commands[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return commands[i].CreatedAt.Before(commands[j].CreatedAt)
	})

	return commands, nil
}

func (db *BadgerDbDriver) GetCommandsByNameAndUser(name string, userId string) ([]*model.Command, error) {
	var commands []*model.Command

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Commands:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var command = &model.Command{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, command)
			})
			if err != nil {
				return err
			}

			if command.Name == name && command.UserId == userId {
				commands = append(commands, command)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if len(commands) == 0 {
		return nil, fmt.Errorf("command not found")
	}

	sort.Slice(commands, func(i, j int) bool {
		zonesI := len(commands[i].Zones)
		zonesJ := len(commands[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return commands[i].CreatedAt.Before(commands[j].CreatedAt)
	})

	return commands, nil
}
