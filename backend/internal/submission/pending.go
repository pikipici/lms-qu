// Pending counters endpoint untuk guru sidebar (Task 4.E.2 — partial,
// activity feed full deferred ke Fase 7).
//
// GET /api/v1/guru/pending-counts — return total submissions yang belum
// dinilai di kelas yang dimiliki guru/admin. Used untuk badge sidebar +
// dashboard summary.
//
// Authorization: guru/admin. Guru hanya melihat kelas yang dia owner;
// admin lihat semua kelas.
package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// PendingCountsResult is the response shape of GET /guru/pending-counts.
//
// Saat ini cuma `ungraded_submissions`. Forward-compat untuk Fase 5/7:
// pending_review_ulangan_bab + pending_review_ulangan_harian (tambah field
// nanti tanpa breaking).
type PendingCountsResult struct {
	UngradedSubmissions int64 `json:"ungraded_submissions"`
}

// kelasOwnedLister provides the list of kelas IDs yang dimiliki guru.
// Service-level interface (kelas package wajib expose ListIDsByGuru).
type kelasOwnedLister interface {
	ListIDsByGuru(ctx context.Context, guruID uuid.UUID) ([]uuid.UUID, error)
}

// pendingCountsRepo is the subset of submission.Repo we need here. Wired
// against the package's *Repo via its DB() handle so we can run cross-
// kelas aggregate without N+1.
type pendingCountsRepo interface {
	DB() *gorm.DB
}

// PendingCounter computes guru pending counters cumulatively across the
// kelas yang dimiliki. Stateless — wrap the deps once at startup.
type PendingCounter struct {
	repo  pendingCountsRepo
	kelas kelasOwnedLister
}

// NewPendingCounter wires a counter for guru pending submissions.
func NewPendingCounter(repo pendingCountsRepo, kelas kelasOwnedLister) *PendingCounter {
	return &PendingCounter{repo: repo, kelas: kelas}
}

// Count returns pending counters scoped to caller. Admin sees all kelas;
// guru sees only kelas dia owner.
//
// Implementation: single SQL aggregate
//   SELECT COUNT(*) FROM submission s JOIN tugas t ON t.id = s.tugas_id
//   WHERE s.status = 'submitted' AND t.kelas_id IN (...)
// Untuk admin tanpa kelas filter → COUNT semua submission status='submitted'.
func (p *PendingCounter) Count(ctx context.Context, callerID uuid.UUID, callerRole string) (*PendingCountsResult, error) {
	q := p.repo.DB().WithContext(ctx).
		Table("submission s").
		Joins("JOIN tugas t ON t.id = s.tugas_id").
		Where("s.status = ?", StatusSubmitted)

	if callerRole == string(auth.Guru) {
		ids, err := p.kelas.ListIDsByGuru(ctx, callerID)
		if err != nil {
			return nil, fmt.Errorf("submission pending kelas list: %w", err)
		}
		if len(ids) == 0 {
			return &PendingCountsResult{UngradedSubmissions: 0}, nil
		}
		q = q.Where("t.kelas_id IN ?", ids)
	} else if callerRole != string(auth.Admin) {
		return nil, ErrForbidden
	}

	var n int64
	if err := q.Count(&n).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &PendingCountsResult{UngradedSubmissions: 0}, nil
		}
		return nil, fmt.Errorf("submission pending count: %w", err)
	}
	return &PendingCountsResult{UngradedSubmissions: n}, nil
}
