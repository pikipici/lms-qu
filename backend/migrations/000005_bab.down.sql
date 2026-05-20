-- LMS — Phase 3 rollback: drop bab table and indexes.

DROP TRIGGER IF EXISTS bab_set_updated_at ON bab;
DROP INDEX IF EXISTS idx_bab_kelas_status;
DROP INDEX IF EXISTS idx_bab_kelas_urutan;
DROP TABLE IF EXISTS bab;

DELETE FROM schema_meta WHERE key = 'schema_version' AND value = '000005_bab';
