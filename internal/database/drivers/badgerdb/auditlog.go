package driver_badgerdb

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func (db *BadgerDbDriver) HasAuditLog() bool {
	return true
}

func (db *BadgerDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	cfg := config.GetServerConfig()
	if cfg.Audit.Retention < 1 {
		return nil
	}

	keyTimeBuffer := new(bytes.Buffer)
	err := binary.Write(keyTimeBuffer, binary.BigEndian, []byte("AuditLogs:"))
	if err != nil {
		return err
	}

	err = binary.Write(keyTimeBuffer, binary.BigEndian, struct {
		Year                             int16
		Month, Day, Hour, Minute, Second int8
		Nanosecond                       int64
	}{
		int16(auditLog.When.Year()),
		int8(auditLog.When.Month()),
		int8(auditLog.When.Day()),
		int8(auditLog.When.Hour()),
		int8(auditLog.When.Minute()),
		int8(auditLog.When.Second()),
		auditLog.When.UnixMicro(),
	})
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

func (db *BadgerDbDriver) GetAuditLogs(filter *model.AuditLogFilter, offset, limit int) ([]*model.AuditLogEntry, int, error) {
	var auditLogs []*model.AuditLogEntry
	totalCount := 0

	err := db.connection.View(func(txn *badger.Txn) error {
		prefix := []byte("AuditLogs:")

		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true
		opts.PrefetchSize = 10
		opts.Reverse = true
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		collected := 0
		for it.Seek(append(prefix, 0xFF)); it.Valid(); it.Next() {
			var entry model.AuditLogEntry
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal audit log entry: %w", err)
			}

			if !entry.MatchesFilter(filter) {
				continue
			}

			totalCount++

			if totalCount <= offset {
				continue
			}

			if limit > 0 && collected >= limit {
				continue
			}

			auditLogs = append(auditLogs, &entry)
			collected++
		}
		return nil
	})

	if err != nil {
		return nil, 0, err
	}

	return auditLogs, totalCount, nil
}

func (db *BadgerDbDriver) GetAuditLogsForExport(filter *model.AuditLogFilter) ([]*model.AuditLogEntry, error) {
	var auditLogs []*model.AuditLogEntry

	err := db.connection.View(func(txn *badger.Txn) error {
		prefix := []byte("AuditLogs:")
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var entry model.AuditLogEntry
			err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				return fmt.Errorf("failed to unmarshal audit log entry: %w", err)
			}
			if entry.MatchesFilter(filter) {
				auditLogs = append(auditLogs, &entry)
			}
		}
		return nil
	})

	return auditLogs, err
}

func (db *BadgerDbDriver) deleteAuditLogs() error {
	cfg := config.GetServerConfig()
	if cfg.Audit.Retention < 1 {
		return nil
	}

	before := time.Now()
	before = before.Add(-time.Duration(cfg.Audit.Retention) * 24 * time.Hour)
	beforeUnixMicro := before.UTC().UnixMicro()

	return db.connection.Update(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("AuditLogs:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			timestamp := int64(binary.BigEndian.Uint64(key[len(key)-8:]))
			if timestamp < beforeUnixMicro {
				if err := txn.Delete(item.Key()); err != nil {
					return fmt.Errorf("failed to delete audit log entry: %w", err)
				}
			}
		}

		return nil
	})
}
