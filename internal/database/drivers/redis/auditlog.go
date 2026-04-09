package driver_redis

import (
	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) HasAuditLog() bool {
	return false
}

func (db *RedisDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	return nil
}

func (db *RedisDbDriver) GetAuditLogs(filter *model.AuditLogFilter, offset int, limit int) ([]*model.AuditLogEntry, int, error) {
	return []*model.AuditLogEntry{}, 0, nil
}

func (db *RedisDbDriver) GetAuditLogsForExport(filter *model.AuditLogFilter) ([]*model.AuditLogEntry, error) {
	return []*model.AuditLogEntry{}, nil
}
