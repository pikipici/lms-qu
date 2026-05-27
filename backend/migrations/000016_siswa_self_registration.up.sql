ALTER TABLE sekolah
    ADD COLUMN IF NOT EXISTS siswa_registration_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS siswa_registration_mode TEXT NOT NULL DEFAULT 'approval_required';

ALTER TABLE sekolah
    DROP CONSTRAINT IF EXISTS sekolah_siswa_registration_mode_check;
ALTER TABLE sekolah
    ADD CONSTRAINT sekolah_siswa_registration_mode_check
    CHECK (siswa_registration_mode IN ('auto_approve', 'approval_required'));

CREATE TABLE IF NOT EXISTS siswa_join_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sekolah_id UUID NOT NULL REFERENCES sekolah(id) ON DELETE CASCADE,
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    decided_by UUID REFERENCES users(id) ON DELETE SET NULL,
    reject_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (siswa_id, kelas_id),
    CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled'))
);

CREATE INDEX IF NOT EXISTS idx_siswa_join_requests_status_requested
    ON siswa_join_requests (status, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_siswa_join_requests_kelas_status
    ON siswa_join_requests (kelas_id, status);
CREATE INDEX IF NOT EXISTS idx_siswa_join_requests_sekolah_status
    ON siswa_join_requests (sekolah_id, status);

DROP TRIGGER IF EXISTS siswa_join_requests_set_updated_at ON siswa_join_requests;
CREATE TRIGGER siswa_join_requests_set_updated_at
    BEFORE UPDATE ON siswa_join_requests
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000016_siswa_self_registration')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
