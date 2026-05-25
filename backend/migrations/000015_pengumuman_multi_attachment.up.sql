CREATE TABLE IF NOT EXISTS pengumuman_attachment (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  pengumuman_id UUID NOT NULL REFERENCES pengumuman(id) ON DELETE CASCADE,
  object_key TEXT NOT NULL,
  original_filename TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pengumuman_attachment_pengumuman_id
  ON pengumuman_attachment (pengumuman_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_pengumuman_attachment_object_key
  ON pengumuman_attachment (object_key);

INSERT INTO pengumuman_attachment (pengumuman_id, object_key, original_filename, mime_type, size_bytes, created_at)
SELECT id, attachment_object_key, COALESCE(NULLIF(attachment_filename, ''), 'lampiran'), COALESCE(NULLIF(attachment_mime, ''), 'application/octet-stream'), COALESCE(attachment_size, 0), created_at
FROM pengumuman
WHERE attachment_object_key IS NOT NULL AND attachment_object_key <> '';
