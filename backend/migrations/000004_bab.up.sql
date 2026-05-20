-- LMS — Phase 3 schema bootstrap (Fase 3): Bab.
--
-- Adds the bab table — chapters belonging to a kelas. Status enum
-- (draft|published|archived) is the single source of truth for visibility
-- and lifecycle; archived_at column is intentionally NOT used (Section 6.1
-- decision: gabung jadi 1 enum, tombstone via Status='archived').
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS bab (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE RESTRICT,
    nomor INTEGER NOT NULL,
    judul TEXT NOT NULL,
    deskripsi TEXT NOT NULL DEFAULT '',
    urutan INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE bab IS 'Chapter (bab) within a kelas. Status enum draft|published|archived is the only lifecycle column — siswa only see published. Version field guards optimistic concurrency on PATCH (#56).';

CREATE INDEX IF NOT EXISTS idx_bab_kelas_urutan
    ON bab USING btree (kelas_id, urutan);

CREATE INDEX IF NOT EXISTS idx_bab_kelas_status
    ON bab USING btree (kelas_id, status);

DROP TRIGGER IF EXISTS bab_set_updated_at ON bab;
CREATE TRIGGER bab_set_updated_at
    BEFORE UPDATE ON bab
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000004_bab')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
