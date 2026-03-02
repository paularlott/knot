package driver_badgerdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/internal/database/model"

	badger "github.com/dgraph-io/badger/v4"
)

func (db *BadgerDbDriver) SaveResponse(response *model.Response) error {
	err := db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(response)
		if err != nil {
			return err
		}

		e := badger.NewEntry([]byte(fmt.Sprintf("Responses:%s", response.Id)), data)
		// Set TTL if expires_at is set
		if response.ExpiresAt != nil {
			ttl := time.Until(*response.ExpiresAt)
			if ttl > 0 {
				e = e.WithTTL(ttl)
			}
		}
		if err = txn.SetEntry(e); err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) DeleteResponse(response *model.Response) error {

	err := db.connection.Update(func(txn *badger.Txn) error {
		err := txn.Delete([]byte(fmt.Sprintf("Responses:%s", response.Id)))
		if err != nil {
			return err
		}

		return nil
	})

	return err
}

func (db *BadgerDbDriver) GetResponse(id string) (*model.Response, error) {
	var response = &model.Response{}

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(fmt.Sprintf("Responses:%s", id)))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, response)
		})
	})

	if err != nil {
		return nil, err
	}

	// Check if expired
	if response.ExpiresAt != nil && time.Now().UTC().After(*response.ExpiresAt) {
		return nil, badger.ErrKeyNotFound
	}

	return response, err
}

func (db *BadgerDbDriver) GetResponses() ([]*model.Response, error) {
	var responses []*model.Response

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("Responses:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var response = &model.Response{}

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, response)
			})
			if err != nil {
				return err
			}

			// Skip expired responses
			if response.ExpiresAt != nil && time.Now().UTC().After(*response.ExpiresAt) {
				continue
			}

			responses = append(responses, response)
		}

		return nil
	})

	// Sort by created_at (newest first)
	sort.Slice(responses, func(i, j int) bool {
		return responses[i].CreatedAt.After(responses[j].CreatedAt)
	})

	return responses, err
}

func (db *BadgerDbDriver) GetResponsesByUser(userId string) ([]*model.Response, error) {
	responses, err := db.GetResponses()
	if err != nil {
		return nil, err
	}

	// Filter by user_id
	var filtered []*model.Response
	for _, response := range responses {
		if response.UserId == userId {
			filtered = append(filtered, response)
		}
	}

	return filtered, nil
}

func (db *BadgerDbDriver) GetResponsesByStatus(status model.ResponseStatus) ([]*model.Response, error) {
	responses, err := db.GetResponses()
	if err != nil {
		return nil, err
	}

	// Filter by status
	var filtered []*model.Response
	for _, response := range responses {
		if response.Status == status {
			filtered = append(filtered, response)
		}
	}

	// For pending/in_progress, sort by created_at (oldest first for processing)
	if status == model.StatusPending || status == model.StatusInProgress {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].CreatedAt.Before(filtered[j].CreatedAt)
		})
	}

	return filtered, nil
}
