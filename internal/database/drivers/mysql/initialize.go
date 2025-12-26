package driver_mysql

import (
	"time"

	"github.com/paularlott/knot/internal/config"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) initialize() error {

	db.logger.Debug("creating users table")
	_, err := db.connection.Exec(`CREATE TABLE IF NOT EXISTS users (
user_id CHAR(36) PRIMARY KEY,
username VARCHAR(64) UNIQUE,
email VARCHAR(255) UNIQUE,
password VARCHAR(255),
totp_secret VARCHAR(16) DEFAULT '',
service_password VARCHAR(255),
preferred_shell VARCHAR(8) DEFAULT 'zsh',
timezone VARCHAR(128) DEFAULT 'UTC',
ssh_public_key TEXT DEFAULT '',
github_username VARCHAR(255) DEFAULT '',
roles JSON DEFAULT NULL,
groups JSON DEFAULT NULL,
active TINYINT(1) NOT NULL DEFAULT 1,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
max_spaces INT UNSIGNED NOT NULL DEFAULT 0,
compute_units INT UNSIGNED NOT NULL DEFAULT 0,
storage_units INT UNSIGNED NOT NULL DEFAULT 0,
max_tunnels INT UNSIGNED NOT NULL DEFAULT 0,
last_login_at TIMESTAMP(6) DEFAULT NULL,
updated_at BIGINT UNSIGNED DEFAULT 0,
created_at TIMESTAMP(6),
INDEX active (active),
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating API tokens table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS tokens (
token_id CHAR(64) PRIMARY KEY,
user_id CHAR(36),
name VARCHAR(255),
expires_after TIMESTAMP(6),
updated_at BIGINT UNSIGNED DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
INDEX expires_after (expires_after),
INDEX user_id (user_id),
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating spaces table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS spaces (
space_id CHAR(36) PRIMARY KEY,
parent_space_id CHAR(36) DEFAULT '',
user_id CHAR(36),
template_id CHAR(36) DEFAULT '',
shared_with_user_id CHAR(36) DEFAULT '',
name VARCHAR(64),
zone VARCHAR(64),
node_id VARCHAR(36) DEFAULT '',
shell VARCHAR(8) DEFAULT '',
template_hash VARCHAR(32) DEFAUlT '',
nomad_namespace VARCHAR(255) DEFAULT '',
container_id VARCHAR(255) DEFAULT '',
icon_url VARCHAR(255) NOT NULL DEFAULT '',
volume_data TEXT DEFAULT '{}',
ssh_host_signer TEXT DEFAULT '',
description TEXT DEFAULT '',
note TEXT DEFAULT '',
custom_fields JSON NOT NULL DEFAULT '[]',
is_deployed TINYINT(1) NOT NULL DEFAULT 0,
is_pending TINYINT(1) NOT NULL DEFAULT 0,
is_deleting TINYINT(1) NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
started_at TIMESTAMP(6) DEFAULT CURRENT_TIMESTAMP,
created_at TIMESTAMP(6),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX user_id (user_id),
INDEX template_id (template_id),
UNIQUE INDEX name (user_id, name),
INDEX parent_space_id (parent_space_id),
INDEX shared_with_user_id (shared_with_user_id),
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating templates table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS templates (
template_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
hash VARCHAR(32) DEFAUlT '',
platform VARCHAR(64) DEFAULT '',
icon_url VARCHAR(255) NOT NULL DEFAULT '',
description TEXT DEFAULT '',
job MEDIUMTEXT,
volumes MEDIUMTEXT,
groups JSON NOT NULL DEFAULT '[]',
schedule JSON DEFAULT NULL,
zones JSON NOT NULL DEFAULT '[]',
custom_fields JSON NOT NULL DEFAULT '[]',
with_terminal TINYINT(1) NOT NULL DEFAULT 1,
with_vscode_tunnel TINYINT(1) NOT NULL DEFAULT 0,
with_code_server TINYINT(1) NOT NULL DEFAULT 0,
with_ssh TINYINT(1) NOT NULL DEFAULT 0,
schedule_enabled TINYINT(1) NOT NULL DEFAULT 0,
auto_start TINYINT(1) NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
active TINYINT(1) NOT NULL DEFAULT 1,
is_managed TINYINT(1) NOT NULL DEFAULT 0,
compute_units INT UNSIGNED NOT NULL DEFAULT 0,
storage_units INT UNSIGNED NOT NULL DEFAULT 0,
max_uptime INT UNSIGNED NOT NULL DEFAULT 0,
max_uptime_unit VARCHAR(16) DEFAULT 'disabled',
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating groups table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS groups (
group_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
max_spaces INT UNSIGNED NOT NULL DEFAULT 0,
compute_units INT UNSIGNED NOT NULL DEFAULT 0,
storage_units INT UNSIGNED NOT NULL DEFAULT 0,
max_tunnels INT UNSIGNED NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating template variables table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS templatevars (
templatevar_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
zones JSON NOT NULL DEFAULT '[]',
value MEDIUMTEXT,
protected TINYINT(1) NOT NULL DEFAULT 0,
local TINYINT(1) NOT NULL DEFAULT 0,
restricted TINYINT(1) NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
is_managed TINYINT(1) NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating volumes table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS volumes (
volume_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
zone VARCHAR(64),
platform VARCHAR(64) DEFAULT '',
definition MEDIUMTEXT,
active TINYINT(1) NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX zone (zone),
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating scripts table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS scripts (
script_id CHAR(36) PRIMARY KEY,
name VARCHAR(64) UNIQUE,
description TEXT DEFAULT '',
content MEDIUMTEXT,
groups JSON NOT NULL DEFAULT '[]',
active TINYINT(1) NOT NULL DEFAULT 1,
script_type VARCHAR(16) DEFAULT 'script',
mcp_input_schema_toml TEXT DEFAULT '',
mcp_keywords JSON NOT NULL DEFAULT '[]',
timeout INT UNSIGNED NOT NULL DEFAULT 0,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX idx_is_deleted (is_deleted),
INDEX script_type (script_type)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating roles table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS roles (
role_id CHAR(36) PRIMARY KEY,
name VARCHAR(64),
permissions JSON DEFAULT NULL,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
created_user_id CHAR(36),
created_at TIMESTAMP(6),
updated_user_id CHAR(36),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating responses table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS responses (
response_id CHAR(36) PRIMARY KEY,
status VARCHAR(32) NOT NULL DEFAULT 'pending',
request JSON DEFAULT NULL,
response JSON DEFAULT NULL,
error_text TEXT DEFAULT '',
previous_response_id CHAR(36) DEFAULT '',
user_id CHAR(36),
space_id CHAR(36) DEFAULT '',
expires_at TIMESTAMP(6) DEFAULT NULL,
is_deleted TINYINT(1) NOT NULL DEFAULT 0,
created_at TIMESTAMP(6),
updated_at BIGINT UNSIGNED DEFAULT 0,
INDEX user_id (user_id),
INDEX status (status),
INDEX expires_at (expires_at),
INDEX idx_is_deleted (is_deleted)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating audit_log table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS audit_logs (
audit_log_id bigint(20) NOT NULL AUTO_INCREMENT PRIMARY KEY,
created_at DATETIME(6) DEFAULT CURRENT_TIMESTAMP,
zone VARCHAR(64) DEFAULT '',
actor VARCHAR(255),
actor_type VARCHAR(255),
event VARCHAR(255),
details MEDIUMTEXT,
properties JSON DEFAULT NULL,
INDEX actor (actor, actor_type),
INDEX event (event),
INDEX created_at (created_at)
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("creating configs table")
	_, err = db.connection.Exec(`CREATE TABLE IF NOT EXISTS configs (
name VARCHAR(64) PRIMARY KEY,
value MEDIUMTEXT
)`)
	if err != nil {
		return err
	}

	db.logger.Debug("MySQL is initialized")

	// Add a task to clean up expired data
	db.logger.Debug("starting database GC")
	cfg := config.GetServerConfig()
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
		again:
			db.logger.Debug("running GC")
			now := time.Now().UTC()

			_, err = db.connection.Exec("DELETE FROM tokens WHERE expires_after < ?", now)
			if err != nil {
				goto again
			}

			// Remove old audit logs
			if cfg.Audit.Retention > 0 {
				_, err = db.connection.Exec("DELETE FROM audit_logs WHERE created_at < ?", now.Add(-time.Duration(24*cfg.Audit.Retention)*time.Hour).UTC())
				if err != nil {
					goto again
				}
			}

			// Remove expired responses (both TTL expired and soft-deleted past grace period)
			// Soft-deleted responses have expires_at set to deleted_at + 7 days
			_, err = db.connection.Exec("DELETE FROM responses WHERE expires_at < ?", now)
			if err != nil {
				goto again
			}
		}
	}()

	return nil
}
