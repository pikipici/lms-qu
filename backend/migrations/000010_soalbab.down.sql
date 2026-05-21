-- LMS — Phase 5 schema rollback. Drop in reverse FK dependency order.

DROP TABLE IF EXISTS soal_assignment;
DROP TABLE IF EXISTS event_bab;
DROP TABLE IF EXISTS jawaban_bab;

DROP TRIGGER IF EXISTS hasil_soal_bab_set_updated_at ON hasil_soal_bab;
DROP TABLE IF EXISTS hasil_soal_bab;

DROP TRIGGER IF EXISTS ulangan_bab_setting_set_updated_at ON ulangan_bab_setting;
DROP TABLE IF EXISTS ulangan_bab_setting;

DROP TRIGGER IF EXISTS soal_bab_set_updated_at ON soal_bab;
DROP TABLE IF EXISTS soal_bab;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000009_submission')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
