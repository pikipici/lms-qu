// Repository for the banksoal (BankSoal) domain.
//
// Fase 6.A foundation skeleton: method signatures only. Bodies that
// require business logic (CRUD validation, image upload coordination,
// bulk paste parsing) return errNotImplemented and will be filled in
// by tasks 6.B-6.E. Pure persistence helpers (Create / FindByID /
// list) are implemented up-front because they have no business
// decisions to make.
package banksoal

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for bank_soal.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a banksoal repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB so service-layer transactions can
// share the same connection (mirror soalbab.Repo.DB pattern).
func (r *Repo) DB() *gorm.DB { return r.db }

// ErrVersionConflict is returned by mutating ops when the row exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("banksoal: version conflict")

// errNotImplemented marks repo methods whose bodies will be added in
// later 6.B-6.E tasks. Returning a typed sentinel keeps server boot +
// build green during the foundation pass.
var errNotImplemented = errors.New("banksoal: not implemented (foundation skeleton)")

// ListFilter narrows ListByOwner results.
type ListFilter struct {
	// Mapel, when non-empty, narrows by mapel (exact match).
	Mapel string
	// Tingkat, when non-empty, narrows by tingkat (exact match).
	Tingkat string
	// Topik, when non-empty, narrows by topik (substring match).
	Topik string
	// Limit caps result count. <=0 → no cap.
	Limit int
	// Offset skips first N rows.
	Offset int
}

// CreateSoal inserts a new bank_soal row.
func (r *Repo) CreateSoal(ctx context.Context, s *BankSoal) error {
	return r.db.WithContext(ctx).Create(s).Error
}

// FindSoalByID returns a non-deleted bank_soal by id.
func (r *Repo) FindSoalByID(ctx context.Context, id uuid.UUID) (*BankSoal, error) {
	var s BankSoal
	if err := r.db.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// FindSoalByIDIncludingDeleted returns a bank_soal even if soft-deleted.
// Used when rendering review for a HasilUjian whose soal_ids point to
// rows guru sudah hapus.
func (r *Repo) FindSoalByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*BankSoal, error) {
	var s BankSoal
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByOwner returns bank_soal owned by guruID, filtered + paginated.
// Soft-deleted rows excluded.
func (r *Repo) ListByOwner(ctx context.Context, guruID uuid.UUID, f ListFilter) ([]BankSoal, error) {
	q := r.db.WithContext(ctx).Model(&BankSoal{}).
		Where("owner_guru_id = ? AND deleted_at IS NULL", guruID)
	if f.Mapel != "" {
		q = q.Where("mapel = ?", f.Mapel)
	}
	if f.Tingkat != "" {
		q = q.Where("tingkat = ?", f.Tingkat)
	}
	if f.Topik != "" {
		q = q.Where("topik ILIKE ?", "%"+f.Topik+"%")
	}
	q = q.Order("created_at DESC, id DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var rows []BankSoal
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByOwner returns total non-deleted soal owned by guruID matching
// the filter — used for pagination metadata + random-mode pool sizing.
func (r *Repo) CountByOwner(ctx context.Context, guruID uuid.UUID, f ListFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&BankSoal{}).
		Where("owner_guru_id = ? AND deleted_at IS NULL", guruID)
	if f.Mapel != "" {
		q = q.Where("mapel = ?", f.Mapel)
	}
	if f.Tingkat != "" {
		q = q.Where("tingkat = ?", f.Tingkat)
	}
	if f.Topik != "" {
		q = q.Where("topik ILIKE ?", "%"+f.Topik+"%")
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListIDsByOwnerFilter returns soal IDs (non-deleted) matching the
// filter — used by Ujian random-mode pool snapshot. Order is
// deterministic by created_at DESC, id DESC; caller shuffles via
// deterministic seed (locked #86).
func (r *Repo) ListIDsByOwnerFilter(ctx context.Context, guruID uuid.UUID, f ListFilter) ([]uuid.UUID, error) {
	q := r.db.WithContext(ctx).Model(&BankSoal{}).
		Where("owner_guru_id = ? AND deleted_at IS NULL", guruID)
	if f.Mapel != "" {
		q = q.Where("mapel = ?", f.Mapel)
	}
	if f.Tingkat != "" {
		q = q.Where("tingkat = ?", f.Tingkat)
	}
	if f.Topik != "" {
		q = q.Where("topik ILIKE ?", "%"+f.Topik+"%")
	}
	q = q.Order("created_at DESC, id DESC")
	var ids []uuid.UUID
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// FindSoalsByIDs loads multiple bank_soal rows, returning them in the
// same order as the input ids slice. Missing/soft-deleted ids are
// skipped silently — caller decides whether that's an error (e.g.
// review render renders placeholder).
func (r *Repo) FindSoalsByIDs(ctx context.Context, ids []uuid.UUID) ([]BankSoal, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []BankSoal
	if err := r.db.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// Re-order to match input ids.
	byID := make(map[uuid.UUID]BankSoal, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	out := make([]BankSoal, 0, len(rows))
	for _, id := range ids {
		if r, ok := byID[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

// UpdateSoal will be implemented in 6.B.1 — patch validated fields with
// optimistic version bump.
func (r *Repo) UpdateSoal(ctx context.Context, s *BankSoal) error {
	return errNotImplemented
}

// SoftDeleteSoal marks deleted_at = now() if version matches. Returns
// ErrVersionConflict if version drifted, gorm.ErrRecordNotFound if
// row already deleted or never existed.
func (r *Repo) SoftDeleteSoal(ctx context.Context, id uuid.UUID, expectedVersion int) error {
	return errNotImplemented
}

// HardDeleteSoal removes the row entirely. Caller MUST ensure no
// HasilUjian still references the soal_id (locked #84 soft-delete
// fallback when references exist).
func (r *Repo) HardDeleteSoal(ctx context.Context, id uuid.UUID) error {
	return errNotImplemented
}
