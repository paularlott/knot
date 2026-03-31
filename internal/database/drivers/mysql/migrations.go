package driver_mysql

// migrations is an ordered list of SQL statements to apply to existing databases.
// Each entry is applied exactly once, tracked by its index (1-based) in the configs table.
// Never remove or reorder entries — only append new ones.
var migrations = []string{
	// 1: add node_id to volumes
	`ALTER TABLE volumes ADD COLUMN IF NOT EXISTS node_id VARCHAR(36) NOT NULL DEFAULT ''`,
	// 2: add external_auth_providers to users
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS external_auth_providers JSON DEFAULT NULL`,
}

func (db *MySQLDriver) runMigrations() error {
	// Ensure the migration tracking table exists
	_, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
version INT UNSIGNED NOT NULL PRIMARY KEY
)`)
	if err != nil {
		return err
	}

	var current int
	db.connection.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current)

	// Fresh install: schema is already up to date, seed the max version to skip all migrations
	if current == 0 && len(migrations) > 0 {
		_, err = db.connection.Exec("INSERT INTO schema_migrations (version) VALUES (?)", len(migrations))
		return err
	}

	for i, sql := range migrations {
		version := i + 1
		if version <= current {
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
