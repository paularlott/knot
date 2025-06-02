package driver_badgerdb

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/spf13/viper"
)

func (db *BadgerDbDriver) HasAuditLog() bool {
	return true
}

func (db *BadgerDbDriver) GetNumberOfAuditLogs() (int, error) {
	var count int

	err := db.connection.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("AuditLogs:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}

		return nil
	})

	return count, err
}

func (db *BadgerDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	// Don't save if no retention is configured
	if viper.GetInt("server.audit_retention") < 1 {
		return nil
	}

	keyTimeBuffer := new(bytes.Buffer)
	err := binary.Write(keyTimeBuffer, binary.BigEndian, []byte("AuditLogs:"))
	if err != nil {
		return err
	}

	err = binary.Write(keyTimeBuffer, binary.BigEndian, auditLog.When.UnixMicro())
	if err != nil {
		return err
	}

	err = db.connection.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(auditLog)
		if err != nil {
			return err
		}

		return txn.SetEntry(badger.NewEntry(keyTimeBuffer.Bytes(), data))
	})

	return err
}

func (db *BadgerDbDriver) GetAuditLogs(offset, limit int) ([]*model.AuditLogEntry, error) {
	var auditLogs []*model.AuditLogEntry

	err := db.connection.View(func(txn *badger.Txn) error {
		prefix := []byte("AuditLogs:")

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.PrefetchSize = 10
		opts.Reverse = true // Set to true to iterate from newest to oldest
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		count := 0
		for it.Seek(append(prefix, 0xFF)); it.Valid(); it.Next() {
			item := it.Item()
			if count < offset {
				count++
				continue
			}

			if limit > 0 && count >= offset+limit {
				break
			}
			count++

			var entry model.AuditLogEntry
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal audit log entry: %w", err)
			}

			auditLogs = append(auditLogs, &entry)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return auditLogs, nil
}

func (db *BadgerDbDriver) deleteAuditLogs() error {
	// Don't save if no retention is configured
	if viper.GetInt("server.audit_retention") < 1 {
		return nil
	}

	// Calculate the cutoff time
	before := time.Now()
	before = before.Add(-time.Duration(viper.GetInt("server.audit_retention")) * 24 * time.Hour)
	beforeUnixMicro := before.UTC().UnixMicro()

	// Iterate through the audit logs and delete those older than the cutoff time
	return db.connection.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("AuditLogs:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			timestamp := int64(binary.BigEndian.Uint64(key[len(prefix):]))
			if timestamp < beforeUnixMicro {
				if err := txn.Delete(item.Key()); err != nil {
					return fmt.Errorf("failed to delete audit log entry: %w", err)
				}
			}
		}

		return nil
	})
}
