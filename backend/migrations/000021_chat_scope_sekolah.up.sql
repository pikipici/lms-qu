ALTER TABLE chat_conversations
    ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'kelas',
    ADD COLUMN IF NOT EXISTS sekolah_id UUID REFERENCES sekolah(id) ON DELETE SET NULL;

ALTER TABLE chat_conversations
    DROP CONSTRAINT IF EXISTS chat_conversations_scope_check;

ALTER TABLE chat_conversations
    ADD CONSTRAINT chat_conversations_scope_check CHECK (scope IN ('kelas', 'admin'));

UPDATE chat_conversations cc
SET sekolah_id = k.sekolah_id
FROM kelas k
WHERE cc.kelas_id = k.id
  AND cc.sekolah_id IS NULL;

DROP INDEX IF EXISTS idx_chat_conversations_unique_active;

CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_conversations_unique_active
    ON chat_conversations (kelas_id, siswa_id)
    WHERE deleted_at IS NULL AND scope = 'kelas';

CREATE INDEX IF NOT EXISTS idx_chat_conversations_scope_sekolah_last
    ON chat_conversations (scope, sekolah_id, last_message_at DESC);

INSERT INTO schema_meta (key, value)
VALUES ('schema_version', '000021_chat_scope_sekolah')
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, set_at = NOW();
