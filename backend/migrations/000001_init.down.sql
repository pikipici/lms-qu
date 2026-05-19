-- Rollback for 000001_init.
--
-- We drop only what we added. Extensions are intentionally NOT dropped —
-- pgcrypto/citext are commonly needed across the database and removing them
-- could affect other roles. If a rollback truly needs to remove them, do it
-- manually with awareness.

DROP TABLE IF EXISTS schema_meta;
