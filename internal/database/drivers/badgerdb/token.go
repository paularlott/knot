package driver_badgerdb

import (
	"encoding/json"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveToken(token *model.Token) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(token)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Tokens:%s", token.Id)), data).WithTTL(model.MaxTokenAge)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		e = badger.NewEntry([]byte(fmt.Sprintf("TokensByUserId:%s:%s", token.UserId, token.Id)), []byte(token.Id)).WithTTL(model.MaxTokenAge)
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteToken(token *model.Token) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Tokens:%s", token.Id)))
		if err != nil {
			return err
		}

		err = txn.Delete([]byte(fmt.Sprintf("TokensByUserId:%s:%s", token.UserId, token.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetToken(id string) (*model.Token, error) {
	var token = &model.Token{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Tokens:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, token)
		})
	})

	if err != nil {
		return nil, err
	}

	return token, err
}

func (db *BadgerDbDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
	var tokens []*model.Token

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(fmt.Sprintf("TokensByUserId:%s:", userId))
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			var tokenId string
			err := item.Value(func(val []byte) error {
				tokenId = string(val)
				return nil
			})
			if err != nil {
				return err
			}

			token, err := db.GetToken(tokenId)
			if err != nil {
				return err
			}

			tokens = append(tokens, token)
		}

		return nil
	})

	return tokens, err
}

func (db *BadgerDbDriver) GetTokens() ([]*model.Token, error) {
	var tokens []*model.Token

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Tokens:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var token = &model.Token{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, token)
			})
			if err != nil {
				return err
			}

			tokens = append(tokens, token)
		}

		return nil
	})

	return tokens, err
}
