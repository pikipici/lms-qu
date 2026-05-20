// Bab duplicate endpoint (Task 3.A.4).
//
// Duplicate copies the basic fields of an existing bab into a new row in the
// SAME kelas, with status=draft, version=1, urutan=max+1. The new bab gets a
// suffixed judul ("<original> (Salinan)") unless the caller supplies a
// custom judul.
//
// SCOPE NOTE: this is the bab-only duplicate. Materi + Pengumuman child copy
// (with R2 CopyObject for PDF objects) is deferred to Task 3.C.x / 3.F.x
// once those tables exist. The hooks (childCopier interface) are wired here
// so the extension can plug in without changing the handler signature.
package bab

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DuplicateInput holds the optional override for POST /bab/:id/duplicate.
type DuplicateInput struct {
	// Judul overrides the auto-suffixed default. Empty = use
	// "<original_judul> (Salinan)".
	Judul string
}

// childCopier is the extension point for copying materi + pengumuman during
// duplicate. It is intentionally tiny — when Task 3.C / 3.F land, the
// implementations will be wired in main.go and called inside the duplicate
// transaction. Returning an error rolls the whole duplicate back; any R2
// side effects already performed by the implementation must be compensated
// by the implementation itself (it knows what it copied).
type childCopier interface {
	// CopyChildren copies all child rows attached to srcBabID into newBabID,
	// running inside the provided tx. Implementations are responsible for
	// their own external side effects (e.g. R2 CopyObject) and compensation
	// on failure.
	CopyChildren(ctx context.Context, tx *gorm.DB, srcBabID, newBabID uuid.UUID) (childSummary, error)
}

// childSummary is recorded in the audit log so guru can see what was copied.
type childSummary struct {
	MateriCount     int `json:"materi_count"`
	PengumumanCount int `json:"pengumuman_count"`
}

// Duplicate creates a copy of an existing bab in the same kelas.
//
// Behavior:
//   - Source bab must exist and caller must own its kelas (or be admin).
//   - Source bab status='archived' is rejected (cannot duplicate a tombstone).
//   - Kelas status='archived' is rejected (cannot add new bab to archived kelas).
//   - New bab: same nomor + deskripsi as source; judul = override or
//     "<source.judul> (Salinan)"; urutan = max+1; status=draft; version=1.
//   - Audit: action='bab_duplicated', meta carries source_bab_id + judul +
//     materi_count + pengumuman_count (zeros when no childCopier wired).
func (s *Service) Duplicate(ctx context.Context, srcID, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Bab, error) {
	src, err := s.repo.FindByID(ctx, srcID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bab duplicate find: %w", err)
	}
	k, err := s.findKelasOrForbidden(ctx, src.KelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}
	if src.Status == StatusArchived {
		return nil, ErrAlreadyArchived
	}

	// Resolve target judul. Empty override -> auto-suffix.
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		judul = src.Judul + " (Salinan)"
	}

	maxUrutan, err := s.repo.MaxUrutan(ctx, src.KelasID)
	if err != nil {
		return nil, fmt.Errorf("bab duplicate max urutan: %w", err)
	}

	dst := &Bab{
		KelasID:   src.KelasID,
		Nomor:     src.Nomor,
		Judul:     judul,
		Deskripsi: src.Deskripsi,
		Urutan:    maxUrutan + 1,
		Status:    StatusDraft,
		Version:   1,
	}

	// Run create + child copy in a single tx so we can roll back if a child
	// copy fails. When childCopier is nil (current MVP), the tx body just
	// inserts the bab row and commits immediately.
	summary := childSummary{}
	db := s.repo.DB()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(dst).Error; err != nil {
			return fmt.Errorf("bab duplicate create: %w", err)
		}
		if s.childCopier != nil {
			cs, copyErr := s.childCopier.CopyChildren(ctx, tx, src.ID, dst.ID)
			if copyErr != nil {
				return fmt.Errorf("bab duplicate child copy: %w", copyErr)
			}
			summary = cs
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.logAudit(ctx, "bab_duplicated", &callerID, callerRole, &dst.ID, &src.KelasID, ip, userAgent, map[string]any{
		"source_bab_id":    src.ID.String(),
		"new_bab_id":       dst.ID.String(),
		"new_judul":        dst.Judul,
		"new_urutan":       dst.Urutan,
		"materi_count":     summary.MateriCount,
		"pengumuman_count": summary.PengumumanCount,
	})
	return dst, nil
}
