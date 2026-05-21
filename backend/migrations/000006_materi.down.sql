-- LMS — Phase 3 rollback: drop materi + materi_read tables and indexes.

DROP INDEX IF EXISTS idx_materi_read_siswa;
DROP TABLE IF EXISTS materi_read;

DROP TRIGGER IF EXISTS materi_set_updated_at ON materi;
DROP INDEX IF EXISTS idx_materi_bab_urutan;
DROP INDEX IF EXISTS idx_materi_kelas_tipe;
DROP INDEX IF EXISTS idx_materi_kelas_bab_urutan;
DROP TABLE IF EXISTS materi;

DELETE FROM schema_meta WHERE key = 'schema_version' AND value = '000006_materi';
