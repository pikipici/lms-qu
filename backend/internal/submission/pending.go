// Pending counters endpoint untuk guru sidebar (Task 4.E.2 + Task 7.D
// consolidated — locked #40 + #93).
//
// GET /api/v1/guru/pending-counts — return cumulative attention counters
// across kelas yang dimiliki guru/admin. Used untuk badge sidebar +
// dashboard summary. Polling 30s di FE.
//
// Authorization: guru/admin. Guru hanya melihat kelas yang dia owner;
// admin lihat semua kelas.
package submission

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// PendingCountsResult is the response shape of GET /guru/pending-counts
// (locked #93).
//
//   - UngradedSubmissions: COUNT submission status='submitted' AND
//     tugas.kelas_id IN guru-kelas. Tugas perlu di-grade.
//   - PendingReviewUlangan: COUNT hasil_soal_bab status='selesai' AND
//     mode='ulangan' AND ulangan_bab_setting.izinkan_review_setelah_submit
//     = true AND bab.kelas_id IN guru-kelas. Ulangan bab attempt yang
//     bisa di-review siswa (proxy attention).
//   - PendingReviewUjian: COUNT hasil_ujian status='selesai' AND
//     deleted_at IS NULL AND ujian.izinkan_review_setelah_submit=true AND
//     ujian.kelas_id IN guru-kelas.
//
// Note: lock #93 mengizinkan tweak semantic kalau test feel-test noisy —
// MVP pakai definition di atas.
type PendingCountsResult struct {
	UngradedSubmissions  int64 `json:"ungraded_submissions"`
	PendingReviewUlangan int64 `json:"pending_review_ulangan"`
	PendingReviewUjian   int64 `json:"pending_review_ujian"`
}

// PendingItemsResult gives the dashboard concrete destinations instead of
// only aggregate counts. Each category returns the latest actionable rows.
type PendingItemsResult struct {
	UngradedSubmissions  []PendingItem `json:"ungraded_submissions"`
	PendingReviewUlangan []PendingItem `json:"pending_review_ulangan"`
	PendingReviewUjian   []PendingItem `json:"pending_review_ujian"`
}

type PendingItem struct {
	ID          uuid.UUID  `json:"id"`
	KelasID     uuid.UUID  `json:"kelas_id"`
	KelasNama   string     `json:"kelas_nama"`
	Title       string     `json:"title"`
	Subtitle    string     `json:"subtitle,omitempty"`
	TargetURL   string     `json:"target_url"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
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
// Implementation: 3 SQL aggregates in parallel (sync.WaitGroup) sharing
// the kelas-IDs filter:
//
//   - ungraded_submissions: submission s JOIN tugas t WHERE
//     s.status='submitted' AND t.kelas_id IN (kelasIDs)
//   - pending_review_ulangan: hasil_soal_bab h JOIN soal_bab.bab b JOIN
//     ulangan_bab_setting u WHERE h.status='selesai' AND h.mode='ulangan'
//     AND b.kelas_id IN (kelasIDs) AND
//     u.izinkan_review_setelah_submit=true
//   - pending_review_ujian: hasil_ujian h JOIN ujian u WHERE
//     h.status='selesai' AND h.deleted_at IS NULL AND
//     u.izinkan_review_setelah_submit=true AND u.kelas_id IN (kelasIDs)
//
// Untuk admin tanpa kelas filter → COUNT semua (kelasIDs filter dropped).
func (p *PendingCounter) Count(ctx context.Context, callerID uuid.UUID, callerRole string) (*PendingCountsResult, error) {
	var kelasIDs []uuid.UUID
	switch callerRole {
	case string(auth.Guru):
		ids, err := p.kelas.ListIDsByGuru(ctx, callerID)
		if err != nil {
			return nil, fmt.Errorf("submission pending kelas list: %w", err)
		}
		if len(ids) == 0 {
			return &PendingCountsResult{}, nil
		}
		kelasIDs = ids
	case string(auth.Admin):
		// no kelas filter — admin sees all
	default:
		return nil, ErrForbidden
	}

	db := p.repo.DB().WithContext(ctx)

	var (
		wg                                            sync.WaitGroup
		ungraded, reviewUlangan, reviewUjian          int64
		errUngraded, errReviewUlangan, errReviewUjian error
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		q := db.Table("submission AS s").
			Joins("JOIN tugas t ON t.id = s.tugas_id").
			Where("s.status = ?", StatusSubmitted)
		if len(kelasIDs) > 0 {
			q = q.Where("t.kelas_id IN ?", kelasIDs)
		}
		errUngraded = q.Count(&ungraded).Error
	}()

	go func() {
		defer wg.Done()
		// hasil_soal_bab → bab via soal_bab snapshot is messy; use
		// hasil_soal_bab.bab_id directly (denormal). Join bab for kelas
		// filter, ulangan_bab_setting for review-flag.
		q := db.Table("hasil_soal_bab AS h").
			Joins("JOIN bab b ON b.id = h.bab_id").
			Joins("JOIN ulangan_bab_setting u ON u.bab_id = h.bab_id").
			Where("h.status = ?", "selesai").
			Where("h.mode = ?", "ulangan").
			Where("u.izinkan_review_setelah_submit = ?", true)
		if len(kelasIDs) > 0 {
			q = q.Where("b.kelas_id IN ?", kelasIDs)
		}
		errReviewUlangan = q.Count(&reviewUlangan).Error
	}()

	go func() {
		defer wg.Done()
		q := db.Table("hasil_ujian AS h").
			Joins("JOIN ujian u ON u.id = h.ujian_id").
			Where("h.status = ?", "selesai").
			Where("h.deleted_at IS NULL").
			Where("u.izinkan_review_setelah_submit = ?", true)
		if len(kelasIDs) > 0 {
			q = q.Where("u.kelas_id IN ?", kelasIDs)
		}
		errReviewUjian = q.Count(&reviewUjian).Error
	}()

	wg.Wait()

	for _, e := range []error{errUngraded, errReviewUlangan, errReviewUjian} {
		if e != nil && !isNoRowsErr(e) {
			return nil, fmt.Errorf("submission pending count: %w", e)
		}
	}

	return &PendingCountsResult{
		UngradedSubmissions:  ungraded,
		PendingReviewUlangan: reviewUlangan,
		PendingReviewUjian:   reviewUjian,
	}, nil
}

// isNoRowsErr treats gorm.ErrRecordNotFound (impossible from COUNT but
// guard) as zero — empty aggregate is a valid 0 not an error.
func isNoRowsErr(err error) bool {
	return err == gorm.ErrRecordNotFound
}

func (p *PendingCounter) Items(ctx context.Context, callerID uuid.UUID, callerRole string, limit int) (*PendingItemsResult, error) {
	if limit <= 0 || limit > 10 {
		limit = 3
	}
	kelasIDs, adminScope, err := p.scopeKelasIDs(ctx, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if !adminScope && len(kelasIDs) == 0 {
		return &PendingItemsResult{}, nil
	}

	db := p.repo.DB().WithContext(ctx)
	var result PendingItemsResult

	ungraded := db.Table("submission AS s").
		Select("s.id, t.kelas_id, k.nama AS kelas_nama, t.judul AS title, u.name AS subtitle, '/guru/kelas/detail/tugas?id=' || t.kelas_id::text || '&tid=' || t.id::text AS target_url, s.submitted_at").
		Joins("JOIN tugas t ON t.id = s.tugas_id").
		Joins("JOIN kelas k ON k.id = t.kelas_id").
		Joins("JOIN users u ON u.id = s.siswa_id").
		Where("s.status = ?", StatusSubmitted).
		Order("s.submitted_at DESC").
		Limit(limit)
	if len(kelasIDs) > 0 {
		ungraded = ungraded.Where("t.kelas_id IN ?", kelasIDs)
	}
	if err := ungraded.Scan(&result.UngradedSubmissions).Error; err != nil {
		return nil, fmt.Errorf("submission pending items ungraded: %w", err)
	}

	ulangan := db.Table("hasil_soal_bab AS h").
		Select("h.id, b.kelas_id, k.nama AS kelas_nama, b.judul AS title, u.name AS subtitle, '/guru/kelas/detail/bab?id=' || b.kelas_id::text || '&bid=' || b.id::text AS target_url, h.selesai_at AS finished_at").
		Joins("JOIN bab b ON b.id = h.bab_id").
		Joins("JOIN kelas k ON k.id = b.kelas_id").
		Joins("JOIN users u ON u.id = h.siswa_id").
		Joins("JOIN ulangan_bab_setting us ON us.bab_id = h.bab_id").
		Where("h.status = ?", "selesai").
		Where("h.mode = ?", "ulangan").
		Where("us.izinkan_review_setelah_submit = ?", true).
		Order("h.selesai_at DESC NULLS LAST, h.updated_at DESC").
		Limit(limit)
	if len(kelasIDs) > 0 {
		ulangan = ulangan.Where("b.kelas_id IN ?", kelasIDs)
	}
	if err := ulangan.Scan(&result.PendingReviewUlangan).Error; err != nil {
		return nil, fmt.Errorf("submission pending items ulangan: %w", err)
	}

	ujian := db.Table("hasil_ujian AS h").
		Select("h.id, uj.kelas_id, k.nama AS kelas_nama, uj.judul AS title, u.name AS subtitle, '/guru/kelas/detail?id=' || uj.kelas_id::text || '&tab=ujian' AS target_url, h.selesai_at AS finished_at").
		Joins("JOIN ujian uj ON uj.id = h.ujian_id").
		Joins("JOIN kelas k ON k.id = uj.kelas_id").
		Joins("JOIN users u ON u.id = h.siswa_id").
		Where("h.status = ?", "selesai").
		Where("h.deleted_at IS NULL").
		Where("uj.izinkan_review_setelah_submit = ?", true).
		Order("h.selesai_at DESC NULLS LAST, h.updated_at DESC").
		Limit(limit)
	if len(kelasIDs) > 0 {
		ujian = ujian.Where("uj.kelas_id IN ?", kelasIDs)
	}
	if err := ujian.Scan(&result.PendingReviewUjian).Error; err != nil {
		return nil, fmt.Errorf("submission pending items ujian: %w", err)
	}

	return &result, nil
}

func (p *PendingCounter) scopeKelasIDs(ctx context.Context, callerID uuid.UUID, callerRole string) ([]uuid.UUID, bool, error) {
	switch callerRole {
	case string(auth.Guru):
		ids, err := p.kelas.ListIDsByGuru(ctx, callerID)
		if err != nil {
			return nil, false, fmt.Errorf("submission pending kelas list: %w", err)
		}
		return ids, false, nil
	case string(auth.Admin):
		return nil, true, nil
	default:
		return nil, false, ErrForbidden
	}
}
