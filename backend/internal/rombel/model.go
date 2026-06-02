package rombel

import (
	"time"

	"github.com/google/uuid"
)

type Rombel struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SekolahID   uuid.UUID  `gorm:"type:uuid;not null" json:"sekolah_id"`
	Nama        string     `gorm:"not null" json:"nama"`
	Deskripsi   string     `gorm:"not null;default:''" json:"deskripsi"`
	Active      bool       `gorm:"not null;default:true" json:"active"`
	Version     int        `gorm:"not null;default:1" json:"version"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	JumlahSiswa int64      `gorm:"->;column:jumlah_siswa" json:"jumlah_siswa,omitempty"`
}

func (Rombel) TableName() string { return "rombels" }

type Membership struct {
	RombelID  uuid.UUID  `gorm:"type:uuid;primaryKey" json:"rombel_id"`
	SekolahID uuid.UUID  `gorm:"type:uuid;not null" json:"sekolah_id"`
	SiswaID   uuid.UUID  `gorm:"type:uuid;primaryKey" json:"siswa_id"`
	Status    string     `gorm:"not null;default:active" json:"status"`
	JoinedVia string     `gorm:"not null;default:self_registration" json:"joined_via"`
	JoinedAt  time.Time  `gorm:"not null;default:now()" json:"joined_at"`
	RemovedAt *time.Time `json:"removed_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Membership) TableName() string { return "rombel_memberships" }

type Member struct {
	SiswaID   uuid.UUID `json:"siswa_id"`
	Nama      string    `json:"nama"`
	Email     string    `json:"email"`
	RombelID  uuid.UUID `json:"rombel_id"`
	JoinedVia string    `json:"joined_via"`
	JoinedAt  time.Time `json:"joined_at"`
}
