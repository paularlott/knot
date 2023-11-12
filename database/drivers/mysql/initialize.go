package driver_mysql

import (
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/rs/zerolog/log"
)
func (db *MySQLDriver) initialize() error {

  log.Debug().Msg("db: creating users table")
  _, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS users (
user_id CHAR(36) PRIMARY KEY,
username VARCHAR(32) UNIQUE,
email VARCHAR(255) UNIQUE,
password VARCHAR(255),
active TINYINT NOT NULL DEFAULT 1,
last_login_at TIMESTAMP DEFAULT NULL,
updated_at TIMESTAMP,
created_at TIMESTAMP,
INDEX active (active)
)`)
  if err != nil {
    return err
  }

  log.Debug().Msg("db: creating session table")
  _, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS sessions (
session_id CHAR(36) PRIMARY KEY,
data TEXT,
expires_after TIMESTAMP
)`)
  if err != nil {
    return err
  }

  log.Debug().Msg("db: MySQL is initialized")

  // Add a task to clean up expired sessions
  go func() {
    ticker := time.NewTicker(15 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
    again:
      log.Debug().Msg("db: running GC")
      now := time.Now().UTC()
      _, err := db.connection.Exec("DELETE FROM sessions WHERE expires_after < ?", now)
      if err != nil {
        goto again
      }
    }
  }()

  return nil
}
