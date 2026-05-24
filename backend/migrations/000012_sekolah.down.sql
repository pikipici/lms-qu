ALTER TABLE kelas DROP COLUMN IF EXISTS sekolah_id;
DROP TRIGGER IF EXISTS sekolah_set_updated_at ON sekolah;
DROP TABLE IF EXISTS sekolah;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000011_ujian')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
