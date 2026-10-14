package driver_postgres

import (
	"time"

	"github.com/paularlott/knot/internal/config"
)

// schema creates the tables and indexes of a new database. A schema change
// goes in both places: here for new databases and appended to migrations for
// existing ones.
var schema = []string{
	`CREATE TABLE IF NOT EXISTS users (
user_id VARCHAR(36) PRIMARY KEY,
username VARCHAR(64) UNIQUE,
email VARCHAR(255) UNIQUE,
password VARCHAR(255),
totp_secret VARCHAR(16) DEFAULT '',
service_password VARCHAR(255),
preferred_shell VARCHAR(8) DEFAULT 'zsh',
timezone VARCHAR(128) DEFAULT 'UTC',
ssh_public_key TEXT DEFAULT '',
ssh_private_key TEXT DEFAULT '',
onepassword_token TEXT DEFAULT '',
github_username VARCHAR(255) DEFAULT '',
roles JSONB DEFAULT NULL,
"groups" JSONB DEFAULT NULL,
linked_users JSONB NOT NULL DEFAULT '[]',
external_auth_providers JSONB DEFAULT NULL,
preferences JSONB DEFAULT NULL,
active BOOLEAN NOT NULL DEFAULT TRUE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
max_spaces BIGINT NOT NULL DEFAULT 0,
compute_units BIGINT NOT NULL DEFAULT 0,
storage_units BIGINT NOT NULL DEFAULT 0,
max_tunnels BIGINT NOT NULL DEFAULT 0,
file_storage_mb BIGINT NOT NULL DEFAULT 0,
max_buckets BIGINT NOT NULL DEFAULT 0,
last_login_at TIMESTAMPTZ DEFAULT NULL,
updated_at BIGINT DEFAULT 0,
created_at TIMESTAMPTZ
)`,
	`CREATE INDEX IF NOT EXISTS users_active ON users (active)`,
	`CREATE INDEX IF NOT EXISTS users_is_deleted ON users (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS tokens (
token_id VARCHAR(64) PRIMARY KEY,
user_id VARCHAR(36),
name VARCHAR(255),
scopes JSONB DEFAULT NULL,
refresh_token BOOLEAN NOT NULL DEFAULT FALSE,
expires_after TIMESTAMPTZ,
updated_at BIGINT DEFAULT 0,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE
)`,
	`CREATE INDEX IF NOT EXISTS tokens_expires_after ON tokens (expires_after)`,
	`CREATE INDEX IF NOT EXISTS tokens_user_id ON tokens (user_id)`,
	`CREATE INDEX IF NOT EXISTS tokens_is_deleted ON tokens (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS spaces (
space_id VARCHAR(36) PRIMARY KEY,
parent_space_id VARCHAR(36) DEFAULT '',
pool_id VARCHAR(36) DEFAULT '',
user_id VARCHAR(36),
template_id VARCHAR(36) DEFAULT '',
shares JSONB NOT NULL DEFAULT '[]',
depends_on JSONB NOT NULL DEFAULT '[]',
startup_script_id VARCHAR(36) DEFAULT '',
name VARCHAR(64),
zone VARCHAR(64),
node_id VARCHAR(36) DEFAULT '',
shell VARCHAR(8) DEFAULT '',
template_hash VARCHAR(32) DEFAULT '',
nomad_namespace VARCHAR(255) DEFAULT '',
container_id VARCHAR(255) DEFAULT '',
ip_address VARCHAR(64) DEFAULT '',
icon_url VARCHAR(255) NOT NULL DEFAULT '',
volume_data TEXT DEFAULT '{}',
ssh_host_signer TEXT DEFAULT '',
description TEXT DEFAULT '',
note TEXT DEFAULT '',
stack VARCHAR(255) DEFAULT '',
stack_prefix VARCHAR(255) DEFAULT '',
custom_fields JSONB NOT NULL DEFAULT '[]',
port_forwards JSONB NOT NULL DEFAULT '[]',
jobs JSONB NOT NULL DEFAULT '[]',
jobs_enabled BOOLEAN NOT NULL DEFAULT TRUE,
restrictions JSONB NOT NULL DEFAULT '{}',
lease_id VARCHAR(36) DEFAULT '',
lease_user_id VARCHAR(36) DEFAULT '',
lease_expires_at TIMESTAMPTZ NULL,
lease_extensions INTEGER NOT NULL DEFAULT 0,
is_deployed BOOLEAN NOT NULL DEFAULT FALSE,
is_pending BOOLEAN NOT NULL DEFAULT FALSE,
is_deleting BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
started_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
created_at TIMESTAMPTZ,
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS spaces_user_id ON spaces (user_id)`,
	`CREATE INDEX IF NOT EXISTS spaces_pool_id ON spaces (pool_id)`,
	`CREATE INDEX IF NOT EXISTS spaces_template_id ON spaces (template_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS spaces_name ON spaces (user_id, name)`,
	`CREATE INDEX IF NOT EXISTS spaces_parent_space_id ON spaces (parent_space_id)`,
	`CREATE INDEX IF NOT EXISTS spaces_is_deleted ON spaces (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS pools (
pool_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64) NOT NULL,
template_id VARCHAR(36) NOT NULL,
startup_script_id VARCHAR(36) DEFAULT '',
desired_count BIGINT NOT NULL DEFAULT 1,
active BOOLEAN NOT NULL DEFAULT TRUE,
zone VARCHAR(64) DEFAULT '',
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
lease_max_time INTEGER NOT NULL DEFAULT 0,
lease_max_extensions INTEGER NOT NULL DEFAULT 0,
custom_fields JSONB NOT NULL DEFAULT '[]',
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS pools_created_user_name ON pools (created_user_id, name)`,
	`CREATE INDEX IF NOT EXISTS pools_template_id ON pools (template_id)`,
	`CREATE INDEX IF NOT EXISTS pools_active ON pools (active)`,
	`CREATE INDEX IF NOT EXISTS pools_is_deleted ON pools (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS space_usage (
space_usage_id VARCHAR(64) PRIMARY KEY,
space_id VARCHAR(36) NOT NULL,
user_id VARCHAR(36) NOT NULL,
bucket_kind VARCHAR(16) NOT NULL DEFAULT 'minute',
bucket_start TIMESTAMPTZ NOT NULL,
cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
memory_used_bytes BIGINT NOT NULL DEFAULT 0,
memory_limit_bytes BIGINT NOT NULL DEFAULT 0,
disk_used_bytes BIGINT NOT NULL DEFAULT 0,
disk_limit_bytes BIGINT NOT NULL DEFAULT 0,
activity_write_count BIGINT NOT NULL DEFAULT 0,
activity_create_count BIGINT NOT NULL DEFAULT 0,
activity_delete_count BIGINT NOT NULL DEFAULT 0,
activity_rename_count BIGINT NOT NULL DEFAULT 0,
activity_distinct_paths BIGINT NOT NULL DEFAULT 0,
activity_space_starts BIGINT NOT NULL DEFAULT 0,
activity_space_stops BIGINT NOT NULL DEFAULT 0,
activity_space_creates BIGINT NOT NULL DEFAULT 0,
activity_space_deletes BIGINT NOT NULL DEFAULT 0,
last_activity_at TIMESTAMPTZ DEFAULT NULL,
created_at TIMESTAMPTZ,
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS space_usage_space_time ON space_usage (space_id, bucket_kind, bucket_start)`,
	`CREATE INDEX IF NOT EXISTS space_usage_user_time ON space_usage (user_id, bucket_kind, bucket_start)`,
	`CREATE INDEX IF NOT EXISTS space_usage_bucket ON space_usage (bucket_kind, bucket_start)`,
	`CREATE TABLE IF NOT EXISTS templates (
template_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64),
hash VARCHAR(32) DEFAULT '',
platform VARCHAR(64) DEFAULT '',
icon_url VARCHAR(255) NOT NULL DEFAULT '',
description TEXT DEFAULT '',
job TEXT,
volumes TEXT,
"groups" JSONB NOT NULL DEFAULT '[]',
schedule JSONB DEFAULT NULL,
zones JSONB NOT NULL DEFAULT '[]',
custom_fields JSONB NOT NULL DEFAULT '[]',
with_terminal BOOLEAN NOT NULL DEFAULT TRUE,
with_vscode_tunnel BOOLEAN NOT NULL DEFAULT FALSE,
with_code_server BOOLEAN NOT NULL DEFAULT FALSE,
with_ssh BOOLEAN NOT NULL DEFAULT FALSE,
with_run_command BOOLEAN NOT NULL DEFAULT FALSE,
with_vnc BOOLEAN NOT NULL DEFAULT FALSE,
allow_node_migration BOOLEAN NOT NULL DEFAULT FALSE,
startup_script_id VARCHAR(36) DEFAULT '',
shutdown_script_id VARCHAR(36) DEFAULT '',
user_startup_script VARCHAR(64) DEFAULT '',
user_shutdown_script VARCHAR(64) DEFAULT '',
schedule_enabled BOOLEAN NOT NULL DEFAULT FALSE,
auto_start BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
active BOOLEAN NOT NULL DEFAULT TRUE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
compute_units BIGINT NOT NULL DEFAULT 0,
storage_units BIGINT NOT NULL DEFAULT 0,
max_uptime BIGINT NOT NULL DEFAULT 0,
max_uptime_unit VARCHAR(16) DEFAULT 'disabled',
idle_timeout BIGINT NOT NULL DEFAULT 0,
idle_timeout_unit VARCHAR(16) DEFAULT 'disabled',
health_check_type VARCHAR(16) NOT NULL DEFAULT 'none',
health_check_config TEXT NOT NULL DEFAULT '',
health_check_skip_ssl_verify BOOLEAN NOT NULL DEFAULT FALSE,
health_check_timeout BIGINT NOT NULL DEFAULT 10,
health_check_interval BIGINT NOT NULL DEFAULT 30,
health_check_max_failures BIGINT NOT NULL DEFAULT 3,
health_check_auto_restart BOOLEAN NOT NULL DEFAULT FALSE,
disable_user_activity BOOLEAN NOT NULL DEFAULT FALSE,
disable_file_storage BOOLEAN NOT NULL DEFAULT FALSE,
disable_user_secrets BOOLEAN NOT NULL DEFAULT FALSE,
ports JSONB NOT NULL DEFAULT '[]',
port_forwards JSONB NOT NULL DEFAULT '[]',
jobs JSONB NOT NULL DEFAULT '[]',
kvm_network_mode VARCHAR(16) NOT NULL DEFAULT '',
kvm_network_cidr VARCHAR(64) NOT NULL DEFAULT '',
kvm_ip_range_start VARCHAR(64) NOT NULL DEFAULT '',
kvm_ip_range_end VARCHAR(64) NOT NULL DEFAULT '',
kvm_gateway VARCHAR(64) NOT NULL DEFAULT '',
kvm_bridge VARCHAR(64) NOT NULL DEFAULT '',
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS templates_is_deleted ON templates (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS "groups" (
group_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64),
max_spaces BIGINT NOT NULL DEFAULT 0,
compute_units BIGINT NOT NULL DEFAULT 0,
storage_units BIGINT NOT NULL DEFAULT 0,
max_tunnels BIGINT NOT NULL DEFAULT 0,
file_storage_mb BIGINT NOT NULL DEFAULT 0,
max_buckets BIGINT NOT NULL DEFAULT 0,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS groups_is_deleted ON "groups" (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS templatevars (
templatevar_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64),
zones JSONB NOT NULL DEFAULT '[]',
value TEXT,
protected BOOLEAN NOT NULL DEFAULT FALSE,
local BOOLEAN NOT NULL DEFAULT FALSE,
restricted BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS templatevars_is_deleted ON templatevars (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS volumes (
volume_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64),
zone VARCHAR(64),
node_id VARCHAR(36) DEFAULT '',
platform VARCHAR(64) DEFAULT '',
definition TEXT,
active BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS volumes_zone ON volumes (zone)`,
	`CREATE INDEX IF NOT EXISTS volumes_is_deleted ON volumes (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS scripts (
script_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
name VARCHAR(64),
description TEXT DEFAULT '',
content TEXT,
"groups" JSONB NOT NULL DEFAULT '[]',
zones JSONB NOT NULL DEFAULT '[]',
script_type VARCHAR(16) DEFAULT 'script',
mcp_input_schema_toml TEXT DEFAULT '',
mcp_keywords JSONB NOT NULL DEFAULT '[]',
active BOOLEAN NOT NULL DEFAULT TRUE,
discoverable BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS scripts_user_id ON scripts (user_id)`,
	`CREATE INDEX IF NOT EXISTS scripts_is_deleted ON scripts (is_deleted)`,
	`CREATE INDEX IF NOT EXISTS scripts_script_type ON scripts (script_type)`,
	`CREATE INDEX IF NOT EXISTS scripts_name_user ON scripts (name, user_id)`,
	`CREATE TABLE IF NOT EXISTS skills (
skill_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
name VARCHAR(64),
description VARCHAR(1024) DEFAULT '',
content TEXT,
"groups" JSONB NOT NULL DEFAULT '[]',
zones JSONB NOT NULL DEFAULT '[]',
active BOOLEAN NOT NULL DEFAULT TRUE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS skills_user_id ON skills (user_id)`,
	`CREATE INDEX IF NOT EXISTS skills_is_deleted ON skills (is_deleted)`,
	`CREATE INDEX IF NOT EXISTS skills_active ON skills (active)`,
	`CREATE INDEX IF NOT EXISTS skills_name_user ON skills (name, user_id)`,
	`CREATE TABLE IF NOT EXISTS commands (
command_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
name VARCHAR(64),
description VARCHAR(1024) DEFAULT '',
argument_hint VARCHAR(256) DEFAULT '',
allowed_tools JSONB NOT NULL DEFAULT '[]',
body TEXT,
"groups" JSONB NOT NULL DEFAULT '[]',
zones JSONB NOT NULL DEFAULT '[]',
active BOOLEAN NOT NULL DEFAULT TRUE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS commands_user_id ON commands (user_id)`,
	`CREATE INDEX IF NOT EXISTS commands_is_deleted ON commands (is_deleted)`,
	`CREATE INDEX IF NOT EXISTS commands_active ON commands (active)`,
	`CREATE INDEX IF NOT EXISTS commands_name_user ON commands (name, user_id)`,
	`CREATE TABLE IF NOT EXISTS conversations (
conversation_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
title VARCHAR(256) DEFAULT '',
data TEXT NOT NULL,
created_at TIMESTAMPTZ,
updated_at BIGINT DEFAULT 0,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE
)`,
	`CREATE INDEX IF NOT EXISTS conversations_user_id ON conversations (user_id)`,
	`CREATE INDEX IF NOT EXISTS conversations_user_deleted ON conversations (user_id, is_deleted)`,
	`CREATE INDEX IF NOT EXISTS conversations_is_deleted ON conversations (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS roles (
role_id VARCHAR(36) PRIMARY KEY,
name VARCHAR(64),
permissions JSONB DEFAULT NULL,
plugin_permissions JSONB DEFAULT NULL,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS roles_is_deleted ON roles (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS responses (
response_id VARCHAR(36) PRIMARY KEY,
status VARCHAR(32) NOT NULL DEFAULT 'pending',
request JSONB DEFAULT NULL,
response JSONB DEFAULT NULL,
error_text TEXT DEFAULT '',
previous_response_id VARCHAR(36) DEFAULT '',
user_id VARCHAR(36),
space_id VARCHAR(36) DEFAULT '',
expires_at TIMESTAMPTZ DEFAULT NULL,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_at TIMESTAMPTZ,
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS responses_user_id ON responses (user_id)`,
	`CREATE INDEX IF NOT EXISTS responses_status ON responses (status)`,
	`CREATE INDEX IF NOT EXISTS responses_expires_at ON responses (expires_at)`,
	`CREATE INDEX IF NOT EXISTS responses_is_deleted ON responses (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS audit_logs (
audit_log_id BIGSERIAL PRIMARY KEY,
created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
zone VARCHAR(64) DEFAULT '',
actor VARCHAR(255),
actor_type VARCHAR(255),
event VARCHAR(255),
details TEXT,
properties JSONB DEFAULT NULL
)`,
	`CREATE INDEX IF NOT EXISTS audit_logs_actor ON audit_logs (actor, actor_type)`,
	`CREATE INDEX IF NOT EXISTS audit_logs_event ON audit_logs (event)`,
	`CREATE INDEX IF NOT EXISTS audit_logs_created_at ON audit_logs (created_at)`,
	`CREATE TABLE IF NOT EXISTS stack_definitions (
stack_definition_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) NOT NULL DEFAULT '',
name VARCHAR(64) NOT NULL,
description TEXT DEFAULT '',
icon_url VARCHAR(255) NOT NULL DEFAULT '',
"groups" JSONB NOT NULL DEFAULT '[]',
zones JSONB NOT NULL DEFAULT '[]',
active BOOLEAN NOT NULL DEFAULT TRUE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
is_managed BOOLEAN NOT NULL DEFAULT FALSE,
components JSONB NOT NULL DEFAULT '[]',
created_user_id VARCHAR(36) DEFAULT '',
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36) DEFAULT '',
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS stack_definitions_user_id ON stack_definitions (user_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS stack_definitions_user_name ON stack_definitions (user_id, name)`,
	`CREATE INDEX IF NOT EXISTS stack_definitions_active ON stack_definitions (active)`,
	`CREATE INDEX IF NOT EXISTS stack_definitions_is_deleted ON stack_definitions (is_deleted)`,
	`CREATE TABLE IF NOT EXISTS configs (
name VARCHAR(64) PRIMARY KEY,
value TEXT
)`,
	`CREATE TABLE IF NOT EXISTS event_sinks (
event_sink_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
name VARCHAR(64),
description TEXT DEFAULT '',
events JSONB NOT NULL DEFAULT '[]',
sink_type VARCHAR(16) DEFAULT 'webhook',
webhook JSONB DEFAULT NULL,
script_id VARCHAR(36) DEFAULT '',
active BOOLEAN NOT NULL DEFAULT TRUE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36),
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36),
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS event_sinks_user_id ON event_sinks (user_id)`,
	`CREATE INDEX IF NOT EXISTS event_sinks_is_deleted ON event_sinks (is_deleted)`,
	`CREATE INDEX IF NOT EXISTS event_sinks_sink_type ON event_sinks (sink_type)`,
	`CREATE TABLE IF NOT EXISTS user_providers (
provider_id VARCHAR(64) NOT NULL,
provider_uid VARCHAR(255) NOT NULL,
user_id VARCHAR(36) NOT NULL,
refresh_token TEXT,
PRIMARY KEY (provider_id, provider_uid)
)`,
	`CREATE INDEX IF NOT EXISTS user_providers_user_id ON user_providers (user_id)`,
	`CREATE TABLE IF NOT EXISTS mcp_servers (
mcp_server_id VARCHAR(36) PRIMARY KEY,
user_id VARCHAR(36) DEFAULT '',
namespace VARCHAR(128) NOT NULL DEFAULT '',
url TEXT DEFAULT '',
command TEXT DEFAULT '',
args JSONB NOT NULL DEFAULT '[]',
env JSONB NOT NULL DEFAULT '[]',
auth_type VARCHAR(16) DEFAULT '',
token TEXT DEFAULT '',
oauth_client_id VARCHAR(255) DEFAULT '',
oauth_token_url TEXT DEFAULT '',
oauth_access_token TEXT DEFAULT '',
oauth_refresh_token TEXT DEFAULT '',
enabled BOOLEAN NOT NULL DEFAULT TRUE,
tool_visibility VARCHAR(16) NOT NULL DEFAULT 'native',
disabled_tools JSONB NOT NULL DEFAULT '[]',
remote_search BOOLEAN NOT NULL DEFAULT FALSE,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
created_user_id VARCHAR(36) DEFAULT '',
created_at TIMESTAMPTZ,
updated_user_id VARCHAR(36) DEFAULT '',
updated_at BIGINT DEFAULT 0
)`,
	`CREATE INDEX IF NOT EXISTS mcp_servers_user_id ON mcp_servers (user_id)`,
	`CREATE INDEX IF NOT EXISTS mcp_servers_is_deleted ON mcp_servers (is_deleted)`,
	`CREATE INDEX IF NOT EXISTS mcp_servers_enabled ON mcp_servers (enabled)`,
	`CREATE INDEX IF NOT EXISTS mcp_servers_namespace_user ON mcp_servers (namespace, user_id)`,
	`CREATE TABLE IF NOT EXISTS sessions (
session_id VARCHAR(255) PRIMARY KEY,
ip VARCHAR(255) DEFAULT '',
user_id VARCHAR(36) NOT NULL DEFAULT '',
original_user_id VARCHAR(36) NOT NULL DEFAULT '',
user_agent TEXT DEFAULT '',
expires_after TIMESTAMPTZ NOT NULL,
updated_at BIGINT DEFAULT 0,
is_deleted BOOLEAN NOT NULL DEFAULT FALSE
)`,
	`CREATE INDEX IF NOT EXISTS sessions_user_id ON sessions (user_id)`,
	`CREATE INDEX IF NOT EXISTS sessions_expires_after ON sessions (expires_after)`,
}

func (db *PostgresDriver) initialize() error {
	db.logger.Debug("ensuring PostgreSQL schema exists")
	for _, stmt := range schema {
		if _, err := db.connection.Exec(stmt); err != nil {
			return err
		}
	}
	db.logger.Debug("PostgreSQL schema ensured")

	// Run schema migrations for existing databases
	if err := db.runMigrations(); err != nil {
		return err
	}

	// Add a task to clean up expired data
	db.logger.Debug("starting database garbage collection")
	cfg := config.GetServerConfig()
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			db.logger.Debug("running GC")
			now := time.Now().UTC()

			if _, err := db.connection.Exec("DELETE FROM tokens WHERE expires_after < ?", now); err != nil {
				db.logger.WithError(err).Error("failed to delete expired tokens")
			}

			if _, err := db.connection.Exec("DELETE FROM sessions WHERE expires_after < ?", now); err != nil {
				db.logger.WithError(err).Error("failed to delete expired sessions")
			}

			// Remove old audit logs
			if cfg.Audit.Retention > 0 {
				if _, err := db.connection.Exec("DELETE FROM audit_logs WHERE created_at < ?", now.Add(-time.Duration(24*cfg.Audit.Retention)*time.Hour)); err != nil {
					db.logger.WithError(err).Error("failed to delete old audit logs")
				}
			}

			if err := db.cleanupExpiredSpaceUsageSamples(); err != nil {
				db.logger.WithError(err).Error("failed to delete expired space usage samples")
			}
		}
	}()

	return nil
}
