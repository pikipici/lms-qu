// Package tugas holds the tugas (assignment) domain model + repository.
//
// Tugas is an assignment within a kelas. Optionally linked to a bab
// (BabID nullable — bisa kelas-wide atau bab-scoped, locked #20).
// Status enum (draft|published|archived) covers visibility lifecycle:
// archived hidden dari siswa list, masih visible ke guru/admin untuk
// audit transparansi.
//
// Locked decisions referenced:
//   - #20 BabID nullable: tugas bisa kelas-wide (NULL) atau bab-scoped.
//   - #46 attachment mime allowlist + size cap.
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #69 hard delete + R2 cleanup compensating pattern.
//   - #71 late submission gating: IzinkanLate + PenaltyPersen (0-100).
//     PenaltyPersen=0 = allow late tanpa potongan. Kalau IzinkanLate=false,
//     backend reject submit post-deadline (403 deadline_passed).
//   - #72 submission attachment policy: WajibAttachment enforce minimum 1
//     attachment kalau true.
//   - #74 tugas attachment policy: cap 5 file × 20MB, R2 prefix "tugas/".
package tugas

import (
	"time"

	"github.com/google/uuid"
)

// Status enumerates the lifecycle of a tugas.
type Status string

const (
	// StatusDraft — default, hidden from siswa, editable by guru.
	StatusDraft Status = "draft"
	// StatusPublished — visible to enrolled siswa, accepts submission.
	StatusPublished Status = "published"
	// StatusArchived — hidden from siswa list, masih visible ke guru/admin.
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

// Tugas represents a single assignment within a kelas.
//
// Version guards optimistic concurrency on PATCH (#56). BabID nullable
// untuk distinguish kelas-wide (NULL) vs bab-scoped (UUID) — locked #20.
// Deadline nullable: tugas tanpa deadline = always-open (siswa bisa
// submit kapan saja, IsLate selalu false).
type Tugas struct {
	ID              uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID         uuid.UUID    `gorm:"type:uuid;not null;index" json:"kelas_id"`
	BabID           *uuid.UUID   `gorm:"type:uuid;index" json:"bab_id,omitempty"`
	Judul           string       `gorm:"not null" json:"judul"`
	Deskripsi       string       `gorm:"not null;default:''" json:"deskripsi"`
	Deadline        *time.Time   `gorm:"column:deadline" json:"deadline,omitempty"`
	IzinkanLate     bool         `gorm:"not null;default:false;column:izinkan_late" json:"izinkan_late"`
	PenaltyPersen   int16        `gorm:"not null;default:0;column:penalty_persen" json:"penalty_persen"`
	WajibAttachment bool         `gorm:"not null;default:false;column:wajib_attachment" json:"wajib_attachment"`
	Bobot           int          `gorm:"not null;default:100;column:bobot" json:"bobot"`
	Status          Status       `gorm:"not null;default:draft" json:"status"`
	Version         int          `gorm:"not null;default:1" json:"version"`
	CreatedByID     uuid.UUID    `gorm:"type:uuid;not null;column:created_by_id" json:"created_by_id"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	Attachments     []Attachment `gorm:"foreignKey:TugasID;references:ID" json:"attachments,omitempty"`
}

// TableName binds the struct to the tugas table.
func (Tugas) TableName() string {
	return "tugas"
}

// Attachment represents a single file attached to a tugas (lampiran soal/
// instruksi dari guru). FK CASCADE ke tugas — DELETE tugas otomatis hapus
// attachment row, tapi caller tetap perlu R2 DeleteObject compensating
// (locked #69 pattern).
//
// R2 path: "tugas/<uuid>.<ext>" (locked #58/#74). Allowlist mime via
// locked #46 (pdf, docx, jpg, png, zip), cap 20MB per file, cap 5 file
// per tugas.
type Attachment struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TugasID          uuid.UUID `gorm:"type:uuid;not null;index;column:tugas_id" json:"tugas_id"`
	ObjectKey        string    `gorm:"not null;column:object_key" json:"object_key"`
	OriginalFilename string    `gorm:"not null;column:original_filename" json:"original_filename"`
	MimeType         string    `gorm:"not null;column:mime_type" json:"mime_type"`
	SizeBytes        int64     `gorm:"not null;column:size_bytes" json:"size_bytes"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName binds the struct to the tugas_attachment table.
func (Attachment) TableName() string {
	return "tugas_attachment"
}
