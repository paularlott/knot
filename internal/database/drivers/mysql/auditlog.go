package driver_mysql

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
)

func (db *MySQLDriver) HasAuditLog() bool {
	cfg := config.GetServerConfig()
	return cfg.Audit.Retention > 0
}

func (db *MySQLDriver) SaveAuditLog(auditLog *model.AuditLogEntry) error {
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

func buildAuditWhere(filter *model.AuditLogFilter) (string, []interface{}) {
	clauses := []string{"1"}
	var args []interface{}

	if filter == nil {
		return "1", args
	}

	if filter.Actor != "" {
		clauses = append(clauses, "actor = ?")
		args = append(args, filter.Actor)
	}
	if filter.ActorType != "" {
		clauses = append(clauses, "actor_type = ?")
		args = append(args, filter.ActorType)
	}
	if filter.Event != "" {
		clauses = append(clauses, "event LIKE ?")
		args = append(args, "%"+filter.Event+"%")
	}
	if filter.From != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, filter.From.UTC())
	}
	if filter.To != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, filter.To.UTC())
	}
	if filter.Query != "" {
		clauses = append(clauses, "(actor LIKE ? OR event LIKE ? OR details LIKE ? OR properties LIKE ?)")
		q := "%" + filter.Query + "%"
		args = append(args, q, q, q, q)
	}

	return strings.Join(clauses, " AND "), args
}

func (db *MySQLDriver) GetAuditLogs(filter *model.AuditLogFilter, offset int, limit int) ([]*model.AuditLogEntry, int, error) {
	where, args := buildAuditWhere(filter)

	// Get count
	var count int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := db.connection.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE "+where, countArgs...).Scan(&count)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	var auditLogs []*model.AuditLogEntry
	if limit > 0 {
		where += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)
	}

	err = db.read("audit_logs", &auditLogs, nil, where, args...)
	if err != nil {
		return nil, 0, err
	}

	return auditLogs, count, nil
}

func (db *MySQLDriver) GetAuditLogsForExport(filter *model.AuditLogFilter) ([]*model.AuditLogEntry, error) {
	where, args := buildAuditWhere(filter)
	where += " ORDER BY created_at ASC"

	var auditLogs []*model.AuditLogEntry
	err := db.read("audit_logs", &auditLogs, nil, where, args...)
	if err != nil {
		return nil, err
	}
	return auditLogs, nil
}
