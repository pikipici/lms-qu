// Submit + Get/List + Grade business logic untuk submission domain.
//
// File ini complementary ke service.go (helpers + wiring). Split supaya
// test file gampang fokus per-flow.
package submission

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
	"github.com/pikip/lms/backend/internal/tugas"
)

// allowedMimes mirrors the locked #46 allowlist (submission category).
// Same as tugas attachment — pdf, docx (zip container), jpg, png, zip.
var allowedMimes = map[string]string{
	"application/pdf":               "pdf",
	"image/jpeg":                    "jpg",
	"image/png":                     "png",
	"application/zip":               "zip", // covers .zip and .docx
	"application/x-zip-compressed":  "zip",
}

// AttachmentInput holds a single file from the multipart form.
type AttachmentInput struct {
	OriginalFilename string
	Body             []byte
}

// SubmitInput holds POST /siswa/tugas/:id/submit body.
type SubmitInput struct {
	Catatan     string
	Attachments []AttachmentInput
}

// SubmitResult bundles the submission row + new attachment rows.
type SubmitResult struct {
	Submission *Submission
	IsResubmit bool
}

// Submit handles siswa submit/resubmit (locked #70/#71/#72/#73).
//
// Tx flow (locked #73):
//  1. Validate input (catatan size, attachment count/size cap).
//  2. Pre-sniff mime + reject early.
//  3. Find tugas, verify status='published', enrollment active.
//  4. Late detection: now > Deadline. Kalau IzinkanLate=false → reject 403.
//  5. Pre-upload R2 putList (collect new ObjectKeys) — kalau salah satu fail,
//     compensating delete semua yang udah berhasil.
//  6. BEGIN tx → LockForUpdate (ada row → resubmit; not found → create new).
//  7. Validate WajibAttachment + status guard (kalau graded → 409).
//  8. Resubmit path: collect old ObjectKeys → DeleteAttachmentsBySubmission
//     (DB) → UpdateOnResubmit (catatan/is_late/version/submitted_at).
//     New row path: Create.
//  9. Insert new attachments (DB).
// 10. Audit log + COMMIT.
// 11. Post-commit: defer compensating R2 delete untuk old object keys (kalau
//     resubmit) — best-effort, log orphan kalau gagal.
// 12. On any tx error: compensating delete semua R2 putList (rollback).
func (s *Service) Submit(ctx context.Context, tugasID, siswaID uuid.UUID, in SubmitInput, ip, userAgent string) (*SubmitResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	// 1. Validate catatan size + attachment count.
	if len(in.Catatan) > MaxCatatanBytes {
		return nil, fmt.Errorf("%w: catatan exceeds %d bytes", ErrInvalidInput, MaxCatatanBytes)
	}
	if len(in.Attachments) > MaxAttachmentsPerSubmission {
		return nil, fmt.Errorf("%w: max %d attachments per submission", ErrAttachmentLimit, MaxAttachmentsPerSubmission)
	}
	// 2. Pre-sniff each attachment + size cap.
	type stagedFile struct {
		ObjectKey        string
		OriginalFilename string
		MimeType         string
		Body             []byte
		SizeBytes        int64
	}
	staged := make([]stagedFile, 0, len(in.Attachments))
	for i, att := range in.Attachments {
		if len(att.Body) == 0 {
			return nil, fmt.Errorf("%w: attachment %d empty", ErrInvalidInput, i+1)
		}
		if int64(len(att.Body)) > MaxAttachmentBytes {
			return nil, fmt.Errorf("%w: attachment %d exceeds %d bytes", ErrAttachmentTooLarge, i+1, MaxAttachmentBytes)
		}
		probe := att.Body
		if len(probe) > 512 {
			probe = probe[:512]
		}
		mime := http.DetectContentType(probe)
		mimeStripped := strings.SplitN(mime, ";", 2)[0]
		mimeStripped = strings.TrimSpace(mimeStripped)
		ext, ok := allowedMimes[mimeStripped]
		if !ok {
			return nil, fmt.Errorf("%w: attachment %d detected %q", ErrUnsupportedMime, i+1, mime)
		}
		objectID := uuid.New()
		objectKey, err := storage.BuildKey(storage.CategorySubmission, objectID.String()+"."+ext)
		if err != nil {
			return nil, fmt.Errorf("submission build key: %w", err)
		}
		staged = append(staged, stagedFile{
			ObjectKey:        objectKey,
			OriginalFilename: sanitizeAttachmentFilename(att.OriginalFilename, ext),
			MimeType:         mimeStripped,
			Body:             att.Body,
			SizeBytes:        int64(len(att.Body)),
		})
	}

	// 3. Find tugas + enrollment guard.
	t, err := s.findTugasOrNotFound(ctx, tugasID)
	if err != nil {
		return nil, err
	}
	if t.Status != tugas.StatusPublished {
		return nil, ErrNotFound
	}
	if err := s.assertEnrolled(ctx, t.KelasID, siswaID); err != nil {
		return nil, err
	}

	// 4. Deadline + late check.
	now := s.now()
	isLate := false
	if t.Deadline != nil && now.After(*t.Deadline) {
		if !t.IzinkanLate {
			return nil, ErrDeadlinePassed
		}
		isLate = true
	}

	// 5. WajibAttachment guard.
	if t.WajibAttachment && len(staged) == 0 {
		return nil, ErrAttachmentRequired
	}

	// 6. R2 PutObject for staged files. Track succeeded keys for compensating.
	uploadedKeys := make([]string, 0, len(staged))
	rollbackR2 := func() {
		for _, k := range uploadedKeys {
			_ = s.store.DeleteObject(context.Background(), k)
		}
	}
	for _, f := range staged {
		if perr := s.store.PutObject(ctx, storage.PutObjectInput{
			Key:         f.ObjectKey,
			Body:        bytes.NewReader(f.Body),
			Size:        f.SizeBytes,
			ContentType: f.MimeType,
		}); perr != nil {
			rollbackR2()
			return nil, fmt.Errorf("%w: %v", ErrAttachmentUploadFailed, perr)
		}
		uploadedKeys = append(uploadedKeys, f.ObjectKey)
	}

	// 7. BEGIN tx — lock submission row, decide create vs resubmit.
	var (
		result          SubmitResult
		oldObjectKeys   []string
		auditAction     string
		auditMeta       map[string]any
	)
	txErr := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := s.repo.LockForUpdate(ctx, tx, tugasID, siswaID)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			// First submit — Create new row.
			sub := &Submission{
				ID:          uuid.New(),
				TugasID:     tugasID,
				SiswaID:     siswaID,
				Catatan:     in.Catatan,
				Status:      StatusSubmitted,
				IsLate:      isLate,
				Version:     1,
				SubmittedAt: now,
			}
			if err := s.repo.Create(ctx, tx, sub); err != nil {
				return fmt.Errorf("submission create: %w", err)
			}
			result.Submission = sub
			result.IsResubmit = false
			auditAction = "submission_submitted"
		case err != nil:
			return fmt.Errorf("submission lock: %w", err)
		default:
			// Existing row — resubmit path.
			if existing.Status == StatusGraded {
				return ErrAlreadyGraded
			}
			oldKeys, derr := s.repo.DeleteAttachmentsBySubmission(ctx, tx, existing.ID)
			if derr != nil {
				return fmt.Errorf("submission delete old attachments: %w", derr)
			}
			oldObjectKeys = oldKeys
			if uerr := s.repo.UpdateOnResubmit(ctx, tx, existing.ID, existing.Version, in.Catatan, isLate); uerr != nil {
				return fmt.Errorf("submission resubmit update: %w", uerr)
			}
			// Re-fetch latest values for response (version/submitted_at bumped).
			refreshed, ferr := s.repo.LockByID(ctx, tx, existing.ID)
			if ferr != nil {
				return fmt.Errorf("submission resubmit refetch: %w", ferr)
			}
			result.Submission = refreshed
			result.IsResubmit = true
			auditAction = "submission_resubmitted"
		}

		// 8. Insert attachments under the same tx.
		for _, f := range staged {
			att := &Attachment{
				SubmissionID:     result.Submission.ID,
				ObjectKey:        f.ObjectKey,
				OriginalFilename: f.OriginalFilename,
				MimeType:         f.MimeType,
				SizeBytes:        f.SizeBytes,
			}
			if aerr := s.repo.AddAttachment(ctx, tx, att); aerr != nil {
				return fmt.Errorf("submission add attachment: %w", aerr)
			}
		}

		auditMeta = map[string]any{
			"tugas_id":         tugasID.String(),
			"submission_id":    result.Submission.ID.String(),
			"is_late":          isLate,
			"attachment_count": len(staged),
			"version":          result.Submission.Version,
		}
		if len(oldObjectKeys) > 0 {
			auditMeta["old_object_keys"] = oldObjectKeys
		}
		return nil
	})
	if txErr != nil {
		// Rollback all staged R2 puts (DB rolled back automatically by tx).
		rollbackR2()
		return nil, txErr
	}

	// 9. Post-commit: best-effort R2 cleanup of old keys (resubmit case).
	for _, k := range oldObjectKeys {
		if derr := s.store.DeleteObject(context.Background(), k); derr != nil {
			s.logAudit(ctx, "submission_attachment_orphan", &siswaID, string(auth.Siswa), &result.Submission.ID, &t.KelasID, ip, userAgent, map[string]any{
				"object_key": k,
				"reason":     "delete_object_failed_post_resubmit",
				"err":        derr.Error(),
			})
		}
	}

	// 10. Audit submitted/resubmitted + late flag.
	s.logAudit(ctx, auditAction, &siswaID, string(auth.Siswa), &result.Submission.ID, &t.KelasID, ip, userAgent, auditMeta)
	if isLate {
		s.logAudit(ctx, "submission_late", &siswaID, string(auth.Siswa), &result.Submission.ID, &t.KelasID, ip, userAgent, map[string]any{
			"tugas_id":      tugasID.String(),
			"submission_id": result.Submission.ID.String(),
			"deadline":      t.Deadline.Format(time.RFC3339),
		})
	}

	// 11. Reload with attachments preloaded for response.
	full, ferr := s.repo.FindByID(ctx, result.Submission.ID)
	if ferr != nil {
		// Audit was already logged; return shallow row.
		return &result, nil
	}
	result.Submission = full
	return &result, nil
}

// sanitizeAttachmentFilename mirrors tugas.sanitizeAttachmentFilename.
func sanitizeAttachmentFilename(raw, ext string) string {
	s := strings.TrimSpace(raw)
	fallback := "submission." + ext
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
