ALTER TABLE ujian
    ADD COLUMN IF NOT EXISTS batas_attempt SMALLINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS attempt_unlimited BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE ujian
    DROP CONSTRAINT IF EXISTS ujian_batas_attempt_check;
ALTER TABLE ujian
    ADD CONSTRAINT ujian_batas_attempt_check CHECK (batas_attempt BETWEEN 1 AND 999);

DROP INDEX IF EXISTS idx_hasil_ujian_unique_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_hasil_ujian_unique_berlangsung
    ON hasil_ujian USING btree (ujian_id, siswa_id)
    WHERE status = 'berlangsung' AND deleted_at IS NULL;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000019_ujian_attempt_limits')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
