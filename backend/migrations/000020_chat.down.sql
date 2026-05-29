DROP TRIGGER IF EXISTS chat_conversations_set_updated_at ON chat_conversations;
DROP TABLE IF EXISTS chat_messages;
DROP TABLE IF EXISTS chat_conversations;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000019_ujian_attempt_limits')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
