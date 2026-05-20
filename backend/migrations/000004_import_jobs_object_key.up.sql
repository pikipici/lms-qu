-- 000004 — Add object_key column to import_jobs (Task 2.D.2).
--
-- ImportJob tracks the R2 object that holds the uploaded CSV. This column
-- gets populated at upload time (preview state) and is used both to read
-- the CSV during confirm (Task 2.D.4) and to delete it during cancel/expire
-- cleanup (Task 2.D.6).
--
-- Locked decision #58/#61: object key follows "<kategori>/<uuid>.csv"
-- where kategori is "import". Single bucket per env (lms-dev / lms-prod).
--
-- Idempotent: column add + index. Existing rows get NULL (none in MVP yet
-- because Fase 2.D never shipped before this migration).

ALTER TABLE import_jobs
    ADD COLUMN IF NOT EXISTS object_key TEXT;

COMMENT ON COLUMN import_jobs.object_key IS 'R2 object key for the uploaded CSV, e.g. import/<job_uuid>.csv (locked decision #58/#61). NULL if upload never persisted.';
