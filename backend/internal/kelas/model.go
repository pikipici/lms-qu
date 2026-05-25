// Package kelas holds the kelas (class/course) and enrollment domain models.
package kelas

import (
	"time"

	"github.com/google/uuid"
)

// JoinedVia distinguishes how a siswa got enrolled into a kelas.
type JoinedVia string

const (
	// JoinedViaAdmin indicates the admin assigned the siswa directly.
	JoinedViaAdmin JoinedVia = "admin"
	// JoinedViaKode indicates the siswa joined via the kelas invite code.
	JoinedViaKode JoinedVia = "kode"
)

// EnrollmentStatus marks an enrollment row as active or soft-removed.
type EnrollmentStatus string

const (
	// EnrollmentActive marks an active membership.
	EnrollmentActive EnrollmentStatus = "active"
	// EnrollmentRemoved marks a soft-removed membership (audit trail kept).
	EnrollmentRemoved EnrollmentStatus = "removed"
)

// Kelas represents a class/course owned by a guru.
//
// Version guards optimistic concurrency on PATCH (#56). ArchivedAt is the
// soft-archive timestamp; a non-NULL value hides the kelas from active lists
// without deleting underlying rows.
type Kelas struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Nama             string     `gorm:"not null" json:"nama"`
	Deskripsi        string     `json:"deskripsi"`
	KodeInvite       string     `gorm:"uniqueIndex;not null" json:"kode_invite"`
	GuruID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"guru_id"`
	SekolahID        *uuid.UUID `gorm:"type:uuid;index" json:"sekolah_id,omitempty"`
	SekolahNama      string     `gorm:"->;column:sekolah_nama" json:"sekolah_nama,omitempty"`
	BobotSoalUlangan int        `gorm:"not null;default:50" json:"bobot_soal_ulangan"`
	BobotTugas       int        `gorm:"not null;default:50" json:"bobot_tugas"`
	Version          int        `gorm:"not null;default:1" json:"version"`
	ArchivedAt       *time.Time `json:"archived_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	JumlahMurid      int64      `gorm:"-" json:"-"`
}

// TableName binds the struct to the kelas table.
func (Kelas) TableName() string {
	return "kelas"
}

// Enrollment is a composite-PK row binding a siswa to a kelas.
type Enrollment struct {
	KelasID   uuid.UUID        `gorm:"type:uuid;primaryKey" json:"kelas_id"`
	SiswaID   uuid.UUID        `gorm:"type:uuid;primaryKey;index" json:"siswa_id"`
	Status    EnrollmentStatus `gorm:"not null;default:active" json:"status"`
	JoinedAt  time.Time        `gorm:"not null;default:now()" json:"joined_at"`
	JoinedVia JoinedVia        `gorm:"not null" json:"joined_via"`
}

// TableName binds the struct to the enrollment table.
func (Enrollment) TableName() string {
	return "enrollment"
}
