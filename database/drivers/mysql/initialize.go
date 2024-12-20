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
service_password VARCHAR(255),
preferred_shell VARCHAR(8) DEFAULT 'zsh',
timezone VARCHAR(128) DEFAULT 'UTC',
ssh_public_key TEXT DEFAULT '',
github_username VARCHAR(255) DEFAULT '',
roles JSON DEFAULT NULL,
groups JSON DEFAULT NULL,
active TINYINT NOT NULL DEFAULT 1,
max_spaces INT UNSIGNED NOT NULL DEFAULT 0,
compute_units INT UNSIGNED NOT NULL DEFAULT 0,
storage_units INT UNSIGNED NOT NULL DEFAULT 0,
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
remote_session_id CHAR(36),
ip VARCHAR(256),
user_agent VARCHAR(255),
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
session_id CHAR(36),
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
parent_space_id CHAR(36) DEFAULT '',
user_id CHAR(36),
template_id CHAR(36) DEFAULT '',
name VARCHAR(64),
location VARCHAR(64),
shell VARCHAR(8) DEFAULT '',
template_hash VARCHAR(32) DEFAUlT '',
nomad_namespace VARCHAR(255) DEFAULT '',
container_id VARCHAR(255) DEFAULT '',
volume_data TEXT DEFAULT '{}',
ssh_host_signer TEXT DEFAULT '',
is_deployed TINYINT NOT NULL DEFAULT 0,
is_pending TINYINT NOT NULL DEFAULT 0,
is_deleting TINYINT NOT NULL DEFAULT 0,
created_at TIMESTAMP,
updated_at TIMESTAMP,
INDEX user_id (user_id),
INDEX template_id (template_id),
UNIQUE INDEX name (user_id, name),
INDEX parent_space_id (parent_space_id)
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: creating templates table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS templates (
template_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
hash VARCHAR(32) DEFAUlT '',
description TEXT DEFAULT '',
job MEDIUMTEXT,
volumes MEDIUMTEXT,
groups JSON DEFAULT NULL,
schedule JSON DEFAULT NULL,
local_container TINYINT NOT NULL DEFAULT 0,
is_manual TINYINT NOT NULL DEFAULT 0,
with_terminal TINYINT NOT NULL DEFAULT 1,
with_vscode_tunnel TINYINT NOT NULL DEFAULT 0,
with_code_server TINYINT NOT NULL DEFAULT 0,
with_ssh TINYINT NOT NULL DEFAULT 0,
schedule_enabled TINYINT NOT NULL DEFAULT 0,
compute_units INT UNSIGNED NOT NULL DEFAULT 0,
storage_units INT UNSIGNED NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP,
updated_user_id CHAR(36),
updated_at TIMESTAMP
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: creating groups table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS groups (
group_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
created_user_id CHAR(36),
created_at TIMESTAMP,
updated_user_id CHAR(36),
updated_at TIMESTAMP
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: creating template variables table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS templatevars (
templatevar_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
location VARCHAR(64),
value MEDIUMTEXT,
protected TINYINT NOT NULL DEFAULT 0,
local TINYINT NOT NULL DEFAULT 0,
restricted TINYINT NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP,
updated_user_id CHAR(36),
updated_at TIMESTAMP
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: creating volumes table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS volumes (
volume_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
location VARCHAR(64),
definition MEDIUMTEXT,
active TINYINT NOT NULL DEFAULT 0,
local_container TINYINT NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP,
updated_user_id CHAR(36),
updated_at TIMESTAMP,
INDEX location (location)
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: creating roles table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS roles (
role_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
permissions JSON DEFAULT NULL,
created_user_id CHAR(36),
created_at TIMESTAMP,
updated_user_id CHAR(36),
updated_at TIMESTAMP
)`)
	if err != nil {
		return err
	}

	log.Debug().Msg("db: MySQL is initialized")

	// Add a task to clean up expired sessions
	log.Debug().Msg("db: starting session GC")
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
