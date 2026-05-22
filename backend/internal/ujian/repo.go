// Repository for the ujian (Ujian + UjianSoal + HasilUjian +
// JawabanUjian + EventUjian) domain.
//
// Fase 6.A foundation skeleton: persistence helpers (Create / FindByID /
// list) implemented up-front because they have no business decisions.
// Methods that require business logic (CRUD validation, attempt
// lifecycle, advisory lock, auto-grade tx, remedial reset) return
// errNotImplemented and will be filled in by tasks 6.C-6.E.
package ujian

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for the ujian domain tables.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a ujian repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB so service-layer transactions can
// share the same connection (mirror soalbab.Repo.DB pattern).
func (r *Repo) DB() *gorm.DB { return r.db }

// ErrVersionConflict is returned by mutating ops when the row exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("ujian: version conflict")

// ErrActiveAttempts signals an Ujian edit / source change is rejected
// because at least one HasilUjian still has Status='berlangsung' &
// DeletedAt IS NULL.
var ErrActiveAttempts = errors.New("ujian: active attempts")

// errNotImplemented marks repo methods whose bodies will be added in
// later 6.C-6.E tasks. Returning a typed sentinel keeps server boot +
// build green during the foundation pass.
var errNotImplemented = errors.New("ujian: not implemented (foundation skeleton)")

// ---------------------------------------------------------------------------
// Ujian persistence
// ---------------------------------------------------------------------------

// CreateUjian inserts a new ujian row.
func (r *Repo) CreateUjian(ctx context.Context, u *Ujian) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// FindUjianByID returns an ujian by id.
func (r *Repo) FindUjianByID(ctx context.Context, id uuid.UUID) (*Ujian, error) {
	var u Ujian
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UjianListFilter narrows ListByKelas / ListByGuru results.
type UjianListFilter struct {
	Status Status // optional pin
	Limit  int    // <=0 → no cap
	Offset int    // skip first N
}

// ListByKelas returns ujian in a kelas, ordered by created_at DESC.
func (r *Repo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f UjianListFilter) ([]Ujian, error) {
	q := r.db.WithContext(ctx).Model(&Ujian{}).Where("kelas_id = ?", kelasID)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	q = q.Order("created_at DESC, id DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	var rows []Ujian
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateUjianBasic applies an optimistic-versioned partial update.
// `fields` is a column→value map; `version` increments + `updated_at`
// is bumped server-side.
//
// Returns gorm.ErrRecordNotFound when row is gone, ErrVersionConflict
// when expectedVersion mismatches the live row.
func (r *Repo) UpdateUjianBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	// Defensive: never let caller stomp on version directly.
	delete(fields, "version")
	delete(fields, "id")
	fields["version"] = gorm.Expr("version + 1")
	fields["updated_at"] = time.Now()

	res := r.db.WithContext(ctx).Model(&Ujian{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var u Ujian
		if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// DeleteUjian hard-deletes an ujian row. Caller MUST verify
// HasilUjian is empty / archive-only before calling — repo just
// enforces optimistic version. Cascade ke UjianSoal via FK ON DELETE
// CASCADE (migration 000011).
func (r *Repo) DeleteUjian(ctx context.Context, id uuid.UUID, expectedVersion int) error {
	res := r.db.WithContext(ctx).
		Where("id = ? AND version = ?", id, expectedVersion).
		Delete(&Ujian{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var u Ujian
		if err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// CountHasilByUjian returns the number of HasilUjian rows attached to
// an ujian (any status, including soft-deleted). Used by Delete to
// reject when attempts exist.
func (r *Repo) CountHasilByUjian(ctx context.Context, ujianID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&HasilUjian{}).
		Where("ujian_id = ?", ujianID).
		Count(&n).Error
	return n, err
}

// HasActiveAttempts reports whether ujianID still has any HasilUjian
// row dengan Status='berlangsung' AND DeletedAt IS NULL.
func (r *Repo) HasActiveAttempts(ctx context.Context, ujianID uuid.UUID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&HasilUjian{}).
		Where("ujian_id = ? AND status = ? AND deleted_at IS NULL", ujianID, HasilBerlangsung).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// ---------------------------------------------------------------------------
// UjianSoal junction (manual mode soal cache)
// ---------------------------------------------------------------------------

// SetUjianSoalIDs replaces the manual-mode soal_ids list for an ujian
// in a single transaction (DELETE existing + bulk INSERT new). Caller
// passes ordered slice; index → Urutan column.
//
// Empty slice clears the junction (used when guru switches dari manual
// ke random mode).
func (r *Repo) SetUjianSoalIDs(ctx context.Context, ujianID uuid.UUID, soalIDs []uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("ujian_id = ?", ujianID).Delete(&UjianSoal{}).Error; err != nil {
			return err
		}
		if len(soalIDs) == 0 {
			return nil
		}
		rows := make([]UjianSoal, 0, len(soalIDs))
		for i, sid := range soalIDs {
			rows = append(rows, UjianSoal{
				UjianID: ujianID,
				SoalID:  sid,
				Urutan:  int16(i),
			})
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

// ListUjianSoalIDs returns ordered soal_ids in the manual junction.
func (r *Repo) ListUjianSoalIDs(ctx context.Context, ujianID uuid.UUID) ([]uuid.UUID, error) {
	var rows []UjianSoal
	if err := r.db.WithContext(ctx).
		Where("ujian_id = ?", ujianID).
		Order("urutan ASC, soal_id ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.SoalID)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// HasilUjian persistence
// ---------------------------------------------------------------------------

// CreateHasil inserts a new hasil_ujian attempt row.
func (r *Repo) CreateHasil(ctx context.Context, h *HasilUjian) error {
	return r.db.WithContext(ctx).Create(h).Error
}

// FindHasilByID returns a hasil_ujian (including deleted, caller checks).
func (r *Repo) FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilUjian, error) {
	var h HasilUjian
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&h).Error; err != nil {
		return nil, err
	}
	return &h, nil
}

// FindActiveHasil returns the active (Status='berlangsung', DeletedAt
// IS NULL) attempt for a (ujian, siswa) pair, if any. Returns
// gorm.ErrRecordNotFound if none.
func (r *Repo) FindActiveHasil(ctx context.Context, ujianID, siswaID uuid.UUID) (*HasilUjian, error) {
	var h HasilUjian
	err := r.db.WithContext(ctx).
		Where("ujian_id = ? AND siswa_id = ? AND status = ? AND deleted_at IS NULL",
			ujianID, siswaID, HasilBerlangsung).
		First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// CountAttempts returns total non-deleted attempts (any status) by
// (ujian, siswa). Used for batas_attempt enforcement when remedial
// reset hasn't yet soft-deleted older rows.
func (r *Repo) CountAttempts(ctx context.Context, ujianID, siswaID uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&HasilUjian{}).
		Where("ujian_id = ? AND siswa_id = ? AND deleted_at IS NULL", ujianID, siswaID).
		Count(&n).Error
	return n, err
}

// ListHasilBySiswa returns all attempts a siswa has under an ujian
// (including soft-deleted? No — visible history only, deleted_at IS
// NULL). Ordered by mulai_at DESC.
func (r *Repo) ListHasilBySiswa(ctx context.Context, ujianID, siswaID uuid.UUID) ([]HasilUjian, error) {
	var rows []HasilUjian
	err := r.db.WithContext(ctx).
		Where("ujian_id = ? AND siswa_id = ? AND deleted_at IS NULL", ujianID, siswaID).
		Order("mulai_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

// ListHasilBySiswaAll returns all attempts a siswa has across ALL ujians
// dalam kelas tertentu (cross-ujian per kelas). Used by 6.E.1 siswa
// list endpoint untuk lobby/history per kelas. Ordered by mulai_at DESC.
func (r *Repo) ListHasilBySiswaAllKelas(ctx context.Context, kelasID, siswaID uuid.UUID) ([]HasilUjian, error) {
	var rows []HasilUjian
	err := r.db.WithContext(ctx).
		Joins("JOIN ujian u ON u.id = hasil_ujian.ujian_id").
		Where("u.kelas_id = ? AND hasil_ujian.siswa_id = ? AND hasil_ujian.deleted_at IS NULL",
			kelasID, siswaID).
		Order("hasil_ujian.mulai_at DESC, hasil_ujian.id DESC").
		Find(&rows).Error
	return rows, err
}

// ListHasilByUjian returns all attempts under a specific ujian (across
// all siswa in kelas). Used by 6.E.1 guru rekap dashboard. Ordered by
// (siswa_id, mulai_at DESC) for stable per-siswa grouping.
func (r *Repo) ListHasilByUjian(ctx context.Context, ujianID uuid.UUID) ([]HasilUjian, error) {
	var rows []HasilUjian
	err := r.db.WithContext(ctx).
		Where("ujian_id = ? AND deleted_at IS NULL", ujianID).
		Order("siswa_id, mulai_at DESC, id DESC").
		Find(&rows).Error
	return rows, err
}

// UpdateHasilStatus mutates a hasil row's lifecycle fields. Used by
// 6.E.1 Cancel (soft-cancel set Status='dibatalkan' + DeletedAt). For
// other transitions (submit, auto-grade) callers write inline UPDATE
// inside their tx.
func (r *Repo) UpdateHasilStatus(ctx context.Context, hasilID uuid.UUID, status HasilStatus, selesaiAt *time.Time, deletedAt *time.Time) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": gorm.Expr("now()"),
	}
	if selesaiAt != nil {
		updates["selesai_at"] = *selesaiAt
	}
	if deletedAt != nil {
		updates["deleted_at"] = *deletedAt
	}
	res := r.db.WithContext(ctx).Model(&HasilUjian{}).
		Where("id = ?", hasilID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ScanExpiredHasilIDs returns IDs of HasilUjian rows whose status is
// 'berlangsung' and deadline_at <= now, capped to limit. Used by
// timer_cron sweep (locked #87). Caller takes per-row advisory lock
// inside its tx.
func (r *Repo) ScanExpiredHasilIDs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	var ids []uuid.UUID
	err := r.db.WithContext(ctx).
		Model(&HasilUjian{}).
		Where("status = ? AND deadline_at IS NOT NULL AND deadline_at <= ? AND deleted_at IS NULL",
			HasilBerlangsung, now).
		Order("deadline_at ASC, id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

// ---------------------------------------------------------------------------
// JawabanUjian persistence
// ---------------------------------------------------------------------------

// UpsertJawaban inserts or updates a jawaban_ujian row keyed on
// (hasil_id, soal_id). Used by 6.D.2 answer save (delayed grade —
// is_benar=NULL, poin_dapat=0 sampai submit/cron locked #87).
func (r *Repo) UpsertJawaban(ctx context.Context, j *JawabanUjian) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO jawaban_ujian (id, hasil_id, soal_id, jawaban, is_benar, poin_dapat, answered_at)
		VALUES (gen_random_uuid(), ?, ?, ?, ?, ?, ?)
		ON CONFLICT (hasil_id, soal_id) DO UPDATE SET
			jawaban = EXCLUDED.jawaban,
			is_benar = EXCLUDED.is_benar,
			poin_dapat = EXCLUDED.poin_dapat,
			answered_at = EXCLUDED.answered_at
	`, j.HasilID, j.SoalID, j.Jawaban, j.IsBenar, j.PoinDapat, j.AnsweredAt).Error
}

// ListJawabanByHasil loads all jawaban rows for an attempt — used
// during submit grading + review render.
func (r *Repo) ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanUjian, error) {
	var rows []JawabanUjian
	err := r.db.WithContext(ctx).
		Where("hasil_id = ?", hasilID).
		Find(&rows).Error
	return rows, err
}

// ---------------------------------------------------------------------------
// EventUjian persistence
// ---------------------------------------------------------------------------

// AppendEvent inserts an event_ujian audit row. Best-effort: caller
// usually wraps in fire-and-forget goroutine to avoid blocking siswa
// flow on audit insert latency.
func (r *Repo) AppendEvent(ctx context.Context, e *EventUjian) error {
	return r.db.WithContext(ctx).Create(e).Error
}
