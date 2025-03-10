package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveUser(user *model.User, updateFields []string) error {
	var err error

	err = db.connection.Update(func(txn *badger.Txn) error {
		var newUser bool = true

		// Load the existing user
		existingUser, _ := db.GetUser(user.Id)
		if existingUser != nil {
			newUser = false

			// Don't allow username to be changed
			user.Username = existingUser.Username
		} else {
			user.CreatedAt = time.Now().UTC()
		}

		// If email address changed, check if the new email address is unique
		if newUser || user.Email != existingUser.Email {
			exists, err := db.keyExists(fmt.Sprintf("UsersByEmail:%s", user.Email))
			if err != nil {
				return err
			} else if exists {
				return fmt.Errorf("duplicate email address")
			}

			if !newUser {
				// Delete the old email address
				err = txn.Delete([]byte(fmt.Sprintf("UsersByEmail:%s", existingUser.Email)))
				if err != nil {
					return err
				}
			}
		}

		// Check if the new username is unique
		if newUser {
			exists, err := db.keyExists(fmt.Sprintf("UsersByUsername:%s", strings.ToLower(user.Username)))
			if err != nil {
				return err
			} else if exists {
				return fmt.Errorf("duplicate username")
			}
		}

		// Apply changes from new to existing existing if doing partial update
		if existingUser != nil && len(updateFields) > 0 {
			util.CopyFields(user, existingUser, updateFields)
			user = existingUser
		}

		user.UpdatedAt = time.Now().UTC()
		data, err := json.Marshal(user)
		if err != nil {
			return err
		}

		// Save the new user
		err = txn.Set([]byte(fmt.Sprintf("Users:%s", user.Id)), data)
		if err != nil {
			return err
		}

		err = txn.Set([]byte(fmt.Sprintf("UsersByEmail:%s", user.Email)), []byte(user.Id))
		if err != nil {
			return err
		}

		err = txn.Set([]byte(fmt.Sprintf("UsersByUsername:%s", strings.ToLower(user.Username))), []byte(user.Id))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteUser(user *model.User) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Users:%s", user.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("UsersByEmail:%s", user.Email)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("UsersByUsername:%s", strings.ToLower(user.Username))))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetUser(id string) (*model.User, error) {
	var user = &model.User{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Users:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, user)
		})
	})

	if err != nil {
		return nil, err
	}

	return user, err
}

func (db *BadgerDbDriver) GetUserByEmail(email string) (*model.User, error) {
	var user *model.User = nil

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("UsersByEmail:%s", email)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			var err error
			user, err = db.GetUser(string(val))
			return err
		})
	})

	return user, err
}

func (db *BadgerDbDriver) GetUserByUsername(name string) (*model.User, error) {
	var user *model.User = nil

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("UsersByUsername:%s", name)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			var err error
			user, err = db.GetUser(string(val))
			return err
		})
	})

	return user, err
}

func (db *BadgerDbDriver) GetUsers() ([]*model.User, error) {
	var users []*model.User

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Users:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var user = &model.User{}
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, user)
			})
			if err != nil {
				return err
			}

			users = append(users, user)
		}

		return nil
	})

	// Sort the users by username
	sort.Slice(users, func(i, j int) bool {
		return users[i].Username < users[j].Username
	})

	return users, err
}

func (db *BadgerDbDriver) HasUsers() (bool, error) {
	var count int = 0

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Users:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var user model.User
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &user)
			})
			if err != nil {
				return err
			}

			if user.Active {
				count++
			}
		}

		return nil
	})

	return count > 0, err
}
