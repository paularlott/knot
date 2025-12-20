/* Migrations from previous version */

/* Add node_id column to spaces table for container affinity */
ALTER TABLE spaces ADD COLUMN node_id VARCHAR(36) DEFAULT '' AFTER zone;
