package rombel

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrVersionConflict = errors.New("rombel: version conflict")

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) ListBySekolah(ctx context.Context, sekolahID uuid.UUID, includeArchived bool, limit, offset int) ([]Rombel, int64, error) {
	q := r.db.WithContext(ctx).Table("rombels r").Where("r.sekolah_id = ?", sekolahID)
	if !includeArchived {
		q = q.Where("r.archived_at IS NULL")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]Rombel, 0)
	err := q.Select("r.*, COALESCE(COUNT(rm.siswa_id) FILTER (WHERE rm.status = 'active'), 0) AS jumlah_siswa").
		Joins("LEFT JOIN rombel_memberships rm ON rm.rombel_id = r.id").
		Group("r.id").Order("r.nama ASC").Limit(limit).Offset(offset).Scan(&rows).Error
	return rows, total, err
}

func (r *Repo) ListMembers(ctx context.Context, rombelID uuid.UUID) ([]Member, error) {
	rows := make([]Member, 0)
	err := r.db.WithContext(ctx).Table("rombel_memberships rm").
		Select("rm.siswa_id, u.name AS nama, u.email, rm.rombel_id, rm.joined_via, rm.joined_at").
		Joins("JOIN users u ON u.id = rm.siswa_id").
		Where("rm.rombel_id = ? AND rm.status = 'active'", rombelID).
		Order("u.name ASC, u.email ASC").Scan(&rows).Error
	return rows, err
}

func (r *Repo) ListPublicBySekolah(ctx context.Context, sekolahID uuid.UUID) ([]Rombel, error) {
	rows := make([]Rombel, 0)
	err := r.db.WithContext(ctx).Table("rombels r").
		Select("r.id, r.sekolah_id, r.nama, r.deskripsi, r.active, r.version, r.archived_at, r.created_at, r.updated_at").
		Joins("JOIN sekolah s ON s.id = r.sekolah_id").
		Where("r.sekolah_id = ? AND r.archived_at IS NULL AND r.active = true AND s.siswa_registration_enabled = true", sekolahID).
		Order("r.nama ASC").Scan(&rows).Error
	return rows, err
}

func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Rombel, error) {
	var row Rombel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repo) Create(ctx context.Context, row *Rombel) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repo) Update(ctx context.Context, id uuid.UUID, version int, nama, deskripsi string) (*Rombel, error) {
	res := r.db.WithContext(ctx).Model(&Rombel{}).Where("id = ? AND version = ? AND archived_at IS NULL", id, version).
		UpdateColumns(map[string]any{"nama": nama, "deskripsi": deskripsi, "version": gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrVersionConflict
	}
	return r.FindByID(ctx, id)
}

func (r *Repo) Archive(ctx context.Context, id uuid.UUID) (*Rombel, error) {
	res := r.db.WithContext(ctx).Model(&Rombel{}).Where("id = ? AND archived_at IS NULL", id).
		UpdateColumns(map[string]any{"archived_at": gorm.Expr("now()"), "active": false, "version": gorm.Expr("version + 1"), "updated_at": gorm.Expr("now()")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.FindByID(ctx, id)
}

func (r *Repo) DeleteIfEmpty(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Table("rombel_memberships").Where("rombel_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrNotEmpty
		}
		if err := tx.Table("siswa_join_requests").Where("rombel_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrNotEmpty
		}
		res := tx.Delete(&Rombel{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *Repo) UpsertMembership(ctx context.Context, rombelID, siswaID uuid.UUID, joinedVia string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.upsertMembershipTx(tx, rombelID, siswaID, joinedVia)
	})
}

func (r *Repo) MoveMembership(ctx context.Context, toRombelID, siswaID uuid.UUID, joinedVia string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.upsertMembershipTx(tx, toRombelID, siswaID, joinedVia)
	})
}

func (r *Repo) upsertMembershipTx(tx *gorm.DB, rombelID, siswaID uuid.UUID, joinedVia string) error {
	var rb Rombel
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND archived_at IS NULL AND active = true", rombelID).First(&rb).Error; err != nil {
		return err
	}
	var role string
	if err := tx.Table("users").Select("role").Where("id = ?", siswaID).Scan(&role).Error; err != nil {
		return err
	}
	if role != "siswa" {
		return ErrInvalidSiswa
	}
	if err := tx.Table("rombel_memberships").Where("sekolah_id = ? AND siswa_id = ? AND status = 'active' AND rombel_id <> ?", rb.SekolahID, siswaID, rombelID).
		Updates(map[string]any{"status": "removed", "removed_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")}).Error; err != nil {
		return err
	}
	m := &Membership{RombelID: rombelID, SekolahID: rb.SekolahID, SiswaID: siswaID, Status: "active", JoinedVia: joinedVia}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "rombel_id"}, {Name: "siswa_id"}},
		DoUpdates: clause.Assignments(map[string]any{"sekolah_id": rb.SekolahID, "status": "active", "joined_at": gorm.Expr("now()"), "joined_via": joinedVia, "removed_at": nil}),
	}).Create(m).Error
}

var ErrNotEmpty = errors.New("rombel: not empty")
