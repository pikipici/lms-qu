// Repository for the tugas (assignment) domain.
package tugas

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for tugas + tugas_attachment.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a tugas repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by mutating ops when the row exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("tugas: version conflict")

// BabFilterMode discriminates BabFilter behaviour.
type BabFilterMode int

const (
	// BabFilterAny ignores bab_id (return all in kelas).
	BabFilterAny BabFilterMode = iota
	// BabFilterNull pins bab_id IS NULL (tugas kelas-wide).
	BabFilterNull
	// BabFilterEq pins bab_id = BabID.
	BabFilterEq
)

// BabFilter scopes a list query by bab_id.
type BabFilter struct {
	Mode  BabFilterMode
	BabID uuid.UUID
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

// Create inserts a new tugas. Caller is responsible for setting all
// required fields (KelasID, Judul, CreatedByID, etc).
func (r *Repo) Create(ctx context.Context, t *Tugas) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// FindByID returns a tugas by id with attachments preloaded.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Tugas, error) {
	var t Tugas
	if err := r.db.WithContext(ctx).
		Preload("Attachments").
		Where("id = ?", id).
		First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// ListByKelas returns tugas in a kelas, ordered by created_at DESC.
//
// Status, when non-nil, pins the result. Bab narrows by bab association.
// Attachments are NOT preloaded (list view typically only needs metadata;
// caller should call FindByID for detail).
func (r *Repo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Tugas, error) {
	q := r.db.WithContext(ctx).Model(&Tugas{}).Where("kelas_id = ?", kelasID)
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

	var rows []Tugas
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListByBab returns tugas rows belonging to a single bab, ordered by
// created_at DESC. Convenience wrapper that does NOT scope by kelas —
// caller must verify bab→kelas ownership separately.
func (r *Repo) ListByBab(ctx context.Context, babID uuid.UUID, f ListFilter) ([]Tugas, error) {
	q := r.db.WithContext(ctx).Model(&Tugas{}).Where("bab_id = ?", babID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	q = q.Order("created_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}

	var rows []Tugas
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByKelas returns the number of tugas in a kelas matching ListFilter.
func (r *Repo) CountByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&Tugas{}).Where("kelas_id = ?", kelasID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	switch f.Bab.Mode {
	case BabFilterNull:
		q = q.Where("bab_id IS NULL")
	case BabFilterEq:
		q = q.Where("bab_id = ?", f.Bab.BabID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountByBab returns the total number of tugas rows attached to a bab.
func (r *Repo) CountByBab(ctx context.Context, babID uuid.UUID, f ListFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&Tugas{}).Where("bab_id = ?", babID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateBasic applies an optimistic-concurrency patch over editable fields.
// Caller computes resolved values (judul, deskripsi, bab_id, deadline,
// izinkan_late, penalty_persen, wajib_attachment, status); repo just runs
// the UPDATE with version guard.
//
// On RowsAffected==0 we distinguish "row missing" (gorm.ErrRecordNotFound)
// from "version mismatch" (ErrVersionConflict) by re-reading the row.
func (r *Repo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	// Force version bump + updated_at refresh on every UpdateBasic call.
	fields["version"] = gorm.Expr("version + 1")
	fields["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).
		Model(&Tugas{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Tugas
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

// Delete hard-deletes a tugas row and returns the ObjectKeys of its
// attachments for compensating R2 cleanup (locked #69 pattern). FK CASCADE
// auto-deletes tugas_attachment rows; caller must DeleteObject on R2 for
// each returned key.
//
// Returns gorm.ErrRecordNotFound when the row does not exist.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID) ([]string, error) {
	var attachments []Attachment
	if err := r.db.WithContext(ctx).
		Select("id", "object_key").
		Where("tugas_id = ?", id).
		Find(&attachments).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(attachments))
	for _, a := range attachments {
		keys = append(keys, a.ObjectKey)
	}
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Tugas{})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return keys, nil
}

// AddAttachment inserts a single tugas_attachment row. Caller has already
// validated mime + size + count cap (locked #46/#74) and uploaded the
// object to R2.
func (r *Repo) AddAttachment(ctx context.Context, a *Attachment) error {
	return r.db.WithContext(ctx).Create(a).Error
}

// FindAttachmentByID returns a single attachment by id, scoped to its
// parent tugas via TugasID match (defensive — caller's URL must match).
func (r *Repo) FindAttachmentByID(ctx context.Context, tugasID, attachmentID uuid.UUID) (*Attachment, error) {
	var a Attachment
	if err := r.db.WithContext(ctx).
		Where("id = ? AND tugas_id = ?", attachmentID, tugasID).
		First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAttachmentsByTugas returns all attachments for a tugas, ordered by
// created_at ASC (display order = upload order).
func (r *Repo) ListAttachmentsByTugas(ctx context.Context, tugasID uuid.UUID) ([]Attachment, error) {
	var rows []Attachment
	if err := r.db.WithContext(ctx).
		Where("tugas_id = ?", tugasID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountAttachmentsByTugas returns the number of attachments for a tugas.
// Used to enforce the 5-attachment cap on upload (locked #74).
func (r *Repo) CountAttachmentsByTugas(ctx context.Context, tugasID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&Attachment{}).
		Where("tugas_id = ?", tugasID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// DeleteAttachment hard-deletes a single tugas_attachment row and returns
// its ObjectKey for compensating R2 cleanup. Returns gorm.ErrRecordNotFound
// when the row does not exist.
func (r *Repo) DeleteAttachment(ctx context.Context, tugasID, attachmentID uuid.UUID) (string, error) {
	var existing Attachment
	if err := r.db.WithContext(ctx).
		Select("id", "object_key").
		Where("id = ? AND tugas_id = ?", attachmentID, tugasID).
		First(&existing).Error; err != nil {
		return "", err
	}
	res := r.db.WithContext(ctx).
		Where("id = ? AND tugas_id = ?", attachmentID, tugasID).
		Delete(&Attachment{})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", gorm.ErrRecordNotFound
	}
	return existing.ObjectKey, nil
}

// DB returns the underlying gorm.DB so callers can run a transaction
// (e.g. service.Delete with attachment loop + R2 cleanup compensating).
// Mirrors materi.Repo.DB pattern.
func (r *Repo) DB() *gorm.DB { return r.db }
