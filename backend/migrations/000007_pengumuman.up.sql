-- LMS — Phase 3 schema bootstrap (Fase 3): Pengumuman.
--
-- Adds the pengumuman table — announcements within a kelas, optionally
-- linked to a bab (BabID nullable: kelas-wide or bab-scoped).
--
-- Status enum (published|archived) — locked #66 passive timestamp model:
-- pengumuman tidak punya read receipt per siswa di MVP, frontend pakai
-- created_at vs last_seen client-side untuk badge "Baru". Status drives
-- visibility: 'archived' hidden dari siswa list, masih visible ke guru
-- pemilik untuk audit.
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS pengumuman (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE RESTRICT,
    bab_id UUID REFERENCES bab(id) ON DELETE SET NULL,
    judul TEXT NOT NULL,
    isi TEXT NOT NULL DEFAULT '',
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published', 'archived')),
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE pengumuman IS 'Announcements within a kelas. BabID nullable — bisa kelas-wide (NULL) atau bab-scoped. Status enum published|archived (#66 passive timestamp, no per-siswa read receipt). Version guards optimistic concurrency on PATCH (#56).';

-- Primary list query: siswa/guru scroll newest-first per kelas. Index
-- supports filter by status as well (siswa filter status=published).
CREATE INDEX IF NOT EXISTS idx_pengumuman_kelas_created
    ON pengumuman USING btree (kelas_id, created_at DESC);

-- Bab-scoped filter: di FE, bab detail page filter pengumuman dengan
-- bab_id=<uuid>. Partial index hanya untuk row yang bab_id IS NOT NULL.
CREATE INDEX IF NOT EXISTS idx_pengumuman_bab_created
    ON pengumuman USING btree (bab_id, created_at DESC)
    WHERE bab_id IS NOT NULL;

-- Status pinning: siswa list always status='published'. Composite with
-- kelas_id supaya planner bisa pilih ini ketimbang scan partial index.
CREATE INDEX IF NOT EXISTS idx_pengumuman_kelas_status_created
    ON pengumuman USING btree (kelas_id, status, created_at DESC);

DROP TRIGGER IF EXISTS pengumuman_set_updated_at ON pengumuman;
CREATE TRIGGER pengumuman_set_updated_at
    BEFORE UPDATE ON pengumuman
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000007_pengumuman')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
