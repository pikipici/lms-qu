// Package submission holds the submission domain model + repository.
//
// Submission represents a siswa's response to a Tugas. Single-row per
// (TugasID, SiswaID) — UNIQUE constraint at DB level (locked #70).
// Resubmit overwrites konten + attachment set + bumps Version (locked #56
// pattern). History per-attempt out-of-scope MVP.
//
// Locked decisions referenced:
//   - #46 attachment mime allowlist + size cap.
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #69 hard delete + R2 cleanup compensating pattern.
//   - #70 single-row + version bump strategy on resubmit.
//   - #71 late submission gating: IsLate flag + penalty calc.
//   - #72 attachment policy: 0..N optional, cap 5 × 20MB.
//   - #73 SELECT FOR UPDATE + idempotent guard for submit/grade tx.
package submission

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// Status enumerates the lifecycle of a submission.
type Status string

const (
	// StatusSubmitted — siswa submit, awaiting grade.
	StatusSubmitted Status = "submitted"
	// StatusGraded — guru kasih nilai; final di MVP (locked #73).
	StatusGraded Status = "graded"
	// StatusReturned — guru return for revision (defer MVP, defined for
	// forward-compat).
	StatusReturned Status = "returned"
)

// Valid reports whether s is a recognised lifecycle value.
func (s Status) Valid() bool {
	switch s {
	case StatusSubmitted, StatusGraded, StatusReturned:
		return true
	}
	return false
}

// RoundNilai rounds a nilai value to 2 decimals (banker-safe via half-up).
// Use saat compute NilaiSetelahPenalty atau saat persist NilaiAsli supaya
// DB numeric(5,2) ga reject.
func RoundNilai(v float64) float64 {
	return math.Round(v*100) / 100
}

// Submission represents a siswa's response to a Tugas.
//
// NilaiAsli, PenaltyPersenApplied, NilaiSetelahPenalty, GradedByID, GradedAt
// are nullable — set saat grade, null saat status='submitted'. Version
// guards optimistic concurrency on grade (locked #56).
//
// Nilai fields use *float64 (NUMERIC(5,2) di DB — 0..100 × 2 decimal).
// Float64 punya cukup significand untuk range ini; service layer wajib
// pakai RoundNilai sebelum persist.
type Submission struct {
	ID                   uuid.UUID    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	TugasID              uuid.UUID    `gorm:"type:uuid;not null;index;column:tugas_id" json:"tugas_id"`
	SiswaID              uuid.UUID    `gorm:"type:uuid;not null;index;column:siswa_id" json:"siswa_id"`
	Catatan              string       `gorm:"not null;default:''" json:"catatan"`
	Status               Status       `gorm:"not null;default:submitted" json:"status"`
	IsLate               bool         `gorm:"not null;default:false;column:is_late" json:"is_late"`
	NilaiAsli            *float64     `gorm:"type:numeric(5,2);column:nilai_asli" json:"nilai_asli,omitempty"`
	PenaltyPersenApplied *int16       `gorm:"column:penalty_persen_applied" json:"penalty_persen_applied,omitempty"`
	NilaiSetelahPenalty  *float64     `gorm:"type:numeric(5,2);column:nilai_setelah_penalty" json:"nilai_setelah_penalty,omitempty"`
	Feedback             string       `gorm:"not null;default:''" json:"feedback"`
	GradedByID           *uuid.UUID   `gorm:"type:uuid;column:graded_by_id" json:"graded_by_id,omitempty"`
	GradedAt             *time.Time   `gorm:"column:graded_at" json:"graded_at,omitempty"`
	Version              int          `gorm:"not null;default:1" json:"version"`
	SubmittedAt          time.Time    `gorm:"not null;default:now();column:submitted_at" json:"submitted_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
	Attachments          []Attachment `gorm:"foreignKey:SubmissionID;references:ID" json:"attachments,omitempty"`
}

// TableName binds the struct to the submission table.
func (Submission) TableName() string {
	return "submission"
}

// Attachment represents a single file attached to a submission (siswa upload).
//
// FK CASCADE ke submission — DELETE submission auto-delete attachment row,
// tapi caller tetap perlu R2 DeleteObject compensating (locked #69 pattern).
//
// R2 path: "submission/<uuid>.<ext>" (locked #58/#72). Allowlist mime via
// locked #46 (pdf, docx, jpg, png, zip), cap 20MB per file, cap 5 file
// per submission.
type Attachment struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SubmissionID     uuid.UUID `gorm:"type:uuid;not null;index;column:submission_id" json:"submission_id"`
	ObjectKey        string    `gorm:"not null;column:object_key" json:"object_key"`
	OriginalFilename string    `gorm:"not null;column:original_filename" json:"original_filename"`
	MimeType         string    `gorm:"not null;column:mime_type" json:"mime_type"`
	SizeBytes        int64     `gorm:"not null;column:size_bytes" json:"size_bytes"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName binds the struct to the submission_attachment table.
func (Attachment) TableName() string {
	return "submission_attachment"
}
