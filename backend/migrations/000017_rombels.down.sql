DROP INDEX IF EXISTS idx_siswa_join_requests_rombel_status;
ALTER TABLE siswa_join_requests
    DROP COLUMN IF EXISTS rombel_id;
ALTER TABLE siswa_join_requests
    ALTER COLUMN kelas_id SET NOT NULL;

DROP TRIGGER IF EXISTS rombel_memberships_set_updated_at ON rombel_memberships;
DROP TABLE IF EXISTS rombel_memberships;

DROP TRIGGER IF EXISTS rombels_set_updated_at ON rombels;
DROP TABLE IF EXISTS rombels;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000016_siswa_self_registration')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
