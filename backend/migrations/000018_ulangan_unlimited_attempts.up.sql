ALTER TABLE ulangan_bab_setting
    ADD COLUMN IF NOT EXISTS attempt_unlimited BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE ulangan_bab_setting
    DROP CONSTRAINT IF EXISTS ulangan_bab_setting_batas_attempt_check;
ALTER TABLE ulangan_bab_setting
    ADD CONSTRAINT ulangan_bab_setting_batas_attempt_check
    CHECK (batas_attempt BETWEEN 1 AND 999);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000018_ulangan_unlimited_attempts')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
