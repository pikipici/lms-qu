-- LMS — Phase 6 schema rollback. Drop in reverse FK dependency order.

DROP TABLE IF EXISTS event_ujian;
DROP TABLE IF EXISTS jawaban_ujian;

DROP TRIGGER IF EXISTS hasil_ujian_set_updated_at ON hasil_ujian;
DROP TABLE IF EXISTS hasil_ujian;

DROP TABLE IF EXISTS ujian_soal;

DROP TRIGGER IF EXISTS ujian_set_updated_at ON ujian;
DROP TABLE IF EXISTS ujian;

DROP TRIGGER IF EXISTS bank_soal_set_updated_at ON bank_soal;
DROP TABLE IF EXISTS bank_soal;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000010_soalbab')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
