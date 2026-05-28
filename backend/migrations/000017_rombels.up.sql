CREATE TABLE IF NOT EXISTS rombels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sekolah_id UUID NOT NULL REFERENCES sekolah(id) ON DELETE RESTRICT,
    nama TEXT NOT NULL,
    deskripsi TEXT NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT true,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rombels_unique_active_name
    ON rombels (sekolah_id, lower(nama))
    WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_rombels_sekolah_active
    ON rombels (sekolah_id, archived_at);

DROP TRIGGER IF EXISTS rombels_set_updated_at ON rombels;
CREATE TRIGGER rombels_set_updated_at
    BEFORE UPDATE ON rombels
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS rombel_memberships (
    rombel_id UUID NOT NULL REFERENCES rombels(id) ON DELETE RESTRICT,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'active',
    joined_via TEXT NOT NULL DEFAULT 'self_registration',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (rombel_id, siswa_id),
    CHECK (status IN ('active', 'removed')),
    CHECK (joined_via IN ('self_registration', 'admin'))
);

CREATE INDEX IF NOT EXISTS idx_rombel_memberships_siswa_status
    ON rombel_memberships (siswa_id, status);
CREATE INDEX IF NOT EXISTS idx_rombel_memberships_rombel_status
    ON rombel_memberships (rombel_id, status);

DROP TRIGGER IF EXISTS rombel_memberships_set_updated_at ON rombel_memberships;
CREATE TRIGGER rombel_memberships_set_updated_at
    BEFORE UPDATE ON rombel_memberships
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

ALTER TABLE siswa_join_requests
    ADD COLUMN IF NOT EXISTS rombel_id UUID REFERENCES rombels(id) ON DELETE RESTRICT;
ALTER TABLE siswa_join_requests
    ALTER COLUMN kelas_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_siswa_join_requests_rombel_status
    ON siswa_join_requests (rombel_id, status);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000017_rombels')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
