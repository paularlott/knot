package driver_mysql

import "fmt"

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
	`ALTER TABLE spaces DROP INDEX IF EXISTS shared_with_user_id`,
	// 6: drop legacy single-share column
	`ALTER TABLE spaces DROP COLUMN IF EXISTS shared_with_user_id`,
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
	// 16: add port_forwards to spaces
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS port_forwards JSON NOT NULL DEFAULT '[]'`,
	// 17: add disable_user_activity to templates
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS disable_user_activity TINYINT(1) NOT NULL DEFAULT 0`,
	// 18: remove disable_user_activity from spaces (was added in pro pre-release)
	`ALTER TABLE spaces DROP COLUMN IF EXISTS disable_user_activity`,
	// 19: reconcile legacy spaces schema with current model
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS parent_space_id CHAR(36) DEFAULT ''`,
	// 20
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT ''`,
	// 21
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS node_id VARCHAR(36) DEFAULT ''`,
	// 22
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS shell VARCHAR(8) DEFAULT ''`,
	// 23
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS template_hash VARCHAR(32) DEFAULT ''`,
	// 24
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS nomad_namespace VARCHAR(255) DEFAULT ''`,
	// 25
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS container_id VARCHAR(255) DEFAULT ''`,
	// 26
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS icon_url VARCHAR(255) NOT NULL DEFAULT ''`,
	// 27
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS volume_data TEXT DEFAULT '{}'`,
	// 28
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS ssh_host_signer TEXT DEFAULT ''`,
	// 29
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS note TEXT DEFAULT ''`,
	// 30
	`ALTER TABLE spaces ADD COLUMN IF NOT EXISTS custom_fields JSON NOT NULL DEFAULT '[]'`,
	// 31: reconcile legacy templates schema with current model
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS icon_url VARCHAR(255) NOT NULL DEFAULT ''`,
	// 32
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS description TEXT DEFAULT ''`,
	// 33
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS job MEDIUMTEXT`,
	// 34
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS volumes MEDIUMTEXT`,
	// 35
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS groups JSON NOT NULL DEFAULT '[]'`,
	// 36
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS schedule JSON DEFAULT NULL`,
	// 37
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS zones JSON NOT NULL DEFAULT '[]'`,
	// 38
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS custom_fields JSON NOT NULL DEFAULT '[]'`,
	// 39
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS with_vscode_tunnel TINYINT(1) NOT NULL DEFAULT 0`,
	// 40
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT ''`,
	// 41
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS shutdown_script_id CHAR(36) DEFAULT ''`,
	// 42
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS user_startup_script VARCHAR(64) DEFAULT ''`,
	// 43
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS user_shutdown_script VARCHAR(64) DEFAULT ''`,
	// 44
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS schedule_enabled TINYINT(1) NOT NULL DEFAULT 0`,
	// 45
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS auto_start TINYINT(1) NOT NULL DEFAULT 0`,
	// 46
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS compute_units INT UNSIGNED NOT NULL DEFAULT 0`,
	// 47
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS storage_units INT UNSIGNED NOT NULL DEFAULT 0`,
	// 48
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS max_uptime INT UNSIGNED NOT NULL DEFAULT 0`,
	// 49
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS max_uptime_unit VARCHAR(16) DEFAULT 'disabled'`,
	// 50
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS allow_node_migration TINYINT(1) NOT NULL DEFAULT 0`,
	// 51: add SSH private key storage to users
	`ALTER TABLE users ADD COLUMN IF NOT EXISTS ssh_private_key TEXT DEFAULT ''`,
	// 52: add ports to templates
	`ALTER TABLE templates ADD COLUMN IF NOT EXISTS ports JSON NOT NULL DEFAULT '[]'`,
}

func (db *MySQLDriver) runMigrations() error {
	_, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
version INT UNSIGNED NOT NULL PRIMARY KEY
)`)
	if err != nil {
		return err
	}

	var current int
	db.connection.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current)

	for i, sql := range migrations {
		version := i + 1
		if version <= current {
			continue
		}

		if version == 4 {
			exists, err := db.columnExists("spaces", "shared_with_user_id")
			if err != nil {
				return err
			}
			if !exists {
				db.logger.Debug("skipping migration", "version", version, "reason", "legacy shared_with_user_id column does not exist")
				if _, err := db.connection.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
					return err
				}
				continue
			}
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

func (db *MySQLDriver) columnExists(tableName, columnName string) (bool, error) {
	var count int
	err := db.connection.QueryRow(
		`SELECT COUNT(*)
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`,
		tableName,
		columnName,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check column %s.%s: %w", tableName, columnName, err)
	}

	return count > 0, nil
}
