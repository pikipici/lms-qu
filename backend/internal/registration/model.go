package registration

import (
	"time"

	"github.com/google/uuid"
)

const (
	ModeAutoApprove      = "auto_approve"
	ModeApprovalRequired = "approval_required"

	RequestPending  = "pending"
	RequestApproved = "approved"
	RequestRejected = "rejected"
)

type JoinRequest struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SiswaID      uuid.UUID  `gorm:"type:uuid;not null" json:"siswa_id"`
	SiswaName    string     `gorm:"->;column:siswa_name" json:"siswa_name,omitempty"`
	Username     string     `gorm:"->;column:username" json:"username,omitempty"`
	SekolahID    uuid.UUID  `gorm:"type:uuid;not null" json:"sekolah_id"`
	SekolahNama  string     `gorm:"->;column:sekolah_nama" json:"sekolah_nama,omitempty"`
	KelasID      *uuid.UUID `gorm:"type:uuid" json:"kelas_id,omitempty"`
	KelasNama    string     `gorm:"->;column:kelas_nama" json:"kelas_nama,omitempty"`
	RombelID     *uuid.UUID `gorm:"type:uuid" json:"rombel_id,omitempty"`
	RombelNama   string     `gorm:"->;column:rombel_nama" json:"rombel_nama,omitempty"`
	Status       string     `gorm:"not null;default:pending" json:"status"`
	RequestedAt  time.Time  `gorm:"not null;default:now()" json:"requested_at"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	DecidedBy    *uuid.UUID `gorm:"type:uuid" json:"decided_by,omitempty"`
	RejectReason *string    `json:"reject_reason,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (JoinRequest) TableName() string { return "siswa_join_requests" }

type PublicSekolah struct {
	ID                       uuid.UUID `json:"id"`
	Nama                     string    `json:"nama"`
	SiswaRegistrationMode    string    `json:"siswa_registration_mode"`
	SiswaRegistrationEnabled bool      `json:"siswa_registration_enabled"`
}

type PublicKelas struct {
	ID   uuid.UUID `json:"id"`
	Nama string    `json:"nama"`
}
