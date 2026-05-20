// Package bab holds the bab (chapter) domain model + repository.
//
// Bab is a chapter within a kelas. Materi, soal, tugas, and pengumuman can
// optionally reference a bab (BabID nullable on those tables). The Status
// enum (draft|published|archived) is the single visibility/lifecycle column
// — siswa only see Status='published' (Section 6.1 decision: no separate
// archived_at on bab; gabung jadi 1 enum).
package bab

import (
	"time"

	"github.com/google/uuid"
)

// Status enumerates the lifecycle of a bab.
type Status string

const (
	// StatusDraft is the default — invisible to siswa, editable by guru.
	StatusDraft Status = "draft"
	// StatusPublished is visible to enrolled siswa.
	StatusPublished Status = "published"
	// StatusArchived is hidden from siswa lists but kept for audit/history.
	StatusArchived Status = "archived"
)

// Valid reports whether s is a recognised lifecycle value.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusArchived:
		return true
	}
	return false
}

// Bab represents a chapter within a kelas.
//
// Version guards optimistic concurrency on PATCH (#56). The enum Status
// covers both workflow (draft/published) and tombstone (archived) — there
// is no separate ArchivedAt column.
type Bab struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID   uuid.UUID `gorm:"type:uuid;not null;index" json:"kelas_id"`
	Nomor     int       `gorm:"not null" json:"nomor"`
	Judul     string    `gorm:"not null" json:"judul"`
	Deskripsi string    `gorm:"not null;default:''" json:"deskripsi"`
	Urutan    int       `gorm:"not null" json:"urutan"`
	Status    Status    `gorm:"not null;default:draft" json:"status"`
	Version   int       `gorm:"not null;default:1" json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName binds the struct to the bab table.
func (Bab) TableName() string {
	return "bab"
}
