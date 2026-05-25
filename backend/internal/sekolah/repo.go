package sekolah

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides GORM-backed persistence for sekolah master data.
type Repo struct {
	db *gorm.DB
}

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, row *Sekolah) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *Repo) FindByID(ctx context.Context, id uuid.UUID) (*Sekolah, error) {
	var row Sekolah
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *Repo) List(ctx context.Context, q string, limit, offset int) ([]Sekolah, int64, error) {
	db := r.db.WithContext(ctx).Model(&Sekolah{})
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + q + "%"
		db = db.Where("nama ILIKE ? OR npsn ILIKE ?", like, like)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var rows []Sekolah
	if err := db.Order("nama ASC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	if len(rows) > 0 {
		ids := make([]uuid.UUID, 0, len(rows))
		byID := make(map[uuid.UUID]*Sekolah, len(rows))
		for i := range rows {
			ids = append(ids, rows[i].ID)
			byID[rows[i].ID] = &rows[i]
		}
		var counts []struct {
			SekolahID uuid.UUID
			Total     int64
		}
		if err := r.db.WithContext(ctx).
			Table("kelas").
			Select("sekolah_id, COUNT(*) AS total").
			Where("sekolah_id IN ? AND archived_at IS NULL", ids).
			Group("sekolah_id").
			Scan(&counts).Error; err != nil {
			return nil, 0, err
		}
		for _, c := range counts {
			if row := byID[c.SekolahID]; row != nil {
				row.JumlahKelas = c.Total
			}
		}
	}
	return rows, total, nil
}

func (r *Repo) Update(ctx context.Context, id uuid.UUID, nama string, npsn *string, alamat string) (*Sekolah, error) {
	res := r.db.WithContext(ctx).
		Model(&Sekolah{}).
		Where("id = ?", id).
		Updates(map[string]any{"nama": nama, "npsn": npsn, "alamat": alamat})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return r.FindByID(ctx, id)
}

func (r *Repo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&Sekolah{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
