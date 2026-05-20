-- Reverse 000003: drop kelas/enrollment/import_jobs in reverse dependency order.
-- Trigger first, then indexes, then tables. set_updated_at() function stays
-- (still used by users in 000002).

DROP TRIGGER IF EXISTS kelas_set_updated_at ON kelas;

DROP INDEX IF EXISTS idx_import_jobs_admin_status_expires;
DROP INDEX IF EXISTS idx_enrollment_siswa_id;
DROP INDEX IF EXISTS idx_kelas_guru_id;

DROP TABLE IF EXISTS import_jobs;
DROP TABLE IF EXISTS enrollment;
DROP TABLE IF EXISTS kelas;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000002_auth_schema')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
