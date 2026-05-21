// Package materi holds the materi (learning content) domain model + repository.
//
// Materi is a learning content unit within a kelas. Optionally linked to a
// bab (BabID nullable — bisa nempel ke bab atau berdiri bebas, locked #20).
// Tipe enum (pdf|youtube|markdown) locks 3 modes (decision #63 — drop
// direct video upload, YouTube embed cukup di Fase 3).
//
// Payload mapping per tipe:
//   - pdf:      ObjectKey + OriginalFilename + MimeType + SizeBytes (R2 path
//               "materi/<uuid>.pdf", max 20MB locked #64). Konten kosong.
//   - youtube:  Konten = video_id 11-char (parsed via parseYouTubeID locked
//               #65). ObjectKey/etc. nil.
//   - markdown: Konten = body markdown (max 50KB enforce di handler).
//               ObjectKey/etc. nil.
//
// DB CHECK constraint (materi_tipe_payload_chk) enforces this invariant.
package materi

import (
	"time"

	"github.com/google/uuid"
)

// Tipe enumerates the supported materi content modes (locked #63).
type Tipe string

const (
	// TipePDF is a PDF file uploaded to R2 (max 20MB, locked #64).
	TipePDF Tipe = "pdf"
	// TipeYouTube is a YouTube video referenced by 11-char video_id (locked #65).
	TipeYouTube Tipe = "youtube"
	// TipeMarkdown is a markdown body stored inline in DB.
	TipeMarkdown Tipe = "markdown"
)

// Valid reports whether t is a recognised content tipe.
func (t Tipe) Valid() bool {
	switch t {
	case TipePDF, TipeYouTube, TipeMarkdown:
		return true
	}
	return false
}

// Materi represents a single learning content unit within a kelas.
//
// Version guards optimistic concurrency on PATCH (#56). PDF payload fields
// (ObjectKey/OriginalFilename/MimeType/SizeBytes) are pointer types because
// they are NULL in DB for non-pdf tipe — the DB CHECK constraint
// materi_tipe_payload_chk enforces tipe<->payload coherence.
type Materi struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"kelas_id"`
	BabID            *uuid.UUID `gorm:"type:uuid;index" json:"bab_id,omitempty"`
	Judul            string     `gorm:"not null" json:"judul"`
	Tipe             Tipe       `gorm:"not null" json:"tipe"`
	Konten           string     `gorm:"not null;default:''" json:"konten"`
	ObjectKey        *string    `gorm:"column:object_key" json:"object_key,omitempty"`
	OriginalFilename *string    `gorm:"column:original_filename" json:"original_filename,omitempty"`
	MimeType         *string    `gorm:"column:mime_type" json:"mime_type,omitempty"`
	SizeBytes        *int64     `gorm:"column:size_bytes" json:"size_bytes,omitempty"`
	Urutan           int        `gorm:"not null;default:0" json:"urutan"`
	Version          int        `gorm:"not null;default:1" json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TableName binds the struct to the materi table.
func (Materi) TableName() string {
	return "materi"
}

// Read represents a per-siswa mark-as-read row for a materi (composite PK).
//
// Idempotent insert via ON CONFLICT DO NOTHING (locked #25). Dipakai untuk
// progress calc Fase-3-partial: materi_dibaca / total_materi (locked #68).
type Read struct {
	MateriID uuid.UUID `gorm:"type:uuid;primaryKey;column:materi_id" json:"materi_id"`
	SiswaID  uuid.UUID `gorm:"type:uuid;primaryKey;column:siswa_id" json:"siswa_id"`
	ReadAt   time.Time `gorm:"column:read_at" json:"read_at"`
}

// TableName binds the struct to the materi_read table.
func (Read) TableName() string {
	return "materi_read"
}
