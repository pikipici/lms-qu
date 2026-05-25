ALTER TABLE pengumuman
  ADD COLUMN attachment_object_key TEXT,
  ADD COLUMN attachment_filename TEXT,
  ADD COLUMN attachment_mime TEXT,
  ADD COLUMN attachment_size BIGINT;

CREATE INDEX IF NOT EXISTS idx_pengumuman_attachment_object_key
  ON pengumuman (attachment_object_key)
  WHERE attachment_object_key IS NOT NULL;
