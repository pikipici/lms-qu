DROP INDEX IF EXISTS idx_pengumuman_attachment_object_key;

ALTER TABLE pengumuman
  DROP COLUMN IF EXISTS attachment_size,
  DROP COLUMN IF EXISTS attachment_mime,
  DROP COLUMN IF EXISTS attachment_filename,
  DROP COLUMN IF EXISTS attachment_object_key;
