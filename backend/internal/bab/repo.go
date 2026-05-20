// Repository for the bab (chapter) domain.
package bab

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for bab.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a bab repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by mutating ops when the row exists but its
// current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("bab: version conflict")

// ErrInvalidStatus is returned when a transition target is not a valid Status.
var ErrInvalidStatus = errors.New("bab: invalid status")

// ListFilter narrows ListByKelas results.
//
// IncludeArchived=false hides Status='archived' rows. Status, when non-nil,
// pins the result to a single status (overrides IncludeArchived).
type ListFilter struct {
	IncludeArchived bool
	Status          *Status
}

// Create inserts a new bab. Caller is responsible for choosing nomor + urutan
// (urutan is typically max(urutan)+1 within the kelas).
func (r *Repo) Create(ctx context.Context, b *Bab) error {
	return r.db.WithContext(ctx).Create(b).Error
}

// FindByID returns a bab by id.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Bab, error) {
	var b Bab
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&b).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

// MaxUrutan returns the highest urutan value across all bab in a kelas, or 0
// when the kelas has no bab yet. Used to compute the next urutan slot on
// Create without an extra round trip.
func (r *Repo) MaxUrutan(ctx context.Context, kelasID uuid.UUID) (int, error) {
	var max *int
	if err := r.db.WithContext(ctx).
		Model(&Bab{}).
		Where("kelas_id = ?", kelasID).
		Select("MAX(urutan)").
		Scan(&max).Error; err != nil {
		return 0, err
	}
	if max == nil {
		return 0, nil
	}
	return *max, nil
}

// ListByKelas returns all bab in a kelas, ordered by urutan ASC. Filter
// controls archived/status visibility (siswa list typically passes
// Status=&StatusPublished).
func (r *Repo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Bab, error) {
	q := r.db.WithContext(ctx).Model(&Bab{}).Where("kelas_id = ?", kelasID)
	switch {
	case f.Status != nil:
		q = q.Where("status = ?", *f.Status)
	case !f.IncludeArchived:
		q = q.Where("status <> ?", StatusArchived)
	}

	var rows []Bab
	if err := q.Order("urutan ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByKelas returns the number of bab in a kelas matching the filter.
func (r *Repo) CountByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&Bab{}).Where("kelas_id = ?", kelasID)
	switch {
	case f.Status != nil:
		q = q.Where("status = ?", *f.Status)
	case !f.IncludeArchived:
		q = q.Where("status <> ?", StatusArchived)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateBasic applies an optimistic-concurrency update over the editable
// basic fields of a bab. Pin both id and the caller's expected version; on
// RowsAffected==0 we distinguish "row missing" (gorm.ErrRecordNotFound)
// from "version mismatch" (ErrVersionConflict) by re-reading the row.
//
// Status is updated separately via UpdateStatus to keep the audit log
// distinct (status_changed vs updated). Pass Urutan via UpdateUrutan when
// reordering — bulk reorder uses a transaction in service layer.
func (r *Repo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, nomor int, judul, deskripsi string) error {
	res := r.db.WithContext(ctx).
		Model(&Bab{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"nomor":      nomor,
			"judul":      judul,
			"deskripsi":  deskripsi,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Bab
		if err := r.db.WithContext(ctx).
			Select("id", "version").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// UpdateStatus transitions a bab to a new status with optimistic concurrency
// guard. All transitions are valid (draft <-> published <-> archived). The
// service layer logs the lama/baru pair to AuditLog.
func (r *Repo) UpdateStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status Status) error {
	if !status.Valid() {
		return ErrInvalidStatus
	}
	res := r.db.WithContext(ctx).
		Model(&Bab{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"status":     status,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Bab
		if err := r.db.WithContext(ctx).
			Select("id", "version").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// Archive is a convenience wrapper for UpdateStatus(StatusArchived). Kept
// separate so handlers can mount a dedicated POST /bab/:id/archive endpoint
// matching the kelas archive pattern (idempotent semantics).
//
// Returns gorm.ErrRecordNotFound if the row is missing or already archived
// (idempotent caller can ignore that). Version-aware variant lives in
// UpdateStatus.
func (r *Repo) Archive(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&Bab{}).
		Where("id = ? AND status <> ?", id, StatusArchived).
		UpdateColumns(map[string]any{
			"status":     StatusArchived,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUrutan sets the urutan of a single bab inside a transaction —
// caller (service-level Reorder) wraps multiple calls in a single tx with
// version checks.
func (r *Repo) UpdateUrutan(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion, urutan int) error {
	if tx == nil {
		tx = r.db.WithContext(ctx)
	} else {
		tx = tx.WithContext(ctx)
	}
	res := tx.Model(&Bab{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"urutan":     urutan,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Bab
		if err := tx.Select("id", "version").Where("id = ?", id).First(&probe).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// DB returns the underlying gorm.DB so callers can run a transaction. Used
// by the service-level Reorder which must update many rows atomically.
func (r *Repo) DB() *gorm.DB { return r.db }
