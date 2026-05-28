package registration

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ListPublicSekolah(ctx context.Context) ([]PublicSekolah, error) {
	rows := make([]PublicSekolah, 0)
	err := r.db.WithContext(ctx).
		Table("sekolah").
		Select("id, nama, siswa_registration_enabled, siswa_registration_mode").
		Where("siswa_registration_enabled = true").
		Order("nama ASC").
		Scan(&rows).Error
	return rows, err
}

func (r *Repo) ListPublicKelas(ctx context.Context, sekolahID uuid.UUID) ([]PublicKelas, error) {
	rows := make([]PublicKelas, 0)
	err := r.db.WithContext(ctx).
		Table("kelas k").
		Select("k.id, k.nama").
		Joins("JOIN sekolah s ON s.id = k.sekolah_id").
		Where("k.sekolah_id = ? AND k.archived_at IS NULL AND s.siswa_registration_enabled = true", sekolahID).
		Order("k.nama ASC").
		Scan(&rows).Error
	return rows, err
}

type sekolahRegistration struct {
	ID                       uuid.UUID
	Nama                     string
	SiswaRegistrationEnabled bool
	SiswaRegistrationMode    string
}

type kelasRegistration struct {
	ID        uuid.UUID
	Nama      string
	SekolahID uuid.UUID
}

func (r *Repo) RegisterSiswa(ctx context.Context, user *auth.User, sekolahID, kelasID uuid.UUID) (string, error) {
	return r.withTx(ctx, func(tx *gorm.DB) (string, error) {
		var s sekolahRegistration
		if err := tx.Table("sekolah").
			Select("id, nama, siswa_registration_enabled, siswa_registration_mode").
			Where("id = ?", sekolahID).
			First(&s).Error; err != nil {
			return "", err
		}
		if !s.SiswaRegistrationEnabled {
			return "", ErrRegistrationDisabled
		}

		var k kelasRegistration
		if err := tx.Table("kelas").
			Select("id, nama, sekolah_id").
			Where("id = ? AND archived_at IS NULL", kelasID).
			First(&k).Error; err != nil {
			return "", err
		}
		if k.SekolahID != sekolahID {
			return "", ErrKelasNotInSekolah
		}

		if err := tx.Create(user).Error; err != nil {
			return "", err
		}

		mode := s.SiswaRegistrationMode
		if mode == "" {
			mode = ModeApprovalRequired
		}

		req := &JoinRequest{ID: uuid.New(), SiswaID: user.ID, SekolahID: sekolahID, KelasID: kelasID, Status: RequestPending}
		if mode == ModeAutoApprove {
			now := time.Now()
			req.Status = RequestApproved
			req.DecidedAt = &now
			enrollment := &kelas.Enrollment{KelasID: kelasID, SiswaID: user.ID, Status: kelas.EnrollmentActive, JoinedAt: now, JoinedVia: kelas.JoinedViaKode}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "kelas_id"}, {Name: "siswa_id"}},
				DoUpdates: clause.Assignments(map[string]any{"status": kelas.EnrollmentActive, "joined_at": now, "joined_via": kelas.JoinedViaKode}),
			}).Create(enrollment).Error; err != nil {
				return "", err
			}
		}
		if err := tx.Create(req).Error; err != nil {
			return "", err
		}
		return mode, nil
	})
}

func (r *Repo) withTx(ctx context.Context, fn func(*gorm.DB) (string, error)) (string, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return "", tx.Error
	}
	mode, err := fn(tx)
	if err != nil {
		_ = tx.Rollback().Error
		return "", err
	}
	if err := tx.Commit().Error; err != nil {
		return "", err
	}
	return mode, nil
}

func (r *Repo) ListRequests(ctx context.Context, status string, guruID *uuid.UUID) ([]JoinRequest, error) {
	q := r.db.WithContext(ctx).
		Table("siswa_join_requests r").
		Select("r.*, u.name AS siswa_name, u.email AS username, s.nama AS sekolah_nama, k.nama AS kelas_nama").
		Joins("JOIN users u ON u.id = r.siswa_id").
		Joins("JOIN sekolah s ON s.id = r.sekolah_id").
		Joins("JOIN kelas k ON k.id = r.kelas_id")
	if status != "" {
		q = q.Where("r.status = ?", status)
	}
	if guruID != nil {
		q = q.Where("k.guru_id = ?", *guruID)
	}
	var rows []JoinRequest
	err := q.Order("r.requested_at DESC").Limit(100).Scan(&rows).Error
	return rows, err
}

func (r *Repo) Approve(ctx context.Context, requestID, actorID uuid.UUID, guruID *uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var req JoinRequest
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("siswa_join_requests").Where("id = ?", requestID)
		if err := q.First(&req).Error; err != nil {
			return err
		}
		if guruID != nil {
			var count int64
			if err := tx.Table("kelas").Where("id = ? AND guru_id = ?", req.KelasID, *guruID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrForbiddenScope
			}
		}
		if req.Status != RequestPending {
			return ErrRequestNotPending
		}
		now := time.Now()
		enrollment := &kelas.Enrollment{KelasID: req.KelasID, SiswaID: req.SiswaID, Status: kelas.EnrollmentActive, JoinedAt: now, JoinedVia: kelas.JoinedViaAdmin}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "kelas_id"}, {Name: "siswa_id"}},
			DoUpdates: clause.Assignments(map[string]any{"status": kelas.EnrollmentActive, "joined_at": now, "joined_via": kelas.JoinedViaAdmin}),
		}).Create(enrollment).Error; err != nil {
			return err
		}
		return tx.Model(&JoinRequest{}).Where("id = ?", requestID).Updates(map[string]any{"status": RequestApproved, "decided_at": now, "decided_by": actorID}).Error
	})
}

func (r *Repo) Reject(ctx context.Context, requestID, actorID uuid.UUID, reason string, guruID *uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var req JoinRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Table("siswa_join_requests").Where("id = ?", requestID).First(&req).Error; err != nil {
			return err
		}
		if guruID != nil {
			var count int64
			if err := tx.Table("kelas").Where("id = ? AND guru_id = ?", req.KelasID, *guruID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrForbiddenScope
			}
		}
		if req.Status != RequestPending {
			return ErrRequestNotPending
		}
		now := time.Now()
		updates := map[string]any{"status": RequestRejected, "decided_at": now, "decided_by": actorID}
		if reason != "" {
			updates["reject_reason"] = reason
		}
		return tx.Model(&JoinRequest{}).Where("id = ?", requestID).Updates(updates).Error
	})
}

var (
	ErrRegistrationDisabled = errors.New("registration disabled")
	ErrKelasNotInSekolah    = errors.New("kelas not in sekolah")
	ErrRequestNotPending    = errors.New("join request not pending")
	ErrForbiddenScope       = errors.New("forbidden kelas scope")
)
