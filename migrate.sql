/* Migrations from previous version */

/* Add startup_script_id and shutdown_script_id to templates table */
ALTER TABLE templates ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT '';
ALTER TABLE templates ADD COLUMN IF NOT EXISTS shutdown_script_id CHAR(36) DEFAULT '';
