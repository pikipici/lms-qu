// Repository for the kelas domain: kelas CRUD + enrollment membership.
package kelas

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo provides GORM-backed persistence for kelas + enrollment.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a kelas repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by UpdateBasic when the kelas exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("kelas: version conflict")

// Create inserts a new kelas. KodeInvite must already be generated + unique.
func (r *Repo) Create(ctx context.Context, k *Kelas) error {
	return r.db.WithContext(ctx).Create(k).Error
}

// FindByID returns a kelas by id.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Kelas, error) {
	var k Kelas
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// FindByKodeInvite returns a kelas by its 6-char invite code.
func (r *Repo) FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error) {
	var k Kelas
	if err := r.db.WithContext(ctx).Where("kode_invite = ?", kode).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// ListByGuru returns a page of kelas owned by a guru, plus the matching count.
// includeArchived=false filters out rows with archived_at IS NOT NULL.
func (r *Repo) ListByGuru(ctx context.Context, guruID uuid.UUID, sekolahID *uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	q := r.db.WithContext(ctx).Model(&Kelas{}).Where("kelas.guru_id = ?", guruID)
	if sekolahID != nil {
		q = q.Where("kelas.sekolah_id = ?", *sekolahID)
	}
	if !includeArchived {
		q = q.Where("kelas.archived_at IS NULL")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Kelas
	if err := q.
		Select("kelas.*, sekolah.nama AS sekolah_nama").
		Joins("LEFT JOIN sekolah ON sekolah.id = kelas.sekolah_id").
		Order("kelas.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if err := r.attachJumlahMurid(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListIDsByGuru returns the kelas IDs (active + archived) yang dimiliki
// guruID. Used by aggregate queries (pending counters, feed scope).
//
// Includes archived kelas — pending submissions di kelas archived tetap
// counted supaya guru ga lupa nilai pas pre-archive.
func (r *Repo) ListIDsByGuru(ctx context.Context, guruID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	if err := r.db.WithContext(ctx).
		Model(&Kelas{}).
		Where("guru_id = ?", guruID).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// ListAll returns a page of every kelas regardless of guru, plus the matching
// count. Used by admin scope.
func (r *Repo) ListAll(ctx context.Context, sekolahID *uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	q := r.db.WithContext(ctx).Model(&Kelas{})
	if sekolahID != nil {
		q = q.Where("kelas.sekolah_id = ?", *sekolahID)
	}
	if !includeArchived {
		q = q.Where("kelas.archived_at IS NULL")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Kelas
	if err := q.
		Select("kelas.*, sekolah.nama AS sekolah_nama").
		Joins("LEFT JOIN sekolah ON sekolah.id = kelas.sekolah_id").
		Order("kelas.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if err := r.attachJumlahMurid(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) attachJumlahMurid(ctx context.Context, rows []Kelas) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}

	type countRow struct {
		KelasID uuid.UUID
		Total   int64
	}
	var counts []countRow
	if err := r.db.WithContext(ctx).
		Model(&Enrollment{}).
		Select("kelas_id, COUNT(*) AS total").
		Where("kelas_id IN ? AND status = ?", ids, EnrollmentActive).
		Group("kelas_id").
		Scan(&counts).Error; err != nil {
		return err
	}

	byID := make(map[uuid.UUID]int64, len(counts))
	for _, c := range counts {
		byID[c.KelasID] = c.Total
	}
	for i := range rows {
		rows[i].JumlahMurid = byID[rows[i].ID]
	}
	return nil
}

// UpdateBasic applies an optimistic-concurrency update. The WHERE clause
// pins both id and the caller's expected version; on RowsAffected==0 we
// distinguish "row missing" (gorm.ErrRecordNotFound) from "version mismatch"
// (ErrVersionConflict) by re-reading the row.
func (r *Repo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, nama, deskripsi string, bobotSoalUlangan, bobotTugas int) error {
	res := r.db.WithContext(ctx).
		Model(&Kelas{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"nama":               nama,
			"deskripsi":          deskripsi,
			"bobot_soal_ulangan": bobotSoalUlangan,
			"bobot_tugas":        bobotTugas,
			"version":            gorm.Expr("version + 1"),
			"updated_at":         gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Kelas
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

// Archive soft-archives a kelas. No-op (returns gorm.ErrRecordNotFound) if
// the row is missing or already archived — caller can ignore that error if
// idempotency is desired.
func (r *Repo) Archive(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&Kelas{}).
		Where("id = ? AND archived_at IS NULL", id).
		UpdateColumns(map[string]any{
			"archived_at": gorm.Expr("now()"),
			"updated_at":  gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Unarchive clears archived_at. Returns gorm.ErrRecordNotFound if no row
// matched (id missing or already active).
func (r *Repo) Unarchive(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&Kelas{}).
		Where("id = ? AND archived_at IS NOT NULL", id).
		UpdateColumns(map[string]any{
			"archived_at": nil,
			"updated_at":  gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Enroll inserts an enrollment row using ON CONFLICT DO NOTHING. Returns
// inserted=true when the row was actually written, inserted=false when the
// pair already existed (idempotent join).
func (r *Repo) Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via JoinedVia) (bool, error) {
	row := Enrollment{
		KelasID:   kelasID,
		SiswaID:   siswaID,
		Status:    EnrollmentActive,
		JoinedVia: via,
	}
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "kelas_id"}, {Name: "siswa_id"}},
			DoNothing: true,
		}).
		Create(&row)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// FindEnrollment returns the enrollment row for a (kelas, siswa) pair.
func (r *Repo) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*Enrollment, error) {
	var e Enrollment
	if err := r.db.WithContext(ctx).
		Where("kelas_id = ? AND siswa_id = ?", kelasID, siswaID).
		First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// ListEnrollmentsByKelas returns a page of enrollments + count for a kelas.
func (r *Repo) ListEnrollmentsByKelas(ctx context.Context, kelasID uuid.UUID, limit, offset int) ([]Enrollment, int64, error) {
	q := r.db.WithContext(ctx).Model(&Enrollment{}).Where("kelas_id = ?", kelasID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Enrollment
	if err := q.Order("joined_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ListEnrollmentsBySiswa returns a page of enrollments + count for a siswa.
func (r *Repo) ListEnrollmentsBySiswa(ctx context.Context, siswaID uuid.UUID, limit, offset int) ([]Enrollment, int64, error) {
	q := r.db.WithContext(ctx).Model(&Enrollment{}).Where("siswa_id = ?", siswaID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Enrollment
	if err := q.Order("joined_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// RemoveEnrollment soft-removes a membership by flipping status. Hard delete
// would lose audit trail (#54-adjacent retention reasoning).
func (r *Repo) RemoveEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&Enrollment{}).
		Where("kelas_id = ? AND siswa_id = ? AND status = ?", kelasID, siswaID, EnrollmentActive).
		UpdateColumn("status", EnrollmentRemoved)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
