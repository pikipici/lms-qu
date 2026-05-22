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

// BulkCreateSoal inserts multiple bank_soal rows in a single transaction.
// Returns the number of inserted rows. All-or-nothing — kalau salah satu
// row gagal Create, tx rollback dan return error.
//
// Caller (Service.BulkCreate) is responsible for parse + per-line
// validation; this method assumes input rows already pass validation.
// Mirror soalbab.Repo.BulkCreateSoal pattern (locked Task 5.B.3).
func (r *Repo) BulkCreateSoal(ctx context.Context, soals []BankSoal) (int, error) {
	if len(soals) == 0 {
		return 0, nil
	}
	var inserted int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use CreateInBatches to avoid a single huge INSERT that blows
		// past Postgres' parameter limit (~65k). 50 row × ~20 cols = 1000
		// params per batch — comfortable headroom.
		const batchSize = 50
		res := tx.CreateInBatches(soals, batchSize)
		if res.Error != nil {
			return res.Error
		}
		inserted = int(res.RowsAffected)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return inserted, nil
}

// UpdateSoalBasic applies a partial PATCH to bank_soal with optimistic
// concurrency (#56). Mirror soalbab.Repo.UpdateSoalBasic semantics:
// caller supplies resolved fields; repo bumps version + updated_at.
// Returns ErrVersionConflict when row exists but version drifted,
// gorm.ErrRecordNotFound if row deleted/never existed.
func (r *Repo) UpdateSoalBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	fields["version"] = gorm.Expr("version + 1")
	fields["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).
		Model(&BankSoal{}).
		Where("id = ? AND version = ? AND deleted_at IS NULL", id, expectedVersion).
		UpdateColumns(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe BankSoal
		if err := r.db.WithContext(ctx).
			Select("id", "version", "deleted_at").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		if probe.DeletedAt != nil {
			return gorm.ErrRecordNotFound
		}
		return ErrVersionConflict
	}
	return nil
}

// UpdateSoalImageSlot atomically swaps a single image-key column on
// the bank_soal row. Returns the OLD object key (before the swap) so
// the caller can dispatch a compensating R2 delete (locked #69). When
// the column was already nil, returned *string is nil.
//
// `column` MUST be one of: pertanyaan_object_key, opsi_a_object_key,
// opsi_b_object_key, opsi_c_object_key, opsi_d_object_key,
// opsi_e_object_key. Caller validates this — repo trusts input.
//
// Image swap does NOT bump version — keeping image swap orthogonal to
// content edits supaya guru bisa fix typo gambar tanpa invalidasi tab
// editor lain (locked #56 explicit applies to text edits, gambar is
// idempotent set/clear). Mirror soalbab.UpdateSoalImageSlot.
func (r *Repo) UpdateSoalImageSlot(ctx context.Context, id uuid.UUID, column string, newKey *string) (*string, error) {
	switch column {
	case "pertanyaan_object_key",
		"opsi_a_object_key", "opsi_b_object_key",
		"opsi_c_object_key", "opsi_d_object_key", "opsi_e_object_key":
		// allowed
	default:
		return nil, errors.New("banksoal: invalid image column")
	}
	var old *string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var probe BankSoal
		if err := tx.Select("id", column, "deleted_at").
			Where("id = ?", id).First(&probe).Error; err != nil {
			return err
		}
		if probe.DeletedAt != nil {
			return gorm.ErrRecordNotFound
		}
		switch column {
		case "pertanyaan_object_key":
			old = probe.PertanyaanObjectKey
		case "opsi_a_object_key":
			old = probe.OpsiAObjectKey
		case "opsi_b_object_key":
			old = probe.OpsiBObjectKey
		case "opsi_c_object_key":
			old = probe.OpsiCObjectKey
		case "opsi_d_object_key":
			old = probe.OpsiDObjectKey
		case "opsi_e_object_key":
			old = probe.OpsiEObjectKey
		}
		updates := map[string]interface{}{
			column:       newKey,
			"updated_at": gorm.Expr("now()"),
		}
		res := tx.Model(&BankSoal{}).
			Where("id = ?", id).
			UpdateColumns(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return old, nil
}

// SoftDeleteSoal marks deleted_at = now() and bumps version. Returns
// ErrVersionConflict if version drifted, gorm.ErrRecordNotFound if
// row already deleted or never existed.
func (r *Repo) SoftDeleteSoal(ctx context.Context, id uuid.UUID, expectedVersion int) error {
	res := r.db.WithContext(ctx).
		Model(&BankSoal{}).
		Where("id = ? AND version = ? AND deleted_at IS NULL", id, expectedVersion).
		UpdateColumns(map[string]interface{}{
			"deleted_at": gorm.Expr("now()"),
			"version":    gorm.Expr("version + 1"),
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe BankSoal
		if err := r.db.WithContext(ctx).
			Select("id", "version", "deleted_at").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		if probe.DeletedAt != nil {
			return gorm.ErrRecordNotFound
		}
		return ErrVersionConflict
	}
	return nil
}

// HardDeleteSoal removes the row entirely + returns image keys for R2
// compensating cleanup. Caller MUST ensure no HasilUjian.SoalIDsJSON
// still references the soal_id (locked #84 soft-delete fallback when
// references exist). Used by guru force-delete jalur khusus + admin
// purge cron (Fase 8).
func (r *Repo) HardDeleteSoal(ctx context.Context, id uuid.UUID) ([]string, error) {
	var keys []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var probe BankSoal
		if err := tx.Where("id = ?", id).First(&probe).Error; err != nil {
			return err
		}
		keys = collectImageKeys(&probe)
		return tx.Where("id = ?", id).Delete(&BankSoal{}).Error
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// collectImageKeys returns the non-nil R2 object keys held by a
// BankSoal — used for compensating delete on hard-delete.
func collectImageKeys(s *BankSoal) []string {
	if s == nil {
		return nil
	}
	keys := make([]string, 0, 6)
	for _, k := range []*string{
		s.PertanyaanObjectKey,
		s.OpsiAObjectKey, s.OpsiBObjectKey, s.OpsiCObjectKey,
		s.OpsiDObjectKey, s.OpsiEObjectKey,
	} {
		if k != nil && *k != "" {
			keys = append(keys, *k)
		}
	}
	return keys
}
