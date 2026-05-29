ALTER TABLE chat_conversations
    ALTER COLUMN kelas_id DROP NOT NULL,
    ALTER COLUMN guru_id DROP NOT NULL;

DROP INDEX IF EXISTS idx_chat_conversations_admin_unique_active;
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_conversations_admin_unique_active
    ON chat_conversations (sekolah_id, siswa_id)
    WHERE deleted_at IS NULL AND scope = 'admin';

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000022_chat_admin_support')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
