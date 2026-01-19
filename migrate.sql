/* Migrations from previous version */
/* Add startup_script_id and shutdown_script_id to templates table */
ALTER TABLE templates
ADD COLUMN IF NOT EXISTS startup_script_id CHAR(36) DEFAULT '';

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS shutdown_script_id CHAR(36) DEFAULT '';

/* Add user script fields to templates table */
ALTER TABLE templates
ADD COLUMN IF NOT EXISTS user_startup_script VARCHAR(64) DEFAULT '';

ALTER TABLE templates
ADD COLUMN IF NOT EXISTS user_shutdown_script VARCHAR(64) DEFAULT '';

/* Script enhancements - Phase 1-4 */
/* Add user_id, zones, and is_managed to scripts table */
ALTER TABLE scripts
ADD COLUMN IF NOT EXISTS user_id CHAR(36) DEFAULT '';

ALTER TABLE scripts
ADD COLUMN IF NOT EXISTS zones JSON NOT NULL DEFAULT '[]';

ALTER TABLE scripts
ADD COLUMN IF NOT EXISTS is_managed TINYINT (1) NOT NULL DEFAULT 0;

/* Drop old unique constraint on name and add new composite unique constraint */
ALTER TABLE scripts
DROP INDEX IF EXISTS name;

ALTER TABLE scripts ADD UNIQUE INDEX IF NOT EXISTS name_user (name, user_id);

ALTER TABLE scripts ADD INDEX IF NOT EXISTS user_id (user_id);

/* Allow zone-specific script overrides - Phase 5 */
/* Remove unique constraint to allow multiple scripts with same name for different zones */
ALTER TABLE scripts DROP INDEX IF EXISTS name_user;

/* Add regular index for lookups (non-unique to allow zone-specific overrides) */
ALTER TABLE scripts ADD INDEX IF NOT EXISTS name_user (name, user_id);