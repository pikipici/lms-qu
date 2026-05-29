CREATE TABLE IF NOT EXISTS chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kelas_id UUID NOT NULL REFERENCES kelas(id) ON DELETE CASCADE,
    siswa_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    guru_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'open',
    last_message_at TIMESTAMPTZ,
    last_message_preview TEXT NOT NULL DEFAULT '',
    siswa_unread_count INTEGER NOT NULL DEFAULT 0,
    guru_unread_count INTEGER NOT NULL DEFAULT 0,
    admin_unread_count INTEGER NOT NULL DEFAULT 0,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chat_conversations_status_check CHECK (status IN ('open', 'closed'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_conversations_unique_active
    ON chat_conversations (kelas_id, siswa_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_chat_conversations_guru_status_last
    ON chat_conversations (guru_id, status, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_kelas_last
    ON chat_conversations (kelas_id, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_siswa_last
    ON chat_conversations (siswa_id, last_message_at DESC);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_deleted_at
    ON chat_conversations (deleted_at);

CREATE TABLE IF NOT EXISTS chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES chat_conversations(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    sender_role TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT chat_messages_sender_role_check CHECK (sender_role IN ('siswa', 'guru', 'admin')),
    CONSTRAINT chat_messages_body_len_check CHECK (char_length(body) BETWEEN 1 AND 4000)
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation_created
    ON chat_messages (conversation_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_chat_messages_deleted_at
    ON chat_messages (deleted_at);

DROP TRIGGER IF EXISTS chat_conversations_set_updated_at ON chat_conversations;
CREATE TRIGGER chat_conversations_set_updated_at
    BEFORE UPDATE ON chat_conversations
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000020_chat')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
