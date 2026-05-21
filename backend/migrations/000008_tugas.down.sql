-- LMS — Phase 4 rollback: drop tugas + tugas_attachment tables and indexes.

DROP INDEX IF EXISTS idx_tugas_attachment_tugas;
DROP TABLE IF EXISTS tugas_attachment;

DROP TRIGGER IF EXISTS tugas_set_updated_at ON tugas;
DROP INDEX IF EXISTS idx_tugas_kelas_deadline;
DROP INDEX IF EXISTS idx_tugas_bab_status;
DROP INDEX IF EXISTS idx_tugas_kelas_status_created;
DROP TABLE IF EXISTS tugas;

DELETE FROM schema_meta WHERE key = 'schema_version' AND value = '000008_tugas';
