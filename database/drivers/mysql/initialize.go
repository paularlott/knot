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
username VARCHAR(64) UNIQUE,
email VARCHAR(255) UNIQUE,
password VARCHAR(255),
preferred_shell VARCHAR(8) DEFAULT 'zsh',
ssh_public_key TEXT DEFAULT '',
roles TEXT DEFAULT '',
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
user_id CHAR(36),
ip VARCHAR(15),
user_agent VARCHAR(255),
data TEXT,
expires_after TIMESTAMP,
INDEX expires_after (expires_after),
INDEX user_id (user_id)
)`)
  if err != nil {
    return err
  }

  log.Debug().Msg("db: creating API tokens table")
  _, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS tokens (
token_id CHAR(36) PRIMARY KEY,
user_id CHAR(36),
name VARCHAR(255),
expires_after TIMESTAMP,
INDEX expires_after (expires_after),
INDEX user_id (user_id)
)`)
  if err != nil {
    return err
  }

  log.Debug().Msg("db: creating spaces table")
  _, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS spaces (
space_id CHAR(36) PRIMARY KEY,
user_id CHAR(36),
template_id CHAR(36),
name VARCHAR(64),
agent_url VARCHAR(255),
shell VARCHAR(8),
created_at TIMESTAMP,
updated_at TIMESTAMP,
INDEX user_id (user_id),
INDEX template_id (template_id),
UNIQUE INDEX name (user_id, name)
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

      _, err = db.connection.Exec("DELETE FROM tokens WHERE expires_after < ?", now)
      if err != nil {
        goto again
      }
    }
  }()

  return nil
}
