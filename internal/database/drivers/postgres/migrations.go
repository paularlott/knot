package driver_postgres

// migrations is an ordered list of SQL statements to apply to existing databases.
// Each entry is applied exactly once, tracked by its index (1-based) in the
// schema_migrations table. Never remove or reorder entries — only append new
// ones.
//
// A new database is created from the current schema in initialize.go and then
// runs every migration too, so each entry must be safe on a schema that
// already has its change: use ADD COLUMN IF NOT EXISTS, CREATE INDEX IF NOT
// EXISTS and so on.
var migrations = []string{}

func (db *PostgresDriver) runMigrations() error {
	_, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
version INTEGER NOT NULL PRIMARY KEY
)`)
	if err != nil {
		return err
	}

	var current int
	if err := db.connection.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
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
