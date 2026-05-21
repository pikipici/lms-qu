-- LMS — Phase 4 rollback: drop submission + submission_attachment.

DROP INDEX IF EXISTS idx_submission_attachment_submission;
DROP TABLE IF EXISTS submission_attachment;

DROP TRIGGER IF EXISTS submission_set_updated_at ON submission;
DROP INDEX IF EXISTS idx_submission_graded_by_at;
DROP INDEX IF EXISTS idx_submission_siswa_submitted;
DROP INDEX IF EXISTS idx_submission_tugas_status;
DROP TABLE IF EXISTS submission;

DELETE FROM schema_meta WHERE key = 'schema_version' AND value = '000009_submission';
