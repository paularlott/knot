package driver_mysql

import (
	"fmt"

	"github.com/paularlott/knot/database/model"

	"github.com/spf13/viper"
)

func (db *MySQLDriver) HasAuditLog() bool {
	return viper.GetInt("server.audit_retention") > 0
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
	if viper.GetInt("server.audit_retention") < 1 {
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

	err := db.read("audit_logs", &auditLogs, nil, fmt.Sprintf("1 ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset))
	if err != nil {
		fmt.Println("Error reading audit logs:", err)
		return nil, err
	}

	return auditLogs, nil
}
