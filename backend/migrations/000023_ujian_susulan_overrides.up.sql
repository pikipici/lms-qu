CREATE TABLE IF NOT EXISTS ujian_access_override (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ujian_id UUID NOT NULL REFERENCES ujian(id) ON DELETE CASCADE,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    waktu_mulai TIMESTAMPTZ,
    waktu_selesai TIMESTAMPTZ NOT NULL,
    durasi_menit SMALLINT CHECK (durasi_menit BETWEEN 1 AND 300),
    max_attempt SMALLINT NOT NULL DEFAULT 1 CHECK (max_attempt BETWEEN 1 AND 999),
    reason TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ujian_id, siswa_id)
);

CREATE INDEX IF NOT EXISTS idx_ujian_access_override_siswa
    ON ujian_access_override USING btree (siswa_id, ujian_id);

DROP TRIGGER IF EXISTS ujian_access_override_set_updated_at ON ujian_access_override;
CREATE TRIGGER ujian_access_override_set_updated_at
    BEFORE UPDATE ON ujian_access_override
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000023_ujian_susulan_overrides')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
