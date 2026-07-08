package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"
)

// Conversations are keyed by Conversations:<userId>:<id> so that listing a
// single user's history is a cheap prefix scan rather than a scan-and-filter
// of every user's conversations.

func conversationKey(userId, id string) string {
	return fmt.Sprintf("Conversations:%s:%s", userId, id)
}

func (db *BadgerDbDriver) SaveConversation(conv *model.Conversation) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(conv)
		if err != nil {
			return err
		}
		return txn.Set([]byte(conversationKey(conv.UserId, conv.Id)), data)
	})
}

func (db *BadgerDbDriver) DeleteConversation(conv *model.Conversation) error {
	return db.connection.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(conversationKey(conv.UserId, conv.Id)))
	})
}

func (db *BadgerDbDriver) GetConversation(userId string, id string) (*model.Conversation, error) {
	conv := &model.Conversation{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(conversationKey(userId, id)))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, conv)
		})
	})

	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (db *BadgerDbDriver) GetConversationsByUser(userId string) ([]*model.Conversation, error) {
	return db.scanConversations(fmt.Sprintf("Conversations:%s:", userId), false)
}

func (db *BadgerDbDriver) GetConversations() ([]*model.Conversation, error) {
	return db.scanConversations("Conversations:", true)
}

// scanConversations iterates a key prefix. When includeDeleted is false,
// soft-deleted conversations are skipped (the per-user list view).
// DeleteTombstonedConversationsBefore is the reaper: it hard-deletes only
// tombstoned conversations whose UpdatedAt predates the cutoff. (BadgerDB
// also has an hourly soft-delete reap in connect.go; this is the cross-driver
// backstop used by the daily retention sweep.)
func (db *BadgerDbDriver) DeleteTombstonedConversationsBefore(before time.Time) error {
	pfx := []byte("Conversations:")
	return db.connection.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		var keysToDelete [][]byte
		for it.Seek(pfx); it.ValidForPrefix(pfx); it.Next() {
			item := it.Item()
			conv := &model.Conversation{}
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, conv) }); err != nil {
				continue
			}
			if conv.IsDeleted && conv.UpdatedAt.Time().Before(before) {
				key := item.Key()
				keyCopy := make([]byte, len(key))
				copy(keyCopy, key)
				keysToDelete = append(keysToDelete, keyCopy)
			}
		}

		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetStaleConversations returns live (non-deleted) conversations whose
// UpdatedAt predates the cutoff — candidates for retention tombstoning.
func (db *BadgerDbDriver) GetStaleConversations(before time.Time) ([]*model.Conversation, error) {
	var out []*model.Conversation
	pfx := []byte("Conversations:")

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(pfx); it.ValidForPrefix(pfx); it.Next() {
			item := it.Item()
			conv := &model.Conversation{}
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, conv) }); err != nil {
				continue
			}
			if !conv.IsDeleted && conv.UpdatedAt.Time().Before(before) {
				out = append(out, conv)
			}
		}
		return nil
	})

	return out, err
}

func (db *BadgerDbDriver) scanConversations(prefix string, includeDeleted bool) ([]*model.Conversation, error) {
	var conversations []*model.Conversation

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		pfx := []byte(prefix)
		for it.Seek(pfx); it.ValidForPrefix(pfx); it.Next() {
			item := it.Item()
			conv := &model.Conversation{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, conv)
			})
			if err != nil {
				return err
			}

			if !includeDeleted && conv.IsDeleted {
				continue
			}
			conversations = append(conversations, conv)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].UpdatedAt.After(conversations[j].UpdatedAt)
	})

	return conversations, nil
}
