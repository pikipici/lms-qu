// Repository for the importjob domain.
package importjob

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repo provides GORM-backed persistence for import_jobs.
type Repo struct {
	db *gorm.DB
}

// NewRepo creates an importjob repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// Create inserts a new import job row.
func (r *Repo) Create(ctx context.Context, j *ImportJob) error {
	return r.db.WithContext(ctx).Create(j).Error
}

// FindByID returns an import job by id.
func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*ImportJob, error) {
	var j ImportJob
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&j).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

// FindByIDForAdmin returns an import job scoped to a single admin owner.
// Used so admins can only see/confirm jobs they uploaded.
func (r *Repo) FindByIDForAdmin(ctx context.Context, id, adminID uuid.UUID) (*ImportJob, error) {
	var j ImportJob
	if err := r.db.WithContext(ctx).
		Where("id = ? AND admin_id = ?", id, adminID).
		First(&j).Error; err != nil {
		return nil, err
	}
	return &j, nil
}

// ListByAdmin returns a page of jobs + count owned by an admin, newest first.
func (r *Repo) ListByAdmin(ctx context.Context, adminID uuid.UUID, limit, offset int) ([]ImportJob, int64, error) {
	q := r.db.WithContext(ctx).Model(&ImportJob{}).Where("admin_id = ?", adminID)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []ImportJob
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SetStatus updates the lifecycle marker plus optional confirmed/completed
// timestamps. Pass nil for a timestamp arg to leave that column unchanged.
func (r *Repo) SetStatus(ctx context.Context, id uuid.UUID, status Status, confirmedAt, completedAt *time.Time) error {
	updates := map[string]any{
		"status": status,
	}
	if confirmedAt != nil {
		updates["confirmed_at"] = *confirmedAt
	}
	if completedAt != nil {
		updates["completed_at"] = *completedAt
	}
	return r.db.WithContext(ctx).
		Model(&ImportJob{}).
		Where("id = ?", id).
		UpdateColumns(updates).Error
}

// SetCounts updates the success/fail counters after processing.
func (r *Repo) SetCounts(ctx context.Context, id uuid.UUID, success, fail int) error {
	return r.db.WithContext(ctx).
		Model(&ImportJob{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"success_count": success,
			"fail_count":    fail,
		}).Error
}

// SetCredentialsPath stores the on-disk path of the generated credentials.csv.
func (r *Repo) SetCredentialsPath(ctx context.Context, id uuid.UUID, path string) error {
	return r.db.WithContext(ctx).
		Model(&ImportJob{}).
		Where("id = ?", id).
		UpdateColumn("credentials_csv", path).Error
}

// SetErrorsJSON stores a JSONB error blob (raw bytes).
func (r *Repo) SetErrorsJSON(ctx context.Context, id uuid.UUID, errorsJSON []byte) error {
	return r.db.WithContext(ctx).
		Model(&ImportJob{}).
		Where("id = ?", id).
		UpdateColumn("errors_json", datatypes.JSON(errorsJSON)).Error
}

// ExpireCredentialsBefore returns every completed job whose CompletedAt is
// older than cutoff AND still has a credentials_csv key set, then nulls out
// the credentials_csv column for those rows in a single transaction. The
// returned slice contains the rows BEFORE the null-out, so the caller can
// best-effort DeleteObject the R2 credentials.csv blob (Task 2.D.6 cleanup).
//
// Status stays at completed — we only drop the download handle. Audit
// queries that filter on status='completed' still see these rows; only
// DownloadCredentials starts returning ErrCredentialsMissing once the
// credentials_csv pointer is gone.
func (r *Repo) ExpireCredentialsBefore(ctx context.Context, cutoff time.Time) ([]ImportJob, error) {
	var expired []ImportJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []ImportJob
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND credentials_csv IS NOT NULL AND completed_at IS NOT NULL AND completed_at < ?",
				StatusCompleted, cutoff).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if err := tx.
			Model(&ImportJob{}).
			Where("id IN ?", ids).
			UpdateColumn("credentials_csv", gorm.Expr("NULL")).Error; err != nil {
			return err
		}

		expired = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return expired, nil
}

// ExpirePreviewBefore flips every preview job whose ExpiresAt < cutoff to
// status='expired' and returns the rows that were expired (so the caller can
// delete their on-disk CSV). Runs in a single transaction with a row-level
// lock on the candidate set.
func (r *Repo) ExpirePreviewBefore(ctx context.Context, cutoff time.Time) ([]ImportJob, error) {
	var expired []ImportJob
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []ImportJob
		if err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND expires_at < ?", StatusPreview, cutoff).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		ids := make([]uuid.UUID, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.ID)
		}
		if err := tx.
			Model(&ImportJob{}).
			Where("id IN ?", ids).
			UpdateColumn("status", StatusExpired).Error; err != nil {
			return err
		}

		expired = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return expired, nil
}
