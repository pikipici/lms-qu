// Repository for the submission domain.
package submission

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo provides GORM-backed persistence for submission + submission_attachment.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates a submission repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// ErrVersionConflict is returned by mutating ops when the row exists but
// its current version differs from the caller's expected version (#56).
var ErrVersionConflict = errors.New("submission: version conflict")

// StatusFilter narrows ListByTugas results.
//
// nil = no status filter (return all submissions for tugas).
// non-nil = pin to exact status.
type StatusFilter struct {
	Status *Status
}

// DB returns the underlying gorm.DB so service layer bisa orchestrate
// multi-step transactions (submit/grade pakai SELECT FOR UPDATE per
// locked #73). Mirrors materi.Repo.DB / tugas.Repo.DB pattern.
func (r *Repo) DB() *gorm.DB { return r.db }

// LockForUpdate executes `SELECT ... FOR UPDATE` against submission
// scoped to (tugasID, siswaID). Returns the locked row, or
// gorm.ErrRecordNotFound kalau belum ada submission (caller-handled —
// service.Submit treat ini sebagai "first submit" path).
//
// CALLER MUST run inside a transaction (pass tx, not r.db). Lock
// released saat tx Commit/Rollback.
func (r *Repo) LockForUpdate(ctx context.Context, tx *gorm.DB, tugasID, siswaID uuid.UUID) (*Submission, error) {
	var s Submission
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tugas_id = ? AND siswa_id = ?", tugasID, siswaID).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// LockByID executes `SELECT ... FOR UPDATE` against submission scoped to
// id. Used by Grade endpoint flow (locked #73). CALLER MUST run inside tx.
func (r *Repo) LockByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*Submission, error) {
	var s Submission
	if err := tx.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", id).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserts a new submission. Caller is responsible for setting all
// required fields (TugasID, SiswaID, etc).
//
// CALLER MUST pass a tx already locked via LockForUpdate (or fresh on
// first submit). DB-level UNIQUE(tugas_id, siswa_id) enforces single-row.
func (r *Repo) Create(ctx context.Context, tx *gorm.DB, s *Submission) error {
	return tx.WithContext(ctx).Create(s).Error
}

// FindByID returns a submission by id with attachments preloaded.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Submission, error) {
	var s Submission
	if err := r.db.WithContext(ctx).
		Preload("Attachments").
		Where("id = ?", id).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByTugasSiswa returns the (single) submission for a tugas+siswa pair,
// with attachments preloaded. Returns gorm.ErrRecordNotFound kalau belum
// ada (caller treat sebagai "siswa belum submit").
func (r *Repo) FindByTugasSiswa(ctx context.Context, tugasID, siswaID uuid.UUID) (*Submission, error) {
	var s Submission
	if err := r.db.WithContext(ctx).
		Preload("Attachments").
		Where("tugas_id = ? AND siswa_id = ?", tugasID, siswaID).
		First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ListByTugas returns submissions for a tugas, ordered by submitted_at DESC.
// Used by guru rekap view (locked #73). Status filter optional.
//
// Attachments are NOT preloaded (list view typically only needs metadata
// + nilai; caller calls FindByID for detail).
func (r *Repo) ListByTugas(ctx context.Context, tugasID uuid.UUID, f StatusFilter) ([]Submission, error) {
	q := r.db.WithContext(ctx).Model(&Submission{}).Where("tugas_id = ?", tugasID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	q = q.Order("submitted_at DESC")

	var rows []Submission
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByTugas returns the number of submissions for a tugas matching filter.
func (r *Repo) CountByTugas(ctx context.Context, tugasID uuid.UUID, f StatusFilter) (int64, error) {
	q := r.db.WithContext(ctx).Model(&Submission{}).Where("tugas_id = ?", tugasID)
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListBySiswa returns submissions belonging to a siswa, ordered by
// submitted_at DESC. Tidak scope by kelas — caller jelaskan via tugas.kelas
// kalau perlu.
func (r *Repo) ListBySiswa(ctx context.Context, siswaID uuid.UUID, limit int) ([]Submission, error) {
	q := r.db.WithContext(ctx).Model(&Submission{}).Where("siswa_id = ?", siswaID)
	q = q.Order("submitted_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}

	var rows []Submission
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateOnResubmit applies an optimistic-concurrency patch on resubmit:
// overwrite catatan + is_late, bump version + submitted_at. Caller telah
// LockForUpdate + classified attachment swap. Used in Submit endpoint flow.
//
// Status reset ke 'submitted' (kalau sebelumnya 'returned' atau salah-lain).
// Grade fields ga di-clear di sini — Grade endpoint yang ngebatalin kalau
// perlu (defer MVP — kalau status='graded' Submit reject 409 di service).
func (r *Repo) UpdateOnResubmit(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, catatan string, isLate bool) error {
	res := tx.WithContext(ctx).
		Model(&Submission{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(map[string]any{
			"catatan":      catatan,
			"is_late":      isLate,
			"status":       StatusSubmitted,
			"version":      gorm.Expr("version + 1"),
			"submitted_at": gorm.Expr("now()"),
			"updated_at":   gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Submission
		if err := tx.WithContext(ctx).
			Select("id", "version").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// GradeUpdate applies grade fields to a submission with optimistic
// concurrency. Caller telah LockByID + cek Status='submitted'.
func (r *Repo) GradeUpdate(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["version"] = gorm.Expr("version + 1")
	fields["updated_at"] = gorm.Expr("now()")
	res := tx.WithContext(ctx).
		Model(&Submission{}).
		Where("id = ? AND version = ?", id, expectedVersion).
		UpdateColumns(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var probe Submission
		if err := tx.WithContext(ctx).
			Select("id", "version").
			Where("id = ?", id).
			First(&probe).Error; err != nil {
			return err
		}
		return ErrVersionConflict
	}
	return nil
}

// AddAttachment inserts a single submission_attachment row. Caller telah
// validate mime + size + count cap (locked #46/#72) + uploaded ke R2.
//
// CALLER MUST run inside the same tx as the parent submission Create/Update
// supaya rollback consistent.
func (r *Repo) AddAttachment(ctx context.Context, tx *gorm.DB, a *Attachment) error {
	return tx.WithContext(ctx).Create(a).Error
}

// FindAttachmentByID returns a single attachment by id, scoped to its
// parent submission via SubmissionID match (defensive — caller's URL must match).
func (r *Repo) FindAttachmentByID(ctx context.Context, submissionID, attachmentID uuid.UUID) (*Attachment, error) {
	var a Attachment
	if err := r.db.WithContext(ctx).
		Where("id = ? AND submission_id = ?", attachmentID, submissionID).
		First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAttachmentsBySubmission returns all attachments for a submission,
// ordered by created_at ASC (display order = upload order).
func (r *Repo) ListAttachmentsBySubmission(ctx context.Context, submissionID uuid.UUID) ([]Attachment, error) {
	var rows []Attachment
	if err := r.db.WithContext(ctx).
		Where("submission_id = ?", submissionID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// DeleteAttachmentsBySubmission hard-deletes all submission_attachment rows
// for a submission and returns their ObjectKeys for compensating R2 cleanup
// (locked #69 pattern). Used pada resubmit flow (locked #72) — replace
// seluruh attachment set dalam tx.
//
// CALLER MUST run inside tx + handle R2 DeleteObject post-commit.
func (r *Repo) DeleteAttachmentsBySubmission(ctx context.Context, tx *gorm.DB, submissionID uuid.UUID) ([]string, error) {
	var rows []Attachment
	if err := tx.WithContext(ctx).
		Select("id", "object_key").
		Where("submission_id = ?", submissionID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, a := range rows {
		keys = append(keys, a.ObjectKey)
	}
	if len(rows) == 0 {
		return keys, nil
	}
	res := tx.WithContext(ctx).
		Where("submission_id = ?", submissionID).
		Delete(&Attachment{})
	if res.Error != nil {
		return nil, res.Error
	}
	return keys, nil
}
