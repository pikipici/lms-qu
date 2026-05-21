// Repository for the pengumuman (announcement) domain.
package pengumuman

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for pengumuman.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a pengumuman repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by mutating ops when the row exists but its
// current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("pengumuman: version conflict")

// BabFilterMode narrows ListByKelas by bab association.
type BabFilterMode int

const (
	// BabFilterAny — no bab filter (return all in kelas).
	BabFilterAny BabFilterMode = iota
	// BabFilterNull — pin bab_id IS NULL (kelas-wide pengumuman).
	BabFilterNull
	// BabFilterEq — pin bab_id = BabID.
	BabFilterEq
)

// BabFilter narrows ListByKelas results by bab association.
type BabFilter struct {
	Mode  BabFilterMode
	BabID uuid.UUID // only when Mode = BabFilterEq
}

// ListFilter narrows ListByKelas results.
//
// Status, when non-nil, pins the result to a single status (siswa list
// pakai &StatusPublished). Bab narrows by bab association.
type ListFilter struct {
	Status *Status
	Bab    BabFilter
	// Limit caps result count. <=0 → no cap (caller responsibility).
	Limit int
}

// Create inserts a new pengumuman.
func (r *Repo) Create(ctx context.Context, p *Pengumuman) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// FindByID returns a pengumuman by id.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Pengumuman, error) {
	var p Pengumuman
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByKelas returns pengumuman in a kelas, ordered by created_at DESC.
func (r *Repo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Pengumuman, error) {
	q := r.db.WithContext(ctx).Model(&Pengumuman{}).Where("kelas_id = ?", kelasID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	switch f.Bab.Mode {
	case BabFilterNull:
		q = q.Where("bab_id IS NULL")
	case BabFilterEq:
		q = q.Where("bab_id = ?", f.Bab.BabID)
	}
	q = q.Order("created_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}

	var rows []Pengumuman
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateBasic applies a partial patch with optimistic concurrency. Caller
// computes resolved values; repo just runs the UPDATE with version guard.
//
// Returns ErrVersionConflict when no row matches (id, version) — distinguish
// between "row missing" (caller already verified existence) vs "version
// stale" by re-reading; service does this anyway to surface the latest
// state.
func (r *Repo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, isi string, status Status) error {
	res := r.db.WithContext(ctx).
		Model(&Pengumuman{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Updates(map[string]any{
			"judul":   judul,
			"isi":     isi,
			"status":  status,
			"version": gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrVersionConflict
	}
	return nil
}

// Delete hard-deletes a pengumuman by id. Returns gorm.ErrRecordNotFound
// when id missing.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Pengumuman{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
