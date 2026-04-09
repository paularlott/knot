package driver_redis

import (
	"time"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) HasAuditLog() bool {
	return false
}

func (db *RedisDbDriver) GetNumberOfAuditLogs() (int, error) {
	return 0, nil
}

func (db *RedisDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	return nil
}

func (db *RedisDbDriver) GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error) {
	return []*model.AuditLogEntry{}, nil
}

func (db *RedisDbDriver) GetAuditLogsForExport(from, to *time.Time) ([]*model.AuditLogEntry, error) {
	return []*model.AuditLogEntry{}, nil
}
