CREATE TABLE IF NOT EXISTS sekolah (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nama TEXT NOT NULL,
    npsn TEXT UNIQUE,
    alamat TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE sekolah IS 'Admin-managed school master data for grouping kelas.';

ALTER TABLE kelas
    ADD COLUMN IF NOT EXISTS sekolah_id UUID REFERENCES sekolah(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_kelas_sekolah_id
    ON kelas USING btree (sekolah_id);

DROP TRIGGER IF EXISTS sekolah_set_updated_at ON sekolah;
CREATE TRIGGER sekolah_set_updated_at
    BEFORE UPDATE ON sekolah
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000012_sekolah')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
