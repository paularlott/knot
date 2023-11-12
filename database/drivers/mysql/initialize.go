package driver_mysql

import (
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
updated_at TIMESTAMP,
created_at TIMESTAMP,
INDEX active (active)
)`)
  if err != nil {
    return err
  }



  log.Debug().Msg("db: MySQL is initialized")

  return nil
}
