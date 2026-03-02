package driver_badgerdb

import (
	"encoding/json"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"

	badger "github.com/dgraph-io/badger/v4"
)

const (
	gcInterval    = 1 * time.Hour
	garbageMaxAge = 3 * 24 * time.Hour
)

type BadgerDbDriver struct {
	connection *badger.DB
	logger     logger.Logger
}

func (db *BadgerDbDriver) keyExists(key string) (bool, error) {
	var exists = false

	err := db.connection.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}

		exists = item != nil
		return nil
	})

	return exists, err
}

func (db *BadgerDbDriver) Connect() error {
	db.logger = log.WithGroup("db")
	db.logger.Debug("connecting to BadgerDB")

	var err error
	cfg := config.GetServerConfig()
	options := badger.DefaultOptions(cfg.BadgerDB.Path)
	options.Logger = badgerdbLogger()
	options.IndexCacheSize = 100 << 20 // 100MB

	db.connection, err = badger.Open(options)
	if err == nil {

		// Start the garbage collector
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
			again:
				db.logger.Debug("running GC")
				err := db.connection.RunValueLogGC(0.5)
				if err == nil {
					goto again
				}
			}
		}()
	}

	// Start a go routine to clear deleted items from the database
	go func() {
		intervalTimer := time.NewTicker(gcInterval)
		defer intervalTimer.Stop()

		for range intervalTimer.C {
			db.logger.Debug("running garbage collector")

			// Clear old audit logs
			db.deleteAuditLogs()

			before := time.Now().UTC()
			before = before.Add(-garbageMaxAge)

			// Remove old groups
			db.cleanupObjectType("Groups", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Group
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old roles
			db.cleanupObjectType("Roles", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Role
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old spaces
			db.cleanupObjectType("Spaces", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Space
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old templates
			db.cleanupObjectType("Templates", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Template
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old template vars
			db.cleanupObjectType("TemplateVars", before, func(data []byte) (bool, time.Time, error) {
				var obj model.TemplateVar
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old users
			db.cleanupObjectType("Users", before, func(data []byte) (bool, time.Time, error) {
				var obj model.User
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old tokens
			db.cleanupObjectType("Tokens", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Token
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old volumes
			db.cleanupObjectType("Volumes", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Volume
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old scripts
			db.cleanupObjectType("Scripts", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Script
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})

			// Remove old responses
			db.cleanupObjectType("Responses", before, func(data []byte) (bool, time.Time, error) {
				var obj model.Response
				if err := json.Unmarshal(data, &obj); err != nil {
					return false, time.Time{}, err
				}
				return obj.IsDeleted, obj.UpdatedAt.Time(), nil
			})
		}
	}()

	return err
}

// cleanupObjectType iterates through keys of a given object type and deletes old soft-deleted entries
func (db *BadgerDbDriver) cleanupObjectType(objectType string, before time.Time, checkFunc func([]byte) (bool, time.Time, error)) {
	err := db.connection.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(objectType + ":")
		var keysToDelete [][]byte

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			err := item.Value(func(val []byte) error {
				isDeleted, updatedAt, err := checkFunc(val)
				if err != nil {
					db.logger.WithError(err).Error("failed to unmarshal object for cleanup", "object_type", objectType, "key", string(key))
					return nil // Continue processing other items
				}

				if isDeleted && updatedAt.Before(before) {
					// Save key for deletion (can't delete during iteration)
					keyCopy := make([]byte, len(key))
					copy(keyCopy, key)
					keysToDelete = append(keysToDelete, keyCopy)
				}
				return nil
			})
			if err != nil {
				db.logger.WithError(err).Error("failed to read item value during cleanup", "object_type", objectType, "key", string(key))
			}
		}

		// Delete the collected keys
		for _, key := range keysToDelete {
			if err := txn.Delete(key); err != nil {
				db.logger.Error("failed to delete expired object", "error", err, "object_type", objectType, "key", string(key))
			}
		}

		return nil
	})

	if err != nil {
		db.logger.WithError(err).Error("failed to run cleanup transaction", "object_type", objectType)
	}
}
