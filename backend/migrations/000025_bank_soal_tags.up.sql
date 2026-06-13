-- Add flexible multi-tags for Bank Soal. Structured fields mapel/tingkat/topik
-- remain for backward compatibility and existing Ujian source configs.

ALTER TABLE bank_soal
    ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_bank_soal_tags_gin
    ON bank_soal USING gin (tags)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_bank_soal_owner_tags
    ON bank_soal USING btree (owner_guru_id)
    WHERE deleted_at IS NULL;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000025_bank_soal_tags')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
