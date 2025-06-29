package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func (db *MySQLDriver) HasAuditLog() bool {
	cfg := config.GetServerConfig()
	return cfg.Audit.Retention > 0
}

func (db *MySQLDriver) GetNumberOfAuditLogs() (int, error) {
	var count int
	err := db.connection.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

func (db *MySQLDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {

	// Don't save if no retention is configured
	cfg := config.GetServerConfig()
	if cfg.Audit.Retention < 1 {
		return nil
	}

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	err = db.create("audit_logs", auditLog)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error) {
	var auditLogs []*model.AuditLogEntry
	var where string

	if limit > 0 {
		where = fmt.Sprintf("1 ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)
	} else {
		where = fmt.Sprintf("1 ORDER BY created_at DESC OFFSET %d", offset)
	}

	err := db.read("audit_logs", &auditLogs, nil, where)
	if err != nil {
		return nil, err
	}

	return auditLogs, nil
}
