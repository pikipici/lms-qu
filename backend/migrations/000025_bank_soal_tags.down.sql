DROP INDEX IF EXISTS idx_bank_soal_owner_tags;
DROP INDEX IF EXISTS idx_bank_soal_tags_gin;

ALTER TABLE bank_soal
    DROP COLUMN IF EXISTS tags;

UPDATE schema_meta
SET value = '000024_rombel_membership_school_scope', set_at = NOW()
WHERE key = 'schema_version';
