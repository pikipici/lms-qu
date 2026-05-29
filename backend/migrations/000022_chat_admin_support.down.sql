DROP INDEX IF EXISTS idx_chat_conversations_admin_unique_active;

DELETE FROM chat_messages
WHERE conversation_id IN (
    SELECT id FROM chat_conversations WHERE scope = 'admin'
);

DELETE FROM chat_conversations WHERE scope = 'admin';

ALTER TABLE chat_conversations
    ALTER COLUMN kelas_id SET NOT NULL,
    ALTER COLUMN guru_id SET NOT NULL;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000021_chat_scope_sekolah')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
