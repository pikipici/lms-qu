// Package pengumuman holds the pengumuman (announcement) domain model + repository.
//
// Pengumuman is an announcement within a kelas. Optionally linked to a
// bab (BabID nullable — bisa kelas-wide atau bab-scoped, locked #20).
// Status enum (published|archived) covers visibility lifecycle: archived
// pengumuman hidden dari siswa list, masih visible ke guru/admin untuk
// audit.
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #66 Pengumuman passive timestamp: tidak ada per-siswa read receipt
//     di MVP. Frontend pakai created_at vs last_seen client-side untuk
//     badge "Baru" (< 7 hari).
package pengumuman

import (
	"time"

	"github.com/google/uuid"
)

// Status enumerates the lifecycle of a pengumuman.
type Status string

const (
	// StatusPublished is the default — visible to enrolled siswa + guru/admin.
	StatusPublished Status = "published"
	// StatusArchived is hidden from siswa list, masih visible ke guru/admin.
	StatusArchived Status = "archived"
)

// Valid reports whether s is a recognised lifecycle value.
func (s Status) Valid() bool {
	switch s {
	case StatusPublished, StatusArchived:
		return true
	}
	return false
}

// Pengumuman represents a single announcement within a kelas.
//
// Version guards optimistic concurrency on PATCH (#56). BabID nullable
// untuk distinguish kelas-wide (NULL) vs bab-scoped (UUID) announcement.
type Pengumuman struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"kelas_id"`
	BabID               *uuid.UUID `gorm:"type:uuid;index" json:"bab_id,omitempty"`
	Judul               string     `gorm:"not null" json:"judul"`
	Isi                 string     `gorm:"not null;default:''" json:"isi"`
	CreatedByID         uuid.UUID  `gorm:"type:uuid;not null;column:created_by_id" json:"created_by_id"`
	Status              Status     `gorm:"not null;default:published" json:"status"`
	AttachmentObjectKey *string    `gorm:"column:attachment_object_key" json:"attachment_object_key,omitempty"`
	AttachmentFilename  *string    `gorm:"column:attachment_filename" json:"attachment_filename,omitempty"`
	AttachmentMime      *string    `gorm:"column:attachment_mime" json:"attachment_mime,omitempty"`
	AttachmentSize      *int64     `gorm:"column:attachment_size" json:"attachment_size,omitempty"`
	Version             int        `gorm:"not null;default:1" json:"version"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName binds the struct to the pengumuman table.
func (Pengumuman) TableName() string {
	return "pengumuman"
}
