/* Migrations from previous version */

ALTER TABLE templates ADD COLUMN with_run_command TINYINT(1) NOT NULL DEFAULT 0 AFTER with_ssh;
