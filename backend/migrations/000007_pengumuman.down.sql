-- LMS — Phase 3 rollback: drop pengumuman table and indexes.

DROP TRIGGER IF EXISTS pengumuman_set_updated_at ON pengumuman;
DROP INDEX IF EXISTS idx_pengumuman_kelas_status_created;
DROP INDEX IF EXISTS idx_pengumuman_bab_created;
DROP INDEX IF EXISTS idx_pengumuman_kelas_created;
DROP TABLE IF EXISTS pengumuman;

DELETE FROM schema_meta WHERE key = 'schema_version' AND value = '000007_pengumuman';
