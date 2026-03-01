/* Migrations from previous version */
/* Add script fields to templates table */
ALTER TABLE templates
ADD COLUMN IF NOT EXISTS with_run_command TINYINT (1) NOT NULL DEFAULT 0;

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT '';

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS shutdown_script_id CHAR(36) DEFAULT '';

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS user_startup_script VARCHAR(64) DEFAULT '';

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS user_shutdown_script VARCHAR(64) DEFAULT '';

/* Add startup_script_id to spaces table */
ALTER TABLE spaces
ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT '';