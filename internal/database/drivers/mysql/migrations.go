package driver_mysql

// migrations is an ordered list of SQL statements to apply to existing databases.
// Each entry is applied exactly once, tracked by its index (1-based) in the configs table.
// Never remove or reorder entries — only append new ones.
var migrations = []string{
	// 1: add node_id to volumes
	`ALTER TABLE volumes ADD COLUMN IF NOT EXISTS node_id VARCHAR(36) NOT NULL DEFAULT ''`,
	// 2: add external_auth_providers to users
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS external_auth_providers JSON DEFAULT NULL`,
	// 3: add multi-share storage to spaces
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS shares JSON NOT NULL DEFAULT '[]'`,
	// 4: migrate legacy single-share values into shares json
	`UPDATE spaces SET shares = JSON_ARRAY(shared_with_user_id) WHERE shared_with_user_id <> '' AND JSON_LENGTH(shares) = 0`,
	// 5: drop legacy single-share index
	`ALTER TABLE spaces DROP INDEX shared_with_user_id`,
	// 6: drop legacy single-share column
	`ALTER TABLE spaces DROP COLUMN shared_with_user_id`,
	// 7: add space dependency storage
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS depends_on JSON NOT NULL DEFAULT '[]'`,
	// 8: add health check fields to templates
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_type VARCHAR(16) NOT NULL DEFAULT 'none'`,
	// 9
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_config TEXT NOT NULL DEFAULT ''`,
	// 10
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_skip_ssl_verify TINYINT(1) NOT NULL DEFAULT 0`,
	// 11
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_timeout INT UNSIGNED NOT NULL DEFAULT 10`,
	// 12
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_interval INT UNSIGNED NOT NULL DEFAULT 30`,
	// 13
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_max_failures INT UNSIGNED NOT NULL DEFAULT 3`,
	// 14
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS health_check_auto_restart TINYINT(1) NOT NULL DEFAULT 0`,
	// 15: add stack field to spaces
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS stack VARCHAR(255) DEFAULT ''`,
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
