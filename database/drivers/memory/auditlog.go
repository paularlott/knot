package driver_memory

import (
	"errors"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) HasAuditLog() bool {
	return false
}

func (db *MemoryDbDriver) GetNumberOfAuditLogs() (int, error) {
	return 0, errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
	return errors.New("memorydb: not implemented")
}

func (db *MemoryDbDriver) GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error) {
	return nil, errors.New("memorydb: not implemented")
}
