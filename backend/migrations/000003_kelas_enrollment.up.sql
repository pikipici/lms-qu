-- LMS — Phase 2 schema bootstrap (Fase 2).
--
-- Adds kelas, enrollment, and import_jobs tables. No new extensions
-- (pgcrypto + citext already loaded in 000001). The set_updated_at()
-- trigger function is reused from 000002 — only the trigger row on
-- kelas is created here.

CREATE TABLE IF NOT EXISTS kelas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nama TEXT NOT NULL,
    deskripsi TEXT,
    kode_invite TEXT NOT NULL UNIQUE,
    guru_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    bobot_soal_ulangan INTEGER NOT NULL DEFAULT 50,
    bobot_tugas INTEGER NOT NULL DEFAULT 50,
    version INTEGER NOT NULL DEFAULT 1,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE kelas IS 'Class/course owned by a guru. Soft-archived via archived_at; version field guards optimistic concurrency on PATCH.';

CREATE TABLE IF NOT EXISTS enrollment (
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE CASCADE,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'active',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    joined_via TEXT NOT NULL,
    PRIMARY KEY (kelas_id, siswa_id)
);

COMMENT ON TABLE enrollment IS 'Composite-PK siswa membership in a kelas. status active|removed; joined_via admin|kode.';

CREATE TABLE IF NOT EXISTS import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID REFERENCES users(id) ON DELETE SET NULL,
    filename TEXT NOT NULL,
    status TEXT NOT NULL,
    total_rows INTEGER NOT NULL DEFAULT 0,
    valid_count INTEGER NOT NULL DEFAULT 0,
    invalid_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    fail_count INTEGER NOT NULL DEFAULT 0,
    preview_rows_json JSONB,
    errors_json JSONB,
    credentials_csv TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

COMMENT ON TABLE import_jobs IS 'Bulk-import lifecycle: preview -> processing -> completed|expired|failed. Cleanup via hourly cron on (admin_id, status, expires_at).';

CREATE INDEX IF NOT EXISTS idx_kelas_guru_id
    ON kelas USING btree (guru_id);

CREATE INDEX IF NOT EXISTS idx_enrollment_siswa_id
    ON enrollment USING btree (siswa_id);

CREATE INDEX IF NOT EXISTS idx_import_jobs_admin_status_expires
    ON import_jobs USING btree (admin_id, status, expires_at);

DROP TRIGGER IF EXISTS kelas_set_updated_at ON kelas;
CREATE TRIGGER kelas_set_updated_at
    BEFORE UPDATE ON kelas
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000003_kelas_enrollment')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
