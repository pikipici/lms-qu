ALTER TABLE ulangan_bab_setting
    DROP CONSTRAINT IF EXISTS ulangan_bab_setting_batas_attempt_check;
ALTER TABLE ulangan_bab_setting
    ADD CONSTRAINT ulangan_bab_setting_batas_attempt_check
    CHECK (batas_attempt BETWEEN 1 AND 10);

ALTER TABLE ulangan_bab_setting
    DROP COLUMN IF EXISTS attempt_unlimited;

UPDATE schema_meta SET value = '000017_rombels', set_at = NOW() WHERE key = 'schema_version';
