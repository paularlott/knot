package driver_mysql

import (
	"encoding/json"
	"time"

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

	propertiesJSON, err := json.Marshal(auditLog.Properties)
	if err != nil {
		tx.Rollback()
		return err
	}

	_, err = tx.Exec("INSERT INTO audit_logs (actor, actor_type, event, created_at, details, properties) VALUES (?, ?, ?, ?, ?, ?)",
		auditLog.Actor, auditLog.ActorType, auditLog.Event, auditLog.When, auditLog.Details, propertiesJSON,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) GetAuditLogs(offset int, limit int) ([]*model.AuditLogEntry, error) {
	var auditLogs []*model.AuditLogEntry

	rows, err := db.connection.Query("SELECT audit_log_id, actor, actor_type, event, created_at, details, properties FROM audit_logs WHERE created_at ORDER BY created_at DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var auditLog model.AuditLogEntry
		var propertiesJSON []byte
		var when string
		err := rows.Scan(&auditLog.Id, &auditLog.Actor, &auditLog.ActorType, &auditLog.Event, &when, &auditLog.Details, &propertiesJSON)
		if err != nil {
			return nil, err
		}

		auditLog.When, err = time.Parse("2006-01-02 15:04:05", when)
		if err != nil {
			return nil, err
		}

		err = json.Unmarshal(propertiesJSON, &auditLog.Properties)
		if err != nil {
			return nil, err
		}
		auditLogs = append(auditLogs, &auditLog)
	}

	return auditLogs, nil
}
