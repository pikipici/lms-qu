DROP TRIGGER IF EXISTS ujian_access_override_set_updated_at ON ujian_access_override;
DROP TABLE IF EXISTS ujian_access_override;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000022_chat_admin_support')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
