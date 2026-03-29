package driver_mysql

// migrations is an ordered list of SQL statements to apply to existing databases.
// Each entry is applied exactly once, tracked by its index (1-based) in the configs table.
// Never remove or reorder entries — only append new ones.
var migrations = []string{
	// 1: add node_id to volumes
	`ALTER TABLE volumes ADD COLUMN IF NOT EXISTS node_id VARCHAR(36) NOT NULL DEFAULT ''`,
}

func (db *MySQLDriver) runMigrations() error {
	// Ensure the migration tracking table exists
	_, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
version INT UNSIGNED NOT NULL PRIMARY KEY
)`)
	if err != nil {
		return err
	}

	for i, sql := range migrations {
		version := i + 1

		var exists bool
		err := db.connection.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", version).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		db.logger.Debug("applying migration", "version", version)
		if _, err := db.connection.Exec(sql); err != nil {
			return err
		}

		if _, err := db.connection.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			return err
		}
	}

	return nil
}
