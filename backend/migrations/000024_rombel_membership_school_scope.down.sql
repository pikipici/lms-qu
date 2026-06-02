DROP INDEX IF EXISTS idx_rombel_memberships_sekolah_status;
DROP INDEX IF EXISTS idx_rombel_memberships_one_active_per_school;

ALTER TABLE rombel_memberships
    DROP COLUMN IF EXISTS sekolah_id;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000023_ujian_susulan_overrides')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
