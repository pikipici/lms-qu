-- Rollback for 000002_auth_schema.

DROP TABLE IF EXISTS audit_logs, login_attempts, refresh_tokens, users CASCADE;

DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;

DROP FUNCTION IF EXISTS set_updated_at();

-- Roll the schema_version sentinel back to 000001_init (matches 000001 pattern).
INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000001_init')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
