-- LMS — Phase 4 schema bootstrap (Fase 4): Tugas + TugasAttachment.
--
-- Adds the tugas table — assignment within a kelas, optionally linked to a
-- bab (BabID nullable: kelas-wide or bab-scoped, locked #20). Status enum
-- (draft|published|archived) drives visibility lifecycle: archived hidden
-- dari siswa list, masih visible ke guru/admin untuk audit transparansi.
--
-- Late submission policy (locked #71): per-tugas IzinkanLate + PenaltyPersen
-- (0-100). Backend reject submission post-deadline kalau IzinkanLate=false
-- (403 deadline_passed). Kalau IzinkanLate=true, accept + flag IsLate=true
-- + grade calc apply penalty (NilaiAsli × (1 - PenaltyPersen/100)).
--
-- WajibAttachment (locked #72): per-tugas guru bisa enforce siswa wajib
-- upload attachment (kalau true, Submit reject 400 attachment_required).
--
-- TugasAttachment (locked #74): lampiran soal/instruksi guru, FK CASCADE ke
-- tugas. R2 path "tugas/<uuid>.<ext>". Allowlist mime via locked #46
-- (pdf, docx, jpg, png, zip), cap 20MB per file, cap 5 file per tugas.
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS tugas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE RESTRICT,
    bab_id UUID REFERENCES bab(id) ON DELETE SET NULL,
    judul TEXT NOT NULL,
    deskripsi TEXT NOT NULL DEFAULT '',
    deadline TIMESTAMPTZ,
    izinkan_late BOOLEAN NOT NULL DEFAULT false,
    penalty_persen SMALLINT NOT NULL DEFAULT 0 CHECK (penalty_persen BETWEEN 0 AND 100),
    wajib_attachment BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    version INTEGER NOT NULL DEFAULT 1,
    created_by_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE tugas IS 'Assignments within a kelas. BabID nullable — bisa kelas-wide (NULL) atau bab-scoped (locked #20). Status enum draft|published|archived. Late policy: IzinkanLate + PenaltyPersen (locked #71). WajibAttachment enforce minimum 1 attachment (locked #72). Version guards optimistic concurrency on PATCH (#56).';

-- Primary list query: guru + siswa scroll per kelas, filter status. Composite
-- supaya planner pilih ini ketimbang scan partial bab index untuk kelas-wide.
CREATE INDEX IF NOT EXISTS idx_tugas_kelas_status_created
    ON tugas USING btree (kelas_id, status, created_at DESC);

-- Bab-scoped filter: di FE bab detail page filter tugas dengan bab_id=<uuid>.
-- Partial index hanya untuk row yang bab_id IS NOT NULL (efficient untuk
-- kelas-wide-heavy distribution).
CREATE INDEX IF NOT EXISTS idx_tugas_bab_status
    ON tugas USING btree (bab_id, status)
    WHERE bab_id IS NOT NULL;

-- Deadline-based query: future feature "due-soon" (deadline within N days)
-- + sort by deadline ASC. NULL deadline (always-open tugas) tidak ke-index
-- (NULLS LAST default — masuk akal untuk due-soon use case).
CREATE INDEX IF NOT EXISTS idx_tugas_kelas_deadline
    ON tugas USING btree (kelas_id, deadline)
    WHERE deadline IS NOT NULL;

DROP TRIGGER IF EXISTS tugas_set_updated_at ON tugas;
CREATE TRIGGER tugas_set_updated_at
    BEFORE UPDATE ON tugas
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS tugas_attachment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tugas_id UUID NOT NULL REFERENCES tugas(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE tugas_attachment IS 'Lampiran soal/instruksi tugas dari guru. FK CASCADE ke tugas — DELETE tugas auto-delete attachment. R2 path "tugas/<uuid>.<ext>" (locked #58/#74). Allowlist mime locked #46 (pdf, docx, jpg, png, zip). Cap size 20MB per file, cap 5 file per tugas (anti-abuse). Compensating R2 cleanup di service.Delete (locked #69 pattern).';

CREATE INDEX IF NOT EXISTS idx_tugas_attachment_tugas
    ON tugas_attachment USING btree (tugas_id);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000008_tugas')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
