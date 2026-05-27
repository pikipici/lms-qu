package sekolah

import (
	"time"

	"github.com/google/uuid"
)

// Sekolah is a school master row managed by admin.
type Sekolah struct {
	ID                       uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Nama                     string    `gorm:"not null" json:"nama"`
	NPSN                     *string   `gorm:"uniqueIndex" json:"npsn,omitempty"`
	Alamat                   string    `json:"alamat"`
	SiswaRegistrationEnabled bool      `gorm:"not null;default:false" json:"siswa_registration_enabled"`
	SiswaRegistrationMode    string    `gorm:"not null;default:approval_required" json:"siswa_registration_mode"`
	JumlahKelas              int64     `gorm:"-" json:"jumlah_kelas"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (Sekolah) TableName() string { return "sekolah" }
