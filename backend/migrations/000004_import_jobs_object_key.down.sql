-- Reverse 000004 — drop object_key column from import_jobs.
ALTER TABLE import_jobs
    DROP COLUMN IF EXISTS object_key;
