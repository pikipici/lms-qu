// Repository for the materi (learning content) domain.
package materi

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo provides GORM-backed persistence for materi + materi_read.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a materi repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by mutating ops when the row exists but its
// current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("materi: version conflict")

// ErrInvalidTipe is returned when a tipe value is not pdf|youtube|markdown.
var ErrInvalidTipe = errors.New("materi: invalid tipe")

// BabFilter scopes a list query by bab_id.
//
// Mode controls how BabID is used:
//   - BabFilterAny:  ignore BabID, return all rows in the kelas.
//   - BabFilterNull: only rows with bab_id IS NULL (materi berdiri bebas).
//   - BabFilterEq:   only rows with bab_id = BabID.
type BabFilter struct {
	Mode  BabFilterMode
	BabID uuid.UUID
}

// BabFilterMode discriminates BabFilter behaviour.
type BabFilterMode int

const (
	// BabFilterAny ignores bab_id (return all in kelas).
	BabFilterAny BabFilterMode = iota
	// BabFilterNull pins bab_id IS NULL.
	BabFilterNull
	// BabFilterEq pins bab_id = BabID.
	BabFilterEq
)

// Create inserts a new materi. Caller is responsible for computing urutan
// (typically max(urutan)+1 within the bab/kelas scope).
func (r *Repo) Create(ctx context.Context, m *Materi) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// FindByID returns a materi by id.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Materi, error) {
	var m Materi
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

// MaxUrutan returns the highest urutan across materi in a kelas scoped by
// BabFilter, or 0 when no rows match. Used to compute the next urutan slot
// on Create without an extra round trip.
func (r *Repo) MaxUrutan(ctx context.Context, kelasID uuid.UUID, f BabFilter) (int, error) {
	q := r.db.WithContext(ctx).Model(&Materi{}).Where("kelas_id = ?", kelasID)
	switch f.Mode {
	case BabFilterNull:
		q = q.Where("bab_id IS NULL")
	case BabFilterEq:
		q = q.Where("bab_id = ?", f.BabID)
	}
	var max *int
	if err := q.Select("MAX(urutan)").Scan(&max).Error; err != nil {
		return 0, err
	}
	if max == nil {
		return 0, nil
	}
	return *max, nil
}

// ListByKelas returns materi rows in a kelas, ordered by urutan ASC then
// created_at ASC. BabFilter narrows by bab_id (any/null/eq).
func (r *Repo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f BabFilter) ([]Materi, error) {
	q := r.db.WithContext(ctx).Model(&Materi{}).Where("kelas_id = ?", kelasID)
	switch f.Mode {
	case BabFilterNull:
		q = q.Where("bab_id IS NULL")
	case BabFilterEq:
		q = q.Where("bab_id = ?", f.BabID)
	}

	var rows []Materi
	if err := q.Order("urutan ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListByBab returns materi rows belonging to a single bab, ordered by
// urutan ASC then created_at ASC. Convenience wrapper that does NOT scope
// by kelas — caller must verify bab→kelas ownership separately.
func (r *Repo) ListByBab(ctx context.Context, babID uuid.UUID) ([]Materi, error) {
	var rows []Materi
	if err := r.db.WithContext(ctx).
		Where("bab_id = ?", babID).
		Order("urutan ASC, created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByKelas returns the number of materi in a kelas matching BabFilter.
func (r *Repo) CountByKelas(ctx context.Context, kelasID uuid.UUID, f BabFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&Materi{}).Where("kelas_id = ?", kelasID)
	switch f.Mode {
	case BabFilterNull:
		q = q.Where("bab_id IS NULL")
	case BabFilterEq:
		q = q.Where("bab_id = ?", f.BabID)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateBasic applies an optimistic-concurrency update over editable basic
// fields (judul, konten, urutan). Tipe is intentionally NOT updatable —
// changing tipe invalidates the payload invariant; caller must delete +
// recreate (see roadmap 3.C.2). On RowsAffected==0 we distinguish
// "row missing" (gorm.ErrRecordNotFound) from "version mismatch"
// (ErrVersionConflict) by re-reading the row.
func (r *Repo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, konten string, urutan int) error {
	res := r.db.WithContext(ctx).
		Model(&Materi{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"judul":      judul,
			"konten":     konten,
			"urutan":     urutan,
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Materi
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

// Delete hard-deletes a materi row and returns its ObjectKey (or nil for
// non-pdf tipe). Caller is responsible for R2 cleanup compensating delete
// when ObjectKey != nil (locked #69 — hard delete + R2 DeleteObject).
//
// Returns gorm.ErrRecordNotFound when the row does not exist.
func (r *Repo) Delete(ctx context.Context, id uuid.UUID) (*string, error) {
	var existing Materi
	if err := r.db.WithContext(ctx).
		Select("id", "object_key").
		Where("id = ?", id).
		First(&existing).Error; err != nil {
		return nil, err
	}
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Materi{})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return existing.ObjectKey, nil
}

// MarkRead inserts a materi_read row idempotently via ON CONFLICT DO
// NOTHING. Returns (read, wasNew):
//   - wasNew=true  → row was inserted (this call is the first read).
//   - wasNew=false → row already existed (idempotent no-op); read.ReadAt
//     reflects the original timestamp loaded from DB.
//
// Caller must verify enrollment (siswa enrolled in materi.kelas_id) — repo
// does not check authorization.
func (r *Repo) MarkRead(ctx context.Context, materiID, siswaID uuid.UUID) (*Read, bool, error) {
	row := Read{MateriID: materiID, SiswaID: siswaID}
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected == 1 {
		// Insert succeeded — refetch to capture DB-assigned read_at default.
		if err := r.db.WithContext(ctx).
			Where("materi_id = ? AND siswa_id = ?", materiID, siswaID).
			First(&row).Error; err != nil {
			return nil, false, err
		}
		return &row, true, nil
	}
	// Conflict: row already existed. Load the original.
	if err := r.db.WithContext(ctx).
		Where("materi_id = ? AND siswa_id = ?", materiID, siswaID).
		First(&row).Error; err != nil {
		return nil, false, err
	}
	return &row, false, nil
}

// CountReadByBabSiswa returns the number of materi in the given bab that
// the siswa has marked as read. Used to compute progress per bab
// Fase-3-partial: materi_dibaca / total_materi (locked #68).
func (r *Repo) CountReadByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Table("materi_read AS mr").
		Joins("JOIN materi AS m ON m.id = mr.materi_id").
		Where("m.bab_id = ? AND mr.siswa_id = ?", babID, siswaID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountByBab returns the total number of materi rows attached to a bab.
// Companion to CountReadByBabSiswa for progress denominator.
func (r *Repo) CountByBab(ctx context.Context, babID uuid.UUID) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&Materi{}).
		Where("bab_id = ?", babID).
		Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// CountByBabBatch returns total materi count per bab id, in a single query.
// Pre-allocates the result map and zero-fills for ids that have no materi
// (`SELECT COUNT(*) GROUP BY bab_id` only returns rows for non-zero groups).
//
// Used by the siswa bab list endpoint (Task 3.E.1) — denominator for the
// progress fase-3-partial formula. Avoid N+1 by passing all bab ids at once.
func (r *Repo) CountByBabBatch(ctx context.Context, babIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(babIDs))
	for _, id := range babIDs {
		out[id] = 0
	}
	if len(babIDs) == 0 {
		return out, nil
	}
	type row struct {
		BabID uuid.UUID `gorm:"column:bab_id"`
		N     int64     `gorm:"column:n"`
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("materi").
		Select("bab_id, COUNT(*) AS n").
		Where("bab_id IN ?", babIDs).
		Group("bab_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.BabID] = r.N
	}
	return out, nil
}

// CountReadByBabBatch returns the number of materi the given siswa has
// marked as read, grouped by bab id, in a single query. Zero-fills missing
// bab ids in the result map (`GROUP BY` doesn't emit zero rows).
//
// Used by the siswa bab list endpoint (Task 3.E.1) — numerator for progress.
func (r *Repo) CountReadByBabBatch(ctx context.Context, babIDs []uuid.UUID, siswaID uuid.UUID) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(babIDs))
	for _, id := range babIDs {
		out[id] = 0
	}
	if len(babIDs) == 0 {
		return out, nil
	}
	type row struct {
		BabID uuid.UUID `gorm:"column:bab_id"`
		N     int64     `gorm:"column:n"`
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("materi_read AS mr").
		Joins("JOIN materi AS m ON m.id = mr.materi_id").
		Select("m.bab_id AS bab_id, COUNT(*) AS n").
		Where("m.bab_id IN ? AND mr.siswa_id = ?", babIDs, siswaID).
		Group("m.bab_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.BabID] = r.N
	}
	return out, nil
}

// ListReadIDsByBabSiswa returns the materi ids in a bab that the siswa
// has marked as read. Used by the siswa bab detail endpoint (Task 3.E.1)
// to flag each materi card as read/unread without per-materi round trips.
func (r *Repo) ListReadIDsByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).
		Table("materi_read AS mr").
		Joins("JOIN materi AS m ON m.id = mr.materi_id").
		Where("m.bab_id = ? AND mr.siswa_id = ?", babID, siswaID).
		Pluck("mr.materi_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// DB returns the underlying gorm.DB so callers can run a transaction (e.g.
// reorder under tx, future bulk operations). Mirrors bab.Repo.DB pattern.
func (r *Repo) DB() *gorm.DB { return r.db }
