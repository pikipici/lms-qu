// Get/List + Grade business logic untuk submission domain.
package submission

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/storage"
)

// MySubmissionResult bundles a siswa's own submission (kalau ada) + tugas
// info needed untuk pre-fill UI (deadline, izinkan_late, penalty_persen,
// wajib_attachment).
type MySubmissionResult struct {
	Submission *Submission `json:"submission,omitempty"`
	Tugas      TugasInfo   `json:"tugas"`
}

// TugasInfo is the minimal tugas snapshot returned alongside MySubmission.
type TugasInfo struct {
	ID              uuid.UUID  `json:"id"`
	KelasID         uuid.UUID  `json:"kelas_id"`
	Judul           string     `json:"judul"`
	Deadline        *time.Time `json:"deadline,omitempty"`
	IzinkanLate     bool       `json:"izinkan_late"`
	PenaltyPersen   int16      `json:"penalty_persen"`
	WajibAttachment bool       `json:"wajib_attachment"`
}

// GetMySubmission returns siswa's own submission for a tugas + tugas info.
// Submission may be nil (siswa belum submit). Enrollment + tugas published
// guards apply.
func (s *Service) GetMySubmission(ctx context.Context, tugasID, siswaID uuid.UUID) (*MySubmissionResult, error) {
	t, err := s.findTugasOrNotFound(ctx, tugasID)
	if err != nil {
		return nil, err
	}
	// Hide draft/archived from siswa via 404 (locked tugas access policy).
	if t.Status != "published" {
		return nil, ErrNotFound
	}
	if err := s.assertEnrolled(ctx, t.KelasID, siswaID); err != nil {
		return nil, err
	}

	res := &MySubmissionResult{
		Tugas: TugasInfo{
			ID:              t.ID,
			KelasID:         t.KelasID,
			Judul:           t.Judul,
			Deadline:        t.Deadline,
			IzinkanLate:     t.IzinkanLate,
			PenaltyPersen:   t.PenaltyPersen,
			WajibAttachment: t.WajibAttachment,
		},
	}
	sub, err := s.repo.FindByTugasSiswa(ctx, tugasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return res, nil
	}
	if err != nil {
		return nil, fmt.Errorf("submission find by pair: %w", err)
	}
	res.Submission = sub
	return res, nil
}

// ListByTugas returns submissions for a tugas (rekap guru). Owner-only.
// status filter optional.
func (s *Service) ListByTugas(ctx context.Context, tugasID, callerID uuid.UUID, callerRole string, statusFilter *Status) ([]Submission, error) {
	t, err := s.findTugasOrNotFound(ctx, tugasID)
	if err != nil {
		return nil, err
	}
	if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	if statusFilter != nil && !statusFilter.Valid() {
		return nil, fmt.Errorf("%w: invalid status filter", ErrInvalidInput)
	}
	return s.repo.ListByTugas(ctx, tugasID, StatusFilter{Status: statusFilter})
}

// GetByID returns a submission with attachments preloaded. Owner-only OR
// siswa pemilik.
func (s *Service) GetByID(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Submission, error) {
	sub, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission find: %w", err)
	}

	if callerRole == string(auth.Siswa) {
		if sub.SiswaID != callerID {
			return nil, ErrForbidden
		}
		// Verify enrollment masih active untuk defense in depth.
		t, err := s.findTugasOrNotFound(ctx, sub.TugasID)
		if err != nil {
			return nil, err
		}
		if err := s.assertEnrolled(ctx, t.KelasID, callerID); err != nil {
			return nil, err
		}
	} else {
		// guru/admin: owner check via tugas.kelas.
		t, err := s.findTugasOrNotFound(ctx, sub.TugasID)
		if err != nil {
			return nil, err
		}
		if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
			return nil, err
		}
	}
	return sub, nil
}

// AttachmentURLResult bundles presigned URL + meta for download UI.
type AttachmentURLResult struct {
	URL              string
	ExpiresAt        time.Time
	OriginalFilename string
	MimeType         string
}

// PresignAttachmentURL issues a short-lived GET URL for a submission
// attachment. Authorization branches by role:
//   - guru/admin: owner via tugas.kelas
//   - siswa: must own the submission + enrolled
func (s *Service) PresignAttachmentURL(ctx context.Context, submissionID, attachmentID, callerID uuid.UUID, callerRole, ip, userAgent string) (*AttachmentURLResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	sub, err := s.repo.FindByID(ctx, submissionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission presign find: %w", err)
	}
	t, err := s.findTugasOrNotFound(ctx, sub.TugasID)
	if err != nil {
		return nil, err
	}

	if callerRole == string(auth.Siswa) {
		if sub.SiswaID != callerID {
			return nil, ErrForbidden
		}
		if err := s.assertEnrolled(ctx, t.KelasID, callerID); err != nil {
			return nil, err
		}
	} else {
		if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
			return nil, err
		}
	}

	att, err := s.repo.FindAttachmentByID(ctx, submissionID, attachmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission presign attachment find: %w", err)
	}

	url, perr := s.store.PresignGetDownload(ctx, att.ObjectKey, PresignTTL, att.OriginalFilename)
	if perr != nil {
		if errors.Is(perr, storage.ErrObjectNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("submission presign: %w", perr)
	}

	expiresAt := s.now().Add(PresignTTL)
	s.logAudit(ctx, "submission_attachment_url_issued", &callerID, callerRole, &submissionID, &t.KelasID, ip, userAgent, map[string]any{
		"submission_id":  submissionID.String(),
		"attachment_id":  attachmentID.String(),
		"object_key":     att.ObjectKey,
		"ttl":            int(PresignTTL.Seconds()),
	})
	return &AttachmentURLResult{
		URL:              url,
		ExpiresAt:        expiresAt,
		OriginalFilename: att.OriginalFilename,
		MimeType:         att.MimeType,
	}, nil
}

// GradeInput holds POST /submission/:id/grade body.
type GradeInput struct {
	NilaiAsli       float64
	Feedback        string
	ExpectedVersion int
}

// Grade applies guru's nilai + feedback. Owner-only. Penalty calc applied
// kalau is_late=true && tugas.penalty_persen > 0 (locked #71).
//
// Tx flow (locked #73):
//  1. Validate input (nilai 0-100, feedback len, version positive).
//  2. Find submission + tugas + ownership guard (kelas).
//  3. BEGIN tx → LockByID → cek Status='submitted'.
//  4. Compute penalty + nilai_setelah_penalty.
//  5. GradeUpdate (optimistic concurrency #56).
//  6. Audit log + COMMIT.
func (s *Service) Grade(ctx context.Context, submissionID, callerID uuid.UUID, callerRole string, in GradeInput, ip, userAgent string) (*Submission, error) {
	// 1. Validate.
	if in.NilaiAsli < 0 || in.NilaiAsli > 100 {
		return nil, fmt.Errorf("%w: nilai_asli must be 0..100", ErrInvalidInput)
	}
	if len(in.Feedback) > MaxFeedbackBytes {
		return nil, fmt.Errorf("%w: feedback exceeds %d bytes", ErrInvalidInput, MaxFeedbackBytes)
	}
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	// 2. Find submission + tugas + ownership.
	sub, err := s.repo.FindByID(ctx, submissionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission grade find: %w", err)
	}
	t, err := s.findTugasOrNotFound(ctx, sub.TugasID)
	if err != nil {
		return nil, err
	}
	k, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	_ = k

	// 3-5. Tx: lock + status guard + grade update.
	now := s.now()
	var (
		penaltyApplied      int16
		nilaiSetelahPenalty float64
	)
	txErr := s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		locked, lerr := s.repo.LockByID(ctx, tx, submissionID)
		if errors.Is(lerr, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if lerr != nil {
			return fmt.Errorf("submission grade lock: %w", lerr)
		}
		if locked.Status == StatusGraded {
			return ErrAlreadyGraded
		}
		if locked.Status != StatusSubmitted {
			return fmt.Errorf("%w: status must be 'submitted'", ErrInvalidInput)
		}

		// Penalty calc — only when late + penalty configured (locked #71).
		if locked.IsLate && t.PenaltyPersen > 0 {
			penaltyApplied = t.PenaltyPersen
			nilaiSetelahPenalty = RoundNilai(in.NilaiAsli * (1.0 - float64(t.PenaltyPersen)/100.0))
		} else {
			penaltyApplied = 0
			nilaiSetelahPenalty = RoundNilai(in.NilaiAsli)
		}

		nilaiAsliRounded := RoundNilai(in.NilaiAsli)
		fields := map[string]any{
			"status":                  StatusGraded,
			"nilai_asli":              nilaiAsliRounded,
			"penalty_persen_applied":  penaltyApplied,
			"nilai_setelah_penalty":   nilaiSetelahPenalty,
			"feedback":                in.Feedback,
			"graded_by_id":            callerID,
			"graded_at":               now,
		}
		if uerr := s.repo.GradeUpdate(ctx, tx, submissionID, in.ExpectedVersion, fields); uerr != nil {
			return uerr
		}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}

	// 6. Audit log.
	s.logAudit(ctx, "tugas_graded", &callerID, callerRole, &submissionID, &t.KelasID, ip, userAgent, map[string]any{
		"submission_id":           submissionID.String(),
		"tugas_id":                t.ID.String(),
		"siswa_id":                sub.SiswaID.String(),
		"nilai_asli":              RoundNilai(in.NilaiAsli),
		"penalty_persen_applied":  penaltyApplied,
		"nilai_setelah_penalty":   nilaiSetelahPenalty,
		"is_late":                 sub.IsLate,
	})

	// Reload with attachments for response.
	full, ferr := s.repo.FindByID(ctx, submissionID)
	if ferr != nil {
		return nil, fmt.Errorf("submission grade reload: %w", ferr)
	}
	return full, nil
}
