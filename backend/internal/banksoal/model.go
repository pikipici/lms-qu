// Package banksoal holds the BankSoal domain model + repository for
// Fase 6 (Ulangan Harian).
//
// BankSoal (locked #84) is a per-guru pribadi pool of multiple-choice
// questions, scoped lintas-bab (no FK ke Bab). Each soal carries optional
// tag fields mapel/tingkat/topik (free-form text) used by random-mode
// Ujian source filter (locked #85). Image slot mirror SoalBab pattern
// (locked #78): 1 question stem + 5 option images stored in R2 prefix
// `soal-bank/<uuid>.<ext>`.
//
// Ownership is enforced at handler/service: WHERE owner_guru_id =
// current_user.id. Sharing antar-guru di MVP TIDAK ada (open decision
// #8, defer v1).
//
// Soft delete via DeletedAt — once a HasilUjian attempt referensi soal
// ini, hard delete tidak aman. Soft delete bikin soal hidden dari list
// guru tapi tetap renderable di review siswa post-attempt.
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #62 file access via presigned URL.
//   - #69 hard delete + R2 cleanup compensating pattern (untuk soal
//     yang belum pernah dipake, hard delete OK; sebaliknya soft delete).
//   - #78 image upload inline 6-slot 5MB resize 1920px.
//   - #84 Bank Soal scope per-guru pribadi.
//   - #88 backend coverage gate 70% untuk paket ini + ujian.
package banksoal

import (
	"time"

	"github.com/google/uuid"
)

// Jawaban enumerates the answer key letter (single-answer MVP, mirror
// soalbab.Jawaban — duplicated per-package supaya tidak ada cross-import
// soalbab → banksoal).
type Jawaban string

const (
	JawabanA Jawaban = "a"
	JawabanB Jawaban = "b"
	JawabanC Jawaban = "c"
	JawabanD Jawaban = "d"
	JawabanE Jawaban = "e"
)

// Valid reports whether j is one of the recognised answer letters.
func (j Jawaban) Valid() bool {
	switch j {
	case JawabanA, JawabanB, JawabanC, JawabanD, JawabanE:
		return true
	}
	return false
}

// BankSoal represents a single multiple-choice question in a guru's
// private bank. 5 fixed options (a..e) + 1 answer key. Each option +
// the question stem may carry an inline image (object key di R2 prefix
// `soal-bank/<uuid>`, locked #78).
//
// Tag fields (mapel/tingkat/topik) are free-form text, indexed for
// random-mode Ujian filter (locked #85). They are NOT FK ke kelas
// mapel set — guru bebas typo / inconsistency, FE bisa offer suggest
// dropdown dari kelas existing.
//
// DeletedAt: soft delete agar HasilUjian referensi tetap valid setelah
// guru hapus soal (mirror Submission pattern locked #70).
type BankSoal struct {
	ID                  uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	OwnerGuruID         uuid.UUID  `gorm:"type:uuid;not null;index;column:owner_guru_id" json:"owner_guru_id"`
	Mapel               string     `gorm:"not null;default:''" json:"mapel"`
	Tingkat             string     `gorm:"not null;default:''" json:"tingkat"`
	Topik               string     `gorm:"not null;default:''" json:"topik"`
	Pertanyaan          string     `gorm:"not null;default:''" json:"pertanyaan"`
	PertanyaanObjectKey *string    `gorm:"column:pertanyaan_object_key" json:"pertanyaan_object_key,omitempty"`
	OpsiA               string     `gorm:"not null;default:'';column:opsi_a" json:"opsi_a"`
	OpsiAObjectKey      *string    `gorm:"column:opsi_a_object_key" json:"opsi_a_object_key,omitempty"`
	OpsiB               string     `gorm:"not null;default:'';column:opsi_b" json:"opsi_b"`
	OpsiBObjectKey      *string    `gorm:"column:opsi_b_object_key" json:"opsi_b_object_key,omitempty"`
	OpsiC               string     `gorm:"not null;default:'';column:opsi_c" json:"opsi_c"`
	OpsiCObjectKey      *string    `gorm:"column:opsi_c_object_key" json:"opsi_c_object_key,omitempty"`
	OpsiD               string     `gorm:"not null;default:'';column:opsi_d" json:"opsi_d"`
	OpsiDObjectKey      *string    `gorm:"column:opsi_d_object_key" json:"opsi_d_object_key,omitempty"`
	OpsiE               string     `gorm:"not null;default:'';column:opsi_e" json:"opsi_e"`
	OpsiEObjectKey      *string    `gorm:"column:opsi_e_object_key" json:"opsi_e_object_key,omitempty"`
	Jawaban             Jawaban    `gorm:"not null" json:"jawaban"`
	Poin                int16      `gorm:"not null;default:1" json:"poin"`
	Version             int        `gorm:"not null;default:1" json:"version"`
	DeletedAt           *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName binds the struct to the bank_soal table.
func (BankSoal) TableName() string { return "bank_soal" }
