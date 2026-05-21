// Tugas duplicate endpoint (Task 4.A.4).
//
// Duplicate copies the basic fields of an existing tugas into a new row in
// the SAME kelas. Default new judul = "<original> (Salinan)" unless caller
// supplies an override. New tugas:
//   - status = draft (siswa gak lihat sampai guru explicit publish, even kalau
//     source published — ngehindari accidental announce ke siswa)
//   - version = 1
//   - deadline + late + penalty + wajib_attachment + bab_id ikut source
//   - attachments di-copy via R2 CopyObject ke uuid baru per locked #58/#74
//
// Compensating flow (locked #62/#69):
//   1. Copy R2 objects FIRST (server-side, no body transfer).
//   2. INSERT tugas + tugas_attachment rows dalam single tx.
//   3. Kalau tx fail mid-flight → DeleteObject untuk semua copied keys.
//
// Audit: action='tugas_duplicated' meta {source_tugas_id, new_tugas_id,
//   attachment_count}. Mirror bab_duplicated shape.
package tugas

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/storage"
)

// DuplicateInput holds the optional override for POST /tugas/:id/duplicate.
type DuplicateInput struct {
	// Judul overrides the auto-suffixed default. Empty = use
	// "<original_judul> (Salinan)".
	Judul string
}

// Duplicate creates a copy of an existing tugas in the same kelas.
//
// Behavior:
//   - Source tugas must exist and caller must own its kelas (or be admin).
//   - Source tugas status='archived' is rejected (cannot duplicate a tombstone).
//   - Kelas archived is rejected (cannot add new tugas to archived kelas).
//   - New tugas: same fields as source (bab_id, deskripsi, deadline,
//     izinkan_late, penalty_persen, wajib_attachment); judul = override or
//     "<source.judul> (Salinan)"; status=draft; version=1.
//   - Attachments: per source attachment, server-side R2 CopyObject ke
//     "tugas/<uuid_baru>.<ext>" + insert tugas_attachment row di tx baru.
//   - Audit: action='tugas_duplicated', meta carries source_tugas_id +
//     new_tugas_id + attachment_count.
func (s *Service) Duplicate(ctx context.Context, srcID, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Tugas, error) {
	src, err := s.repo.FindByID(ctx, srcID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas duplicate find: %w", err)
	}
	k, err := s.findKelasOrForbidden(ctx, src.KelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}
	if src.Status == StatusArchived {
		return nil, fmt.Errorf("%w: source tugas archived", ErrInvalidInput)
	}

	// Resolve target judul. Empty override -> auto-suffix.
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		judul = src.Judul + " (Salinan)"
	}
	if len(judul) > MaxJudulBytes {
		return nil, fmt.Errorf("%w: judul exceeds %d chars", ErrInvalidInput, MaxJudulBytes)
	}

	// Pre-copy R2 attachments BEFORE DB tx — kalau R2 fail, no DB side
	// effects yet. Track copied keys untuk compensating delete kalau DB
	// tx gagal nanti.
	type attCopy struct {
		newKey   string
		original string
		mime     string
		size     int64
	}
	var copied []attCopy
	rollbackR2 := func() {
		if s.store == nil {
			return
		}
		for _, c := range copied {
			_ = s.store.DeleteObject(context.Background(), c.newKey)
		}
	}

	if len(src.Attachments) > 0 {
		if s.store == nil {
			return nil, ErrR2Required
		}
		for _, a := range src.Attachments {
			ext := strings.TrimPrefix(strings.ToLower(path.Ext(a.ObjectKey)), ".")
			if ext == "" {
				ext = "bin"
			}
			newID := uuid.New()
			newKey, kerr := storage.BuildKey(storage.CategoryTugas, newID.String()+"."+ext)
			if kerr != nil {
				rollbackR2()
				return nil, fmt.Errorf("tugas duplicate build key: %w", kerr)
			}
			if cerr := s.store.CopyObject(ctx, a.ObjectKey, newKey); cerr != nil {
				rollbackR2()
				return nil, fmt.Errorf("%w: copy %s: %v", ErrAttachmentUploadFailed, a.ObjectKey, cerr)
			}
			copied = append(copied, attCopy{
				newKey:   newKey,
				original: a.OriginalFilename,
				mime:     a.MimeType,
				size:     a.SizeBytes,
			})
		}
	}

	dst := &Tugas{
		KelasID:         src.KelasID,
		BabID:           src.BabID,
		Judul:           judul,
		Deskripsi:       src.Deskripsi,
		Deadline:        src.Deadline,
		IzinkanLate:     src.IzinkanLate,
		PenaltyPersen:   src.PenaltyPersen,
		WajibAttachment: src.WajibAttachment,
		Status:          StatusDraft,
		Version:         1,
		CreatedByID:     callerID,
	}

	// Run create + attachment copy in a single tx so DB stays consistent.
	repo, ok := s.repo.(interface{ DB() *gorm.DB })
	if !ok {
		rollbackR2()
		return nil, fmt.Errorf("tugas duplicate: repo does not expose DB()")
	}
	db := repo.DB()
	if db == nil {
		rollbackR2()
		return nil, fmt.Errorf("tugas duplicate: nil db")
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(dst).Error; err != nil {
			return fmt.Errorf("tugas duplicate create: %w", err)
		}
		for _, c := range copied {
			att := &Attachment{
				TugasID:          dst.ID,
				ObjectKey:        c.newKey,
				OriginalFilename: c.original,
				MimeType:         c.mime,
				SizeBytes:        c.size,
			}
			if cerr := tx.Create(att).Error; cerr != nil {
				return fmt.Errorf("tugas duplicate attachment row: %w", cerr)
			}
		}
		return nil
	})
	if err != nil {
		rollbackR2()
		return nil, err
	}

	s.logAudit(ctx, "tugas_duplicated", &callerID, callerRole, &dst.ID, &src.KelasID, ip, userAgent, map[string]any{
		"source_tugas_id":  src.ID.String(),
		"new_tugas_id":     dst.ID.String(),
		"new_judul":        dst.Judul,
		"attachment_count": len(copied),
	})
	return dst, nil
}
