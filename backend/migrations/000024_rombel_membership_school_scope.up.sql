ALTER TABLE rombel_memberships
    ADD COLUMN IF NOT EXISTS sekolah_id UUID REFERENCES sekolah(id) ON DELETE RESTRICT;

UPDATE rombel_memberships rm
SET sekolah_id = r.sekolah_id
FROM rombels r
WHERE r.id = rm.rombel_id
  AND rm.sekolah_id IS NULL;

WITH ranked AS (
    SELECT rm.rombel_id,
           rm.siswa_id,
           ROW_NUMBER() OVER (
               PARTITION BY rm.sekolah_id, rm.siswa_id
               ORDER BY rm.joined_at DESC, rm.updated_at DESC, rm.rombel_id DESC
           ) AS rn
    FROM rombel_memberships rm
    WHERE rm.status = 'active'
      AND rm.sekolah_id IS NOT NULL
)
UPDATE rombel_memberships rm
SET status = 'removed',
    removed_at = COALESCE(rm.removed_at, now()),
    updated_at = now()
FROM ranked r
WHERE r.rombel_id = rm.rombel_id
  AND r.siswa_id = rm.siswa_id
  AND r.rn > 1;

ALTER TABLE rombel_memberships
    ALTER COLUMN sekolah_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_rombel_memberships_one_active_per_school
    ON rombel_memberships (sekolah_id, siswa_id)
    WHERE status = 'active';

CREATE INDEX IF NOT EXISTS idx_rombel_memberships_sekolah_status
    ON rombel_memberships (sekolah_id, status);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000024_rombel_membership_school_scope')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
