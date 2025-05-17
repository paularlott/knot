package driver_badgerdb

import (
	"github.com/paularlott/knot/internal/database/model"
)

func (db *BadgerDbDriver) HasAuditLog() bool {
	return false
}

func (db *BadgerDbDriver) GetNumberOfAuditLogs() (int, error) {
	return 0, nil
}

func (db *BadgerDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	return nil
}

func (db *BadgerDbDriver) GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error) {
	return []*model.AuditLogEntry{}, nil
}
