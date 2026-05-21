-- LMS — Phase 4 schema bootstrap (Fase 4): Submission + SubmissionAttachment.
--
-- Submission represents a siswa's response to a Tugas. One row per
-- (TugasID, SiswaID) — UNIQUE constraint enforces single-row + version
-- bump strategy on resubmit (locked #70). Resubmit overwrite konten +
-- attachment set + bump Version (locked #56 pattern). History per-attempt
-- out-of-scope MVP — audit trail di AuditLog cukup.
--
-- Status enum (submitted|graded|returned) drives review lifecycle:
--   submitted = siswa submit, awaiting grade
--   graded    = guru kasih nilai (final, MVP — kalau salah, hapus + siswa resubmit)
--   returned  = guru return for revision (defer MVP, defined for forward-compat)
-- (locked #73 — draft excluded MVP karena resubmit overwrite ya resubmit; no draft tier.)
--
-- Late submission policy (locked #71):
--   IsLate set saat submit kalau now > tugas.Deadline AND tugas.IzinkanLate=true.
--   PenaltyPersenApplied di-snapshot saat grade (= tugas.PenaltyPersen kalau is_late, else 0).
--   NilaiSetelahPenalty = round(NilaiAsli × (1 - PenaltyPersenApplied/100), 2).
--
-- Attachment policy (locked #72):
--   0..N attachment per submission. WajibAttachment di tugas (locked #74) bisa enforce
--   minimum 1. Cap 5 file × 20MB. R2 path "submission/<uuid>.<ext>".
--
-- Concurrency (locked #73):
--   Submit endpoint pakai SELECT ... FOR UPDATE pada existing row (kalau ada) untuk
--   serialize resubmit + grade. Grade endpoint juga FOR UPDATE + cek Status='submitted'.
--
-- Reuses set_updated_at() trigger function from 000002.

CREATE TABLE IF NOT EXISTS submission (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tugas_id UUID NOT NULL REFERENCES tugas(id) ON DELETE CASCADE,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    catatan TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'submitted' CHECK (status IN ('submitted', 'graded', 'returned')),
    is_late BOOLEAN NOT NULL DEFAULT false,
    nilai_asli NUMERIC(5, 2),
    penalty_persen_applied SMALLINT,
    nilai_setelah_penalty NUMERIC(5, 2),
    feedback TEXT NOT NULL DEFAULT '',
    graded_by_id UUID REFERENCES users(id) ON DELETE SET NULL,
    graded_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tugas_id, siswa_id)
);

COMMENT ON TABLE submission IS 'Siswa response to a Tugas. Single-row per (tugas_id, siswa_id) with version bump on resubmit (locked #70). Status enum submitted|graded|returned drives review lifecycle (locked #73). IsLate flag set saat submit kalau lewat deadline + IzinkanLate=true (locked #71). PenaltyPersenApplied snapshot saat grade. Version guards optimistic concurrency on grade (locked #56).';

-- Primary list query: guru rekap per tugas, filter status (submitted vs graded vs returned).
-- Composite supaya planner bisa langsung pilih status filter tanpa scan ulang.
CREATE INDEX IF NOT EXISTS idx_submission_tugas_status
    ON submission USING btree (tugas_id, status);

-- History per siswa, sorted submitted_at DESC. Gunakan misal di FE siswa list
-- "submission saya di kelas X".
CREATE INDEX IF NOT EXISTS idx_submission_siswa_submitted
    ON submission USING btree (siswa_id, submitted_at DESC);

-- Audit guru grading: list semua submission yang di-grade by guru tertentu,
-- sorted by graded_at DESC. Partial index (graded_by_id IS NOT NULL) hemat
-- space untuk submission yang belum graded.
CREATE INDEX IF NOT EXISTS idx_submission_graded_by_at
    ON submission USING btree (graded_by_id, graded_at DESC)
    WHERE graded_by_id IS NOT NULL;

DROP TRIGGER IF EXISTS submission_set_updated_at ON submission;
CREATE TRIGGER submission_set_updated_at
    BEFORE UPDATE ON submission
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

CREATE TABLE IF NOT EXISTS submission_attachment (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    submission_id UUID NOT NULL REFERENCES submission(id) ON DELETE CASCADE,
    object_key TEXT NOT NULL,
    original_filename TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE submission_attachment IS 'File attached to a submission (siswa upload). FK CASCADE ke submission — DELETE submission auto-delete attachment, tapi caller harus DeleteObject ke R2 (locked #69 pattern). R2 path "submission/<uuid>.<ext>" (locked #58/#72). Allowlist mime locked #46 (pdf, docx, jpg, png, zip). Cap size 20MB per file, cap 5 file per submission. Resubmit replace seluruh attachment set dalam tx (locked #72).';

CREATE INDEX IF NOT EXISTS idx_submission_attachment_submission
    ON submission_attachment USING btree (submission_id);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000009_submission')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
