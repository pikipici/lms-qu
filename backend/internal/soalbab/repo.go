// Repository for the soalbab (Soal Bab + UlanganBabSetting + HasilSoalBab +
// JawabanBab + EventBab + SoalAssignment) domain.
//
// This is the Fase 5.A skeleton: method signatures only. Bodies that
// require business logic (CRUD, attempt lifecycle, etc.) return
// errNotImplemented and will be filled in by tasks 5.B-5.E. Pure
// persistence helpers (Create / FindByID / list) are implemented up-front
// because they have no business decisions to make.
package soalbab

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo provides GORM-backed persistence for soal_bab + ulangan_bab_setting +
// hasil_soal_bab + jawaban_bab + event_bab + soal_assignment tables.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a soalbab repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying *gorm.DB so service-layer transactions can
// share the same connection (mirror tugas.Repo.DB pattern).
func (r *Repo) DB() *gorm.DB { return r.db }

// ErrVersionConflict is returned by mutating ops when the row exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("soalbab: version conflict")

// errNotImplemented marks repo methods whose bodies will be added in
// later 5.B-5.E tasks. Returning a typed sentinel keeps server boot +
// build green during the foundation pass.
var errNotImplemented = errors.New("soalbab: not implemented (foundation skeleton)")

// ---------------------------------------------------------------------------
// SoalBab persistence
// ---------------------------------------------------------------------------

// SoalListFilter narrows ListSoalByBab results.
type SoalListFilter struct {
	// Mode, when non-empty, pins the soal mode (latihan/ulangan/keduanya).
	// Use ModeUlangan to fetch ulangan-eligible (mode IN ('ulangan',
	// 'keduanya')) — caller will widen via OR explicitly when needed.
	Mode Mode
	// Limit caps result count. <=0 → no cap.
	Limit int
}

// CreateSoal inserts a new soal_bab row.
func (r *Repo) CreateSoal(ctx context.Context, s *SoalBab) error {
	return r.db.WithContext(ctx).Create(s).Error
}

// FindSoalByID returns a soal_bab by id.
func (r *Repo) FindSoalByID(ctx context.Context, id uuid.UUID) (*SoalBab, error) {
	var s SoalBab
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSoalByBab returns soal_bab in a bab, ordered by urutan ASC, id ASC.
func (r *Repo) ListSoalByBab(ctx context.Context, babID uuid.UUID, f SoalListFilter) ([]SoalBab, error) {
	q := r.db.WithContext(ctx).Model(&SoalBab{}).Where("bab_id = ?", babID)
	switch f.Mode {
	case ModeLatihan:
		q = q.Where("mode IN ?", []Mode{ModeLatihan, ModeKeduanya})
	case ModeUlangan:
		q = q.Where("mode IN ?", []Mode{ModeUlangan, ModeKeduanya})
	case ModeKeduanya:
		// no narrow — return all
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	var soals []SoalBab
	if err := q.Order("urutan ASC, id ASC").Find(&soals).Error; err != nil {
		return nil, err
	}
	return soals, nil
}

// CountSoalByBabMode returns the count of soal eligible for a given mode
// (used to validate UlanganBabSetting.JumlahSoal upper bound).
//
// For mode='ulangan', count includes 'ulangan' + 'keduanya'. For
// mode='latihan', count includes 'latihan' + 'keduanya'.
func (r *Repo) CountSoalByBabMode(ctx context.Context, babID uuid.UUID, m Mode) (int64, error) {
	q := r.db.WithContext(ctx).Model(&SoalBab{}).Where("bab_id = ?", babID)
	switch m {
	case ModeLatihan:
		q = q.Where("mode IN ?", []Mode{ModeLatihan, ModeKeduanya})
	case ModeUlangan:
		q = q.Where("mode IN ?", []Mode{ModeUlangan, ModeKeduanya})
	default:
		// no narrow
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateSoalBasic applies a partial PATCH to soal_bab with optimistic
// concurrency (#56). Mirror tugas.Repo.UpdateBasic semantics: caller
// supplies resolved fields; repo bumps version + updated_at.
func (r *Repo) UpdateSoalBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	fields["version"] = gorm.Expr("version + 1")
	fields["updated_at"] = gorm.Expr("now()")
	res := r.db.WithContext(ctx).
		Model(&SoalBab{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe SoalBab
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

// DeleteSoal hard-deletes a soal_bab row and returns the list of object
// keys (any of pertanyaan + opsi a..e image slots) so callers can run a
// compensating R2 cleanup (locked #69).
//
// Returns gorm.ErrRecordNotFound when the row does not exist.
func (r *Repo) DeleteSoal(ctx context.Context, id uuid.UUID) ([]string, error) {
	var existing SoalBab
	if err := r.db.WithContext(ctx).
		Select("id",
			"pertanyaan_object_key",
			"opsi_a_object_key", "opsi_b_object_key",
			"opsi_c_object_key", "opsi_d_object_key",
			"opsi_e_object_key").
		Where("id = ?", id).
		First(&existing).Error; err != nil {
		return nil, err
	}
	keys := collectImageKeys(&existing)
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&SoalBab{})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return keys, nil
}

// collectImageKeys gathers non-nil object keys from all 6 image slots.
func collectImageKeys(s *SoalBab) []string {
	out := make([]string, 0, 6)
	for _, p := range []*string{
		s.PertanyaanObjectKey,
		s.OpsiAObjectKey, s.OpsiBObjectKey, s.OpsiCObjectKey,
		s.OpsiDObjectKey, s.OpsiEObjectKey,
	} {
		if p != nil && *p != "" {
			out = append(out, *p)
		}
	}
	return out
}

// BulkCreateSoal inserts multiple soal_bab rows in a single transaction.
// Returns the number of inserted rows. All-or-nothing — kalau salah satu
// row gagal Create, tx rollback dan return error.
//
// Caller (Service.BulkCreate) is responsible for parse + per-line
// validation; this method assumes input rows already pass validation.
func (r *Repo) BulkCreateSoal(ctx context.Context, soals []SoalBab) (int, error) {
	if len(soals) == 0 {
		return 0, nil
	}
	var inserted int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Use CreateInBatches to avoid a single huge INSERT that blows
		// past Postgres' parameter limit (~65k). 50 row × ~25 cols = 1250
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

// UpdateSoalImageSlot atomically swaps one image-slot column on a soal_bab
// row and returns the previous object key so the caller can compensating
// delete it from R2 (locked #69 + Task 5.B.2).
//
//   - column must be one of {pertanyaan,opsi_a..e}_object_key — caller
//     responsibility (handler maps from the validated ImageSlot).
//   - newKey == nil clears the slot (DELETE flow).
//   - newKey != nil writes the new key (UPLOAD flow). The caller is
//     responsible for already having uploaded the new key to R2 before
//     calling this; on row-not-found this method returns
//     gorm.ErrRecordNotFound and the caller MUST drop the new R2 object.
//
// Image swap does NOT bump version — keeping image swap orthogonal to
// content edits supaya guru bisa fix typo gambar tanpa invalidasi tab
// editor lain (locked #56 explicit applies to text edits, gambar is
// idempotent set/clear).
func (r *Repo) UpdateSoalImageSlot(ctx context.Context, id uuid.UUID, column string, newKey *string) (*string, error) {
	switch column {
	case "pertanyaan_object_key",
		"opsi_a_object_key", "opsi_b_object_key",
		"opsi_c_object_key", "opsi_d_object_key", "opsi_e_object_key":
		// allowed
	default:
		return nil, errors.New("soalbab: invalid image column")
	}

	var prevKey *string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing SoalBab
		if err := tx.Select("id", column).Where("id = ?", id).First(&existing).Error; err != nil {
			return err
		}
		// Capture old slot value before write.
		switch column {
		case "pertanyaan_object_key":
			prevKey = existing.PertanyaanObjectKey
		case "opsi_a_object_key":
			prevKey = existing.OpsiAObjectKey
		case "opsi_b_object_key":
			prevKey = existing.OpsiBObjectKey
		case "opsi_c_object_key":
			prevKey = existing.OpsiCObjectKey
		case "opsi_d_object_key":
			prevKey = existing.OpsiDObjectKey
		case "opsi_e_object_key":
			prevKey = existing.OpsiEObjectKey
		}
		updates := map[string]any{
			column:       newKey,
			"updated_at": gorm.Expr("now()"),
		}
		res := tx.Model(&SoalBab{}).Where("id = ?", id).UpdateColumns(updates)
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
	return prevKey, nil
}

// ---------------------------------------------------------------------------
// UlanganBabSetting persistence
// ---------------------------------------------------------------------------

// GetSettingByBab returns the UlanganBabSetting row for a bab (or
// gorm.ErrRecordNotFound if no setting exists yet).
func (r *Repo) GetSettingByBab(ctx context.Context, babID uuid.UUID) (*UlanganBabSetting, error) {
	var s UlanganBabSetting
	if err := r.db.WithContext(ctx).Where("bab_id = ?", babID).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertSetting inserts a new ulangan_bab_setting row when none exists for
// the bab, or updates the existing row with optimistic concurrency
// (locked #56). On update path, expectedVersion must match the current row
// version — caller pre-fetches via GetSettingByBab to learn it. expected=0
// signals insert path (caller asserts no row exists yet).
//
// Mutates s in place: ID + Version + CreatedAt + UpdatedAt are filled from
// the persisted row so caller can return the fresh payload directly.
//
// Returns ErrVersionConflict when the row exists but version mismatches.
// Returns gorm.ErrRecordNotFound only if a concurrent delete races — not
// expected in MVP since bab→setting is 1:1 with FK CASCADE.
func (r *Repo) UpsertSetting(ctx context.Context, s *UlanganBabSetting, expectedVersion int) error {
	if s == nil || s.BabID == uuid.Nil {
		return errors.New("soalbab: setting + bab_id required")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Try to read current row first to decide insert vs update.
		var current UlanganBabSetting
		err := tx.Where("bab_id = ?", s.BabID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Insert path. Caller may have passed expectedVersion=0 (no
			// prior row) or any other value; we ignore on insert because
			// optimistic concurrency only matters once a row exists.
			s.Version = 1
			if err := tx.Create(s).Error; err != nil {
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		// Update path — enforce optimistic concurrency.
		if expectedVersion <= 0 || expectedVersion != current.Version {
			return ErrVersionConflict
		}
		fields := map[string]any{
			"jumlah_soal":                   s.JumlahSoal,
			"durasi_menit":                  s.DurasiMenit,
			"batas_attempt":                 s.BatasAttempt,
			"izinkan_review_setelah_submit": s.IzinkanReviewSetelahSubmit,
			"waktu_buka_review":             s.WaktuBukaReview,
			"version":                       gorm.Expr("version + 1"),
			"updated_at":                    gorm.Expr("now()"),
		}
		res := tx.Model(&UlanganBabSetting{}).
			Where("id = ? AND version = ?", current.ID, expectedVersion).
			UpdateColumns(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrVersionConflict
		}
		// Refetch to populate caller's struct with persisted fields.
		var fresh UlanganBabSetting
		if err := tx.Where("id = ?", current.ID).First(&fresh).Error; err != nil {
			return err
		}
		*s = fresh
		return nil
	})
}

// ---------------------------------------------------------------------------
// HasilSoalBab persistence
// ---------------------------------------------------------------------------

// CreateHasil inserts a new hasil_soal_bab attempt row.
func (r *Repo) CreateHasil(ctx context.Context, h *HasilSoalBab) error {
	return r.db.WithContext(ctx).Create(h).Error
}

// FindHasilByID returns a hasil_soal_bab attempt by id.
func (r *Repo) FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilSoalBab, error) {
	var h HasilSoalBab
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&h).Error; err != nil {
		return nil, err
	}
	return &h, nil
}

// FindActiveHasil returns the in-progress hasil for a (bab, siswa, mode)
// triple. Returns gorm.ErrRecordNotFound when none active.
func (r *Repo) FindActiveHasil(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (*HasilSoalBab, error) {
	var h HasilSoalBab
	if err := r.db.WithContext(ctx).
		Where("bab_id = ? AND siswa_id = ? AND mode = ? AND status = ?", babID, siswaID, mode, HasilBerlangsung).
		Order("mulai_at DESC").
		First(&h).Error; err != nil {
		return nil, err
	}
	return &h, nil
}

// CountHasilByBabSiswa returns how many attempts exist for a (bab, siswa,
// mode), filtered to non-cancelled by default. Used for batas_attempt
// enforcement (locked #76: dibatalkan tidak count).
func (r *Repo) CountHasilByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&HasilSoalBab{}).
		Where("bab_id = ? AND siswa_id = ? AND mode = ? AND status <> ?", babID, siswaID, mode, HasilDibatalkan).
		Count(&n).Error
	return n, err
}

// ListExpiredHasil returns active ulangan attempts whose deadline has
// passed. Used by the timer-expire cron (locked #80). To be implemented
// by Task 5.D.4.
func (r *Repo) ListExpiredHasil(ctx context.Context, limit int) ([]HasilSoalBab, error) {
	return nil, errNotImplemented
}

// ListHasilByBab returns the rekap hasil for a bab (guru dashboard view).
// To be implemented by Task 5.E.1.
type HasilListFilter struct {
	Mode    HasilMode // empty = no narrow
	SiswaID uuid.UUID // uuid.Nil = no narrow
	Status  HasilStatus
	Limit   int
}

func (r *Repo) ListHasilByBab(ctx context.Context, babID uuid.UUID, f HasilListFilter) ([]HasilSoalBab, error) {
	q := r.db.WithContext(ctx).Model(&HasilSoalBab{}).Where("bab_id = ?", babID)
	if f.Mode != "" {
		q = q.Where("mode = ?", f.Mode)
	}
	if f.SiswaID != uuid.Nil {
		q = q.Where("siswa_id = ?", f.SiswaID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	var rows []HasilSoalBab
	if err := q.Order("siswa_id ASC, attempt_no ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListHasilBySiswaBab returns all attempts (Latihan + Ulangan, semua status)
// for a (siswa, bab) ordered by mulai_at DESC. Used by siswa-side list +
// resume hint endpoint (Task 5.E.1).
func (r *Repo) ListHasilBySiswaBab(ctx context.Context, siswaID, babID uuid.UUID) ([]HasilSoalBab, error) {
	var rows []HasilSoalBab
	if err := r.db.WithContext(ctx).
		Where("siswa_id = ? AND bab_id = ?", siswaID, babID).
		Order("mulai_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// JawabanBab persistence
// ---------------------------------------------------------------------------

// UpsertJawaban inserts or updates a JawabanBab row using ON CONFLICT
// (hasil_id, soal_id) DO UPDATE so autosave + re-answer overwrite is
// idempotent (Task 5.C.2 latihan + Task 5.D.2 ulangan).
//
// Caller pre-resolves IsBenar + PoinDapat (latihan: graded immediately;
// ulangan: pass IsBenar=nil + PoinDapat=0 — we default the column when
// updating). Jawaban field is required and non-nil on every call.
//
// AnsweredAt is forced to now() so updates re-stamp the timestamp,
// keeping the autosave audit trail honest.
func (r *Repo) UpsertJawaban(ctx context.Context, j *JawabanBab) error {
	if j == nil || j.HasilID == uuid.Nil || j.SoalID == uuid.Nil {
		return errors.New("soalbab: jawaban + hasil_id + soal_id required")
	}
	// GORM's OnConflict needs a known unique target. The migration has
	// UNIQUE (hasil_id, soal_id); use both columns explicitly.
	cols := []string{"jawaban", "is_benar", "poin_dapat", "answered_at"}
	return r.db.WithContext(ctx).
		Clauses(clausesOnConflictHasilSoal(cols)).
		Create(j).Error
}

// clausesOnConflictHasilSoal returns a GORM ON CONFLICT clause keyed on
// (hasil_id, soal_id) updating the supplied columns. Lives as helper to
// keep UpsertJawaban readable.
func clausesOnConflictHasilSoal(updateCols []string) clause.OnConflict {
	cols := make([]clause.Column, 0, 2)
	cols = append(cols, clause.Column{Name: "hasil_id"}, clause.Column{Name: "soal_id"})
	doUpdate := make([]string, len(updateCols))
	copy(doUpdate, updateCols)
	return clause.OnConflict{
		Columns:   cols,
		DoUpdates: clause.AssignmentColumns(doUpdate),
	}
}

// FindJawabanByHasilSoal returns a single jawaban row for (hasil, soal).
// Used by Latihan answer endpoint to compute is_benar after upsert.
func (r *Repo) FindJawabanByHasilSoal(ctx context.Context, hasilID, soalID uuid.UUID) (*JawabanBab, error) {
	var j JawabanBab
	if err := r.db.WithContext(ctx).
		Where("hasil_id = ? AND soal_id = ?", hasilID, soalID).
		First(&j).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

// UpdateHasilStatus transitions a hasil row to a new status with optional
// finishing fields (selesai_at, nilai_total, jawaban_benar_count,
// jawaban_total). Latihan finish: pass nilai/benar/total all-nil (we set
// selesai_at + status only). Ulangan submit (Task 5.D): pass full set.
//
// Returns gorm.ErrRecordNotFound when the row no longer exists or is
// already cancelled.
func (r *Repo) UpdateHasilStatus(ctx context.Context, hasilID uuid.UUID, status HasilStatus, selesaiAt *time.Time, nilaiTotal *float64, benar, total *int16) error {
	updates := map[string]any{
		"status":     status,
		"updated_at": gorm.Expr("now()"),
	}
	if selesaiAt != nil {
		updates["selesai_at"] = *selesaiAt
	}
	if nilaiTotal != nil {
		updates["nilai_total"] = *nilaiTotal
	}
	if benar != nil {
		updates["jawaban_benar_count"] = *benar
	}
	if total != nil {
		updates["jawaban_total"] = *total
	}
	res := r.db.WithContext(ctx).
		Model(&HasilSoalBab{}).
		Where("id = ?", hasilID).
		UpdateColumns(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ListSoalByIDs returns soal_bab rows for the given id list, in arbitrary
// order. Caller (latihan/ulangan) typically resorts to the snapshot id
// order. Empty input → empty slice.
func (r *Repo) ListSoalByIDs(ctx context.Context, ids []uuid.UUID) ([]SoalBab, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []SoalBab
	if err := r.db.WithContext(ctx).
		Where("id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpsertJawaban (legacy stub removed above) — placeholder kept so search
// for "Task 5.C.2" jumps to the implementation.
var _ = "task-5.C.2-jawaban-upsert"

// ListJawabanByHasil returns all jawaban rows for an attempt. Used by
// submit grading + review endpoints.
func (r *Repo) ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanBab, error) {
	var rows []JawabanBab
	if err := r.db.WithContext(ctx).
		Where("hasil_id = ?", hasilID).
		Order("answered_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// EventBab persistence (anti-cheat audit ledger)
// ---------------------------------------------------------------------------

// AppendEvent inserts a new event_bab audit row.
func (r *Repo) AppendEvent(ctx context.Context, e *EventBab) error {
	return r.db.WithContext(ctx).Create(e).Error
}

// ListEventsByHasil returns the chronological audit ledger for an
// attempt. Used by guru forensic view (out-of-scope MVP UI; data is
// recorded so it's available for v0.11+).
func (r *Repo) ListEventsByHasil(ctx context.Context, hasilID uuid.UUID) ([]EventBab, error) {
	var rows []EventBab
	if err := r.db.WithContext(ctx).
		Where("hasil_id = ?", hasilID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ---------------------------------------------------------------------------
// SoalAssignment persistence
// ---------------------------------------------------------------------------

// CreateAssignment inserts a new soal_assignment audit row (idempotent
// via UNIQUE source/target). To be implemented when copy-soal endpoint
// lands (defer Fase 5+).
func (r *Repo) CreateAssignment(ctx context.Context, a *SoalAssignment) error {
	return errNotImplemented
}

// FindAssignmentByPair returns an existing soal_assignment row for the
// (source, target) pair if any.
func (r *Repo) FindAssignmentByPair(ctx context.Context, sourceID, targetID uuid.UUID) (*SoalAssignment, error) {
	var a SoalAssignment
	if err := r.db.WithContext(ctx).
		Where("source_bab_id = ? AND target_bab_id = ?", sourceID, targetID).
		First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}
