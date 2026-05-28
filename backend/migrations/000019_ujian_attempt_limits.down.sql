DROP INDEX IF EXISTS idx_hasil_ujian_unique_berlangsung;
CREATE UNIQUE INDEX IF NOT EXISTS idx_hasil_ujian_unique_active
    ON hasil_ujian USING btree (ujian_id, siswa_id)
    WHERE deleted_at IS NULL;

ALTER TABLE ujian
    DROP CONSTRAINT IF EXISTS ujian_batas_attempt_check;
ALTER TABLE ujian
    DROP COLUMN IF EXISTS attempt_unlimited,
    DROP COLUMN IF EXISTS batas_attempt;

UPDATE schema_meta SET value = '000018_ulangan_unlimited_attempts', set_at = NOW() WHERE key = 'schema_version';
