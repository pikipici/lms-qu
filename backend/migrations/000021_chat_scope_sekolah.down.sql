DROP INDEX IF EXISTS idx_chat_conversations_scope_sekolah_last;

DROP INDEX IF EXISTS idx_chat_conversations_unique_active;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_conversations_unique_active
    ON chat_conversations (kelas_id, siswa_id)
    WHERE deleted_at IS NULL;

ALTER TABLE chat_conversations
    DROP CONSTRAINT IF EXISTS chat_conversations_scope_check;

ALTER TABLE chat_conversations
    DROP COLUMN IF EXISTS sekolah_id,
    DROP COLUMN IF EXISTS scope;

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000020_chat')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
