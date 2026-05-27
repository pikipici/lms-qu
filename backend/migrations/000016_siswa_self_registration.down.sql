DROP TRIGGER IF EXISTS siswa_join_requests_set_updated_at ON siswa_join_requests;
DROP TABLE IF EXISTS siswa_join_requests;

ALTER TABLE sekolah
    DROP CONSTRAINT IF EXISTS sekolah_siswa_registration_mode_check;
ALTER TABLE sekolah
    DROP COLUMN IF EXISTS siswa_registration_mode,
    DROP COLUMN IF EXISTS siswa_registration_enabled;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000015_pengumuman_multi_attachment')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
