-- LMS auth schema bootstrap (Fase 1).
--
-- Creates account, refresh token, login attempt, and audit log tables.
-- Reuses pgcrypto.gen_random_uuid() + citext from 000001_init (no new extensions).

DO $$
BEGIN
    CREATE TYPE user_role AS ENUM ('admin', 'guru', 'siswa');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    CREATE TYPE user_status AS ENUM ('active', 'suspended', 'locked');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email CITEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role user_role NOT NULL,
    status user_status NOT NULL DEFAULT 'active',
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    failed_login_count INTEGER NOT NULL DEFAULT 0,
    last_failed_login_at TIMESTAMPTZ,
    created_by_id UUID REFERENCES users(id) ON DELETE SET NULL,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE users IS 'Application accounts for admins, teachers, and students, including auth state.';

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    jti UUID UNIQUE NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoked_reason TEXT,
    ip INET,
    user_agent TEXT,
    replaced_by_jti UUID
);

COMMENT ON TABLE refresh_tokens IS 'Refresh token tracking for rotation, revocation, and session cleanup.';

CREATE TABLE IF NOT EXISTS login_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email CITEXT NOT NULL,
    ip INET,
    user_agent TEXT,
    success BOOLEAN NOT NULL,
    reason TEXT,
    at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE login_attempts IS 'High-volume login attempt history for rate limiting and security review.';

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL,
    actor_role TEXT,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id UUID,
    target_kelas_id UUID,
    meta JSONB,
    ip INET,
    user_agent TEXT,
    at TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMENT ON TABLE audit_logs IS 'Audit trail for admin, teacher, and system actions.';

CREATE INDEX IF NOT EXISTS idx_users_status
    ON users USING btree (status);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id_revoked_at
    ON refresh_tokens USING btree (user_id, revoked_at);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at
    ON refresh_tokens USING btree (expires_at);

CREATE INDEX IF NOT EXISTS idx_login_attempts_email_at
    ON login_attempts USING btree (email, at DESC);

CREATE INDEX IF NOT EXISTS idx_login_attempts_ip_at
    ON login_attempts USING btree (ip, at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id_at
    ON audit_logs USING btree (actor_id, at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_target_kelas_id_at
    ON audit_logs USING btree (target_kelas_id, at DESC);

DO $$
BEGIN
    CREATE FUNCTION set_updated_at()
    RETURNS trigger
    LANGUAGE plpgsql
    AS $function$
    BEGIN
        NEW.updated_at = now();
        RETURN NEW;
    END;
    $function$;
EXCEPTION
    WHEN duplicate_function THEN NULL;
END $$;

DROP TRIGGER IF EXISTS users_set_updated_at ON users;
CREATE TRIGGER users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000002_auth_schema')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
