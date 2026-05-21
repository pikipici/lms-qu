-- LMS — Phase 3 schema bootstrap (Fase 3): Materi + MateriRead.
--
-- Adds the materi table — learning content units within a kelas, optionally
-- linked to a bab. Tipe enum (pdf|youtube|markdown) is locked to 3 modes
-- (decision #63 — drop direct video upload, YouTube embed cukup). For tipe
-- 'pdf': use object_key/original_filename/mime_type/size_bytes (R2 path
-- 'materi/<uuid>.pdf', max 20MB locked #64). For 'youtube': simpan video_id
-- 11-char di konten (parsed via parseYouTubeID locked #65). For 'markdown':
-- simpan body markdown di konten (max 50KB enforce di handler).
--
-- materi_read tracks per-siswa read state untuk progress calc Fase-3-partial
-- (locked #68 — materi_dibaca / total_materi). Composite PK + ON CONFLICT
-- DO NOTHING bikin mark-read idempotent.
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS materi (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE RESTRICT,
    bab_id UUID REFERENCES bab(id) ON DELETE SET NULL,
    judul TEXT NOT NULL,
    tipe TEXT NOT NULL CHECK (tipe IN ('pdf', 'youtube', 'markdown')),
    konten TEXT NOT NULL DEFAULT '',
    object_key TEXT,
    original_filename TEXT,
    mime_type TEXT,
    size_bytes BIGINT,
    urutan INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- pdf MUST have object_key + original_filename + mime_type + size_bytes;
    -- youtube/markdown MUST NOT have any of those.
    CONSTRAINT materi_tipe_payload_chk CHECK (
        (tipe = 'pdf' AND object_key IS NOT NULL AND original_filename IS NOT NULL
            AND mime_type IS NOT NULL AND size_bytes IS NOT NULL)
        OR (tipe IN ('youtube', 'markdown') AND object_key IS NULL AND original_filename IS NULL
            AND mime_type IS NULL AND size_bytes IS NULL)
    )
);

COMMENT ON TABLE materi IS 'Learning content within a kelas. Tipe enum pdf|youtube|markdown locked #63. PDF: R2 object via object_key/original_filename/mime_type/size_bytes (max 20MB locked #64). YouTube: video_id 11-char di konten (locked #65). Markdown: body inline di konten. Version field guards optimistic concurrency on PATCH (#56). Hard delete + R2 cleanup compensating (locked #69).';

CREATE INDEX IF NOT EXISTS idx_materi_kelas_bab_urutan
    ON materi USING btree (kelas_id, bab_id, urutan);

CREATE INDEX IF NOT EXISTS idx_materi_kelas_tipe
    ON materi USING btree (kelas_id, tipe);

CREATE INDEX IF NOT EXISTS idx_materi_bab_urutan
    ON materi USING btree (bab_id, urutan)
    WHERE bab_id IS NOT NULL;

DROP TRIGGER IF EXISTS materi_set_updated_at ON materi;
CREATE TRIGGER materi_set_updated_at
    BEFORE UPDATE ON materi
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS materi_read (
    materi_id UUID NOT NULL REFERENCES materi(id) ON DELETE CASCADE,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (materi_id, siswa_id)
);

COMMENT ON TABLE materi_read IS 'Per-siswa mark-as-read state untuk materi. Idempotent via composite PK + ON CONFLICT DO NOTHING. Dipakai untuk progress per bab Fase-3-partial (locked #68).';

CREATE INDEX IF NOT EXISTS idx_materi_read_siswa
    ON materi_read USING btree (siswa_id);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000006_materi')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
