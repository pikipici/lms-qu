-- LMS — initial schema bootstrap (Fase 0).
--
-- Fase 0 only creates the bare minimum: extensions and timezone setup.
-- Domain tables (users, kelas, bab, soal, ...) land in Fase 1+ migrations.
--
-- Conventions:
--   * UUIDs via gen_random_uuid() (pgcrypto). We pick UUIDs over bigserial
--     because some tables (RefreshToken JTI, ImportJob, AuditLog) need
--     globally unique identifiers anyway.
--   * Timestamps as TIMESTAMPTZ; the server runs in Asia/Jakarta but
--     storing in UTC keeps comparisons sane (#29).
--   * Use IF NOT EXISTS so re-runs are safe in dev (AUTOMIGRATE flow).

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- Sentinel table so migrations can be verified without touching domain rows.
-- Removed in 000002_users when the real users table arrives, OR kept as a
-- "schema metadata" table — decide at Fase 1 implementation.
CREATE TABLE IF NOT EXISTS schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    set_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000001_init')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
