// Attachment upload + presigned download for tugas (Task 4.A.3).
//
// Upload pipeline (locked decisions #46 + #62 + #74):
//   1. Auth + ownership guard via service.findKelasOrForbidden.
//   2. File header sniff via http.DetectContentType — must be in allowlist
//      (pdf, docx, jpg/jpeg, png, zip per locked #46 tugas/submission).
//      Reject 415 unsupported_mime if mismatch.
//   3. Size cap MaxTugasAttachmentBytes = 20MB (locked #74). Reject 413.
//   4. Count cap MaxAttachmentsPerTugas = 5 (locked #74). Reject 400.
//   5. Generate uuid → object_key = "tugas/<uuid>.<ext>".
//   6. R2 PutObject. On fail: 500 r2_put_failed (no DB row yet).
//   7. Insert tugas_attachment row. On fail: compensating R2 DeleteObject
//      + 500 (locked #62 trade-off — bandwidth dobel ok).
//   8. Return {attachment, object_key, original_filename, size_bytes}.
//
// Presigned download (locked #62):
//   GET /tugas/:id/attachments/:attID/url → store.PresignGetDownload(key,
//   ttl, original) with attachment disposition (force download).
//   Audit log file_url_issued.
//
// Delete:
//   DELETE /tugas/:id/attachments/:attID → DB delete + R2 DeleteObject
//   compensating cleanup (locked #69 pattern).
package tugas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/storage"
)

// MaxTugasAttachmentBytes is the hard cap per attachment upload (locked #74).
const MaxTugasAttachmentBytes int64 = 20 * 1024 * 1024

// MaxAttachmentsPerTugas caps the count of attachment per tugas (locked #74).
const MaxAttachmentsPerTugas = 5

// PresignTTL is the default TTL for tugas attachment presigned download URLs
// (locked #62 — 15 minutes default).
const PresignTTL = 15 * time.Minute

// allowedTugasMimes maps the locked #46 allowlist for tugas attachment.
// Keys are mime types returned by http.DetectContentType (sniff first 512
// bytes). Values are the canonical extension stored in object_key.
//
// Note: docx is detected as application/zip by stdlib (DOCX is a ZIP
// container with specific structure). We accept both — the original
// filename extension is preserved separately for UX, and the actual
// payload structure is validated upstream by the client UI.
var allowedTugasMimes = map[string]string{
	"application/pdf":  "pdf",
	"image/jpeg":       "jpg",
	"image/png":        "png",
	"application/zip":  "zip", // covers .zip and .docx
	"application/x-zip-compressed": "zip",
}

// Attachment-specific sentinel errors.
var (
	ErrAttachmentUnsupportedMime = errors.New("tugas: attachment mime not allowed")
	ErrAttachmentTooLarge        = errors.New("tugas: attachment too large")
	ErrAttachmentLimitReached    = errors.New("tugas: attachment limit reached")
	ErrAttachmentUploadFailed    = errors.New("tugas: attachment upload failed")
	ErrR2Required                = errors.New("tugas: object store not configured")
)

// UploadAttachmentInput holds fields for POST /tugas/:id/attachments.
type UploadAttachmentInput struct {
	OriginalFilename string
	Body             []byte
}

// UploadAttachment validates + stores a tugas attachment via R2 + inserts
// a DB row. Owner-only. Mirror materi.Upload trade-off pattern.
func (s *Service) UploadAttachment(ctx context.Context, tugasID, callerID uuid.UUID, callerRole string, in UploadAttachmentInput, ip, userAgent string) (*Attachment, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	if len(in.Body) == 0 {
		return nil, fmt.Errorf("%w: empty file body", ErrInvalidInput)
	}
	if int64(len(in.Body)) > MaxTugasAttachmentBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrAttachmentTooLarge, len(in.Body), MaxTugasAttachmentBytes)
	}

	// Mime sniff via stdlib (#46). Only the first 512 bytes inspected.
	probe := in.Body
	if len(probe) > 512 {
		probe = probe[:512]
	}
	mime := http.DetectContentType(probe)
	mimeStripped := strings.SplitN(mime, ";", 2)[0]
	mimeStripped = strings.TrimSpace(mimeStripped)
	ext, ok := allowedTugasMimes[mimeStripped]
	if !ok {
		return nil, fmt.Errorf("%w: detected %q", ErrAttachmentUnsupportedMime, mime)
	}

	// Lookup tugas + ownership guard.
	t, err := s.repo.FindByID(ctx, tugasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas attachment upload find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	// Count cap (locked #74).
	count, err := s.repo.CountAttachmentsByTugas(ctx, tugasID)
	if err != nil {
		return nil, fmt.Errorf("tugas attachment count: %w", err)
	}
	if int(count)+1 > MaxAttachmentsPerTugas {
		return nil, fmt.Errorf("%w: max %d attachments per tugas", ErrAttachmentLimitReached, MaxAttachmentsPerTugas)
	}

	// Build object key under tugas/<uuid>.<ext>.
	objectID := uuid.New()
	objectKey, err := storage.BuildKey(storage.CategoryTugas, objectID.String()+"."+ext)
	if err != nil {
		return nil, fmt.Errorf("tugas attachment build key: %w", err)
	}

	// R2 PutObject before DB insert — compensating delete on DB failure
	// only when actually needed.
	if perr := s.store.PutObject(ctx, storage.PutObjectInput{
		Key:         objectKey,
		Body:        bytes.NewReader(in.Body),
		Size:        int64(len(in.Body)),
		ContentType: mimeStripped,
	}); perr != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttachmentUploadFailed, perr)
	}

	cleanFilename := sanitizeAttachmentFilename(in.OriginalFilename, ext)
	att := &Attachment{
		TugasID:          tugasID,
		ObjectKey:        objectKey,
		OriginalFilename: cleanFilename,
		MimeType:         mimeStripped,
		SizeBytes:        int64(len(in.Body)),
	}
	if cerr := s.repo.AddAttachment(ctx, att); cerr != nil {
		// Compensating R2 cleanup — DB row never landed.
		if derr := s.store.DeleteObject(context.Background(), objectKey); derr != nil {
			s.logAudit(ctx, "tugas_attachment_orphan", &callerID, callerRole, &tugasID, &t.KelasID, ip, userAgent, map[string]any{
				"object_key": objectKey,
				"reason":     "compensating_delete_failed",
				"err":        derr.Error(),
			})
		}
		return nil, fmt.Errorf("tugas attachment db: %w", cerr)
	}

	s.logAudit(ctx, "tugas_attachment_uploaded", &callerID, callerRole, &tugasID, &t.KelasID, ip, userAgent, map[string]any{
		"tugas_id":          tugasID.String(),
		"attachment_id":     att.ID.String(),
		"object_key":        objectKey,
		"original_filename": cleanFilename,
		"mime_type":         mimeStripped,
		"size_bytes":        att.SizeBytes,
	})
	return att, nil
}

// DeleteAttachment hard-deletes a tugas_attachment row + DeleteObject R2
// compensating cleanup. Owner-only. Audit logged.
func (s *Service) DeleteAttachment(ctx context.Context, tugasID, attachmentID, callerID uuid.UUID, callerRole, ip, userAgent string) error {
	if s.store == nil {
		return ErrR2Required
	}
	t, err := s.repo.FindByID(ctx, tugasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("tugas attachment delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
		return err
	}

	objectKey, err := s.repo.DeleteAttachment(ctx, tugasID, attachmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("tugas attachment delete: %w", err)
	}

	if derr := s.store.DeleteObject(ctx, objectKey); derr != nil {
		// Log orphan but don't fail the user-facing delete (DB row gone).
		s.logAudit(ctx, "tugas_attachment_orphan", &callerID, callerRole, &tugasID, &t.KelasID, ip, userAgent, map[string]any{
			"object_key": objectKey,
			"reason":     "delete_object_failed_after_db_delete",
			"err":        derr.Error(),
		})
	}

	s.logAudit(ctx, "tugas_attachment_deleted", &callerID, callerRole, &tugasID, &t.KelasID, ip, userAgent, map[string]any{
		"tugas_id":      tugasID.String(),
		"attachment_id": attachmentID.String(),
		"object_key":    objectKey,
	})
	return nil
}

// AttachmentURLResult is returned by PresignAttachmentURL.
type AttachmentURLResult struct {
	URL              string
	ExpiresAt        time.Time
	OriginalFilename string
	MimeType         string
}

// PresignAttachmentURL issues a short-lived GET URL for a tugas attachment.
// Authorization branches by role:
//   - guru/admin: must own the kelas (findKelasOrForbidden)
//   - siswa: must be enrolled + tugas.Status == 'published' (siswa can
//     download lampiran soal sebelum submit)
func (s *Service) PresignAttachmentURL(ctx context.Context, tugasID, attachmentID, callerID uuid.UUID, callerRole, ip, userAgent string) (*AttachmentURLResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	t, err := s.repo.FindByID(ctx, tugasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas presign find: %w", err)
	}

	if callerRole == string(auth.Siswa) {
		if t.Status != StatusPublished {
			return nil, ErrNotFound
		}
		if err := s.assertEnrolled(ctx, t.KelasID, callerID); err != nil {
			return nil, err
		}
	} else {
		if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
			return nil, err
		}
	}

	att, err := s.repo.FindAttachmentByID(ctx, tugasID, attachmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas presign attachment find: %w", err)
	}

	url, perr := s.store.PresignGetDownload(ctx, att.ObjectKey, PresignTTL, att.OriginalFilename)
	if perr != nil {
		if errors.Is(perr, storage.ErrObjectNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("tugas presign: %w", perr)
	}

	expiresAt := s.now().Add(PresignTTL)
	s.logAudit(ctx, "tugas_attachment_url_issued", &callerID, callerRole, &tugasID, &t.KelasID, ip, userAgent, map[string]any{
		"tugas_id":      tugasID.String(),
		"attachment_id": attachmentID.String(),
		"object_key":    att.ObjectKey,
		"ttl":           int(PresignTTL.Seconds()),
	})
	return &AttachmentURLResult{
		URL:              url,
		ExpiresAt:        expiresAt,
		OriginalFilename: att.OriginalFilename,
		MimeType:         att.MimeType,
	}, nil
}

// ListAttachments returns the attachments for a tugas (no presigned URLs).
// Authorization mirror PresignAttachmentURL — guru/admin own + siswa
// enrolled + tugas published.
func (s *Service) ListAttachments(ctx context.Context, tugasID, callerID uuid.UUID, callerRole string) ([]Attachment, error) {
	t, err := s.repo.FindByID(ctx, tugasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas attachments list find: %w", err)
	}

	if callerRole == string(auth.Siswa) {
		if t.Status != StatusPublished {
			return nil, ErrNotFound
		}
		if err := s.assertEnrolled(ctx, t.KelasID, callerID); err != nil {
			return nil, err
		}
	} else {
		if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
			return nil, err
		}
	}

	return s.repo.ListAttachmentsByTugas(ctx, tugasID)
}

// sanitizeAttachmentFilename strips path separators and trims to a max
// length. Mirrors materi.sanitizeFilename. Falls back to "tugas.<ext>"
// if input is empty/invalid.
func sanitizeAttachmentFilename(raw, ext string) string {
	s := strings.TrimSpace(raw)
	fallback := "tugas." + ext
	if s == "" {
		return fallback
	}
	s = path.Base(s)
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "\x00", "")
	if s == "" || s == "." || s == ".." {
		return fallback
	}
	const maxLen = 200
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}
