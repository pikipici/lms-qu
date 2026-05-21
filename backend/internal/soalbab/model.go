// Package soalbab holds the SoalBab + UlanganBabSetting + HasilSoalBab +
// JawabanBab + EventBab + SoalAssignment domain models + repository.
//
// SoalBab covers two flows:
//
//   - Latihan: formative practice, no nilai persist, immediate is_benar
//     feedback. Siswa boleh re-attempt unlimited (locked #81).
//
//   - Ulangan Bab: graded attempt(s) with timer, attempt limit, optional
//     review gating. Random pool snapshot per attempt via deterministic seed
//     sha256(mulai_unix_micro || siswa_id || bab_id) (locked #79). Auto-grade
//     on submit + cron 30s timer-expire sweep with advisory lock (locked #80).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #62 file access via presigned URL.
//   - #69 hard delete + R2 cleanup compensating pattern.
//   - #76 sub-fase split 5.A-5.G.
//   - #77 bulk paste pipe-delimited 9-kolom.
//   - #78 image upload inline 6-slot 5MB resize 1920px.
//   - #79 random pool deterministic seed sha256.
//   - #80 timer expire cron 30s + advisory lock auto-grade tx.
//   - #81 review gating: Latihan always open, Ulangan gated by setting.
//   - #82 backend coverage gate 70% untuk paket ini.
package soalbab

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// Mode enumerates a soal's flow eligibility.
type Mode string

const (
	// ModeLatihan — soal hanya muncul di flow Latihan (formative).
	ModeLatihan Mode = "latihan"
	// ModeUlangan — soal hanya muncul di flow Ulangan Bab (graded).
	ModeUlangan Mode = "ulangan"
	// ModeKeduanya — soal eligible untuk Latihan dan Ulangan Bab (default).
	ModeKeduanya Mode = "keduanya"
)

// Valid reports whether m is a recognised mode value.
func (m Mode) Valid() bool {
	switch m {
	case ModeLatihan, ModeUlangan, ModeKeduanya:
		return true
	}
	return false
}

// HasilStatus enumerates the lifecycle of a HasilSoalBab attempt.
type HasilStatus string

const (
	// HasilBerlangsung — siswa sedang mengerjakan attempt.
	HasilBerlangsung HasilStatus = "berlangsung"
	// HasilSelesai — siswa selesai (manual finish atau auto-submit) dan nilai
	// final tercatat (untuk ulangan).
	HasilSelesai HasilStatus = "selesai"
	// HasilDibatalkan — guru/admin reset attempt (remedial). Tidak count
	// terhadap batas_attempt (locked #76).
	HasilDibatalkan HasilStatus = "dibatalkan"
)

// Valid reports whether s is a recognised hasil status.
func (s HasilStatus) Valid() bool {
	switch s {
	case HasilBerlangsung, HasilSelesai, HasilDibatalkan:
		return true
	}
	return false
}

// HasilMode enumerates whether an attempt belongs to Latihan or Ulangan flow.
type HasilMode string

const (
	// HasilModeLatihan — formative attempt, nilai_total NULL.
	HasilModeLatihan HasilMode = "latihan"
	// HasilModeUlangan — graded attempt, nilai_total persisted on submit.
	HasilModeUlangan HasilMode = "ulangan"
)

// Valid reports whether m is a recognised hasil mode value.
func (m HasilMode) Valid() bool {
	switch m {
	case HasilModeLatihan, HasilModeUlangan:
		return true
	}
	return false
}

// Jawaban enumerates the answer key letter (single-answer MVP).
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

// SoalBab represents a single multiple-choice question with 5 options
// (a..e) and 1 answer key. Each option + the question stem may carry an
// inline image (object key di R2 prefix soalbab/<uuid>, locked #78).
//
// Mode drives flow eligibility (locked #76): latihan-only, ulangan-only,
// atau keduanya (default). KelasID di-denormal untuk query cepat tanpa
// join via bab.
type SoalBab struct {
	ID                    uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BabID                 uuid.UUID `gorm:"type:uuid;not null;index;column:bab_id" json:"bab_id"`
	KelasID               uuid.UUID `gorm:"type:uuid;not null;index;column:kelas_id" json:"kelas_id"`
	Pertanyaan            string    `gorm:"not null;default:''" json:"pertanyaan"`
	PertanyaanObjectKey   *string   `gorm:"column:pertanyaan_object_key" json:"pertanyaan_object_key,omitempty"`
	OpsiA                 string    `gorm:"not null;default:'';column:opsi_a" json:"opsi_a"`
	OpsiAObjectKey        *string   `gorm:"column:opsi_a_object_key" json:"opsi_a_object_key,omitempty"`
	OpsiB                 string    `gorm:"not null;default:'';column:opsi_b" json:"opsi_b"`
	OpsiBObjectKey        *string   `gorm:"column:opsi_b_object_key" json:"opsi_b_object_key,omitempty"`
	OpsiC                 string    `gorm:"not null;default:'';column:opsi_c" json:"opsi_c"`
	OpsiCObjectKey        *string   `gorm:"column:opsi_c_object_key" json:"opsi_c_object_key,omitempty"`
	OpsiD                 string    `gorm:"not null;default:'';column:opsi_d" json:"opsi_d"`
	OpsiDObjectKey        *string   `gorm:"column:opsi_d_object_key" json:"opsi_d_object_key,omitempty"`
	OpsiE                 string    `gorm:"not null;default:'';column:opsi_e" json:"opsi_e"`
	OpsiEObjectKey        *string   `gorm:"column:opsi_e_object_key" json:"opsi_e_object_key,omitempty"`
	Jawaban               Jawaban   `gorm:"not null" json:"jawaban"`
	Poin                  int16     `gorm:"not null;default:1" json:"poin"`
	Mode                  Mode      `gorm:"not null;default:keduanya" json:"mode"`
	Urutan                int       `gorm:"not null;default:0" json:"urutan"`
	Version               int       `gorm:"not null;default:1" json:"version"`
	CreatedByID           uuid.UUID `gorm:"type:uuid;not null;column:created_by_id" json:"created_by_id"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// TableName binds the struct to the soal_bab table.
func (SoalBab) TableName() string {
	return "soal_bab"
}

// UlanganBabSetting stores the ulangan-bab configuration per Bab (1:1 via
// UNIQUE bab_id). JumlahSoal harus ≤ count(soal mode IN ('ulangan',
// 'keduanya')) — backend validate saat upsert.
//
// Review gating (locked #81): IzinkanReviewSetelahSubmit gated to true
// AND (WaktuBukaReview NULL OR <= now). Latihan always open regardless.
type UlanganBabSetting struct {
	ID                         uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BabID                      uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex;column:bab_id" json:"bab_id"`
	JumlahSoal                 int16      `gorm:"not null;default:10;column:jumlah_soal" json:"jumlah_soal"`
	DurasiMenit                int16      `gorm:"not null;default:30;column:durasi_menit" json:"durasi_menit"`
	BatasAttempt               int16      `gorm:"not null;default:1;column:batas_attempt" json:"batas_attempt"`
	IzinkanReviewSetelahSubmit bool       `gorm:"not null;default:true;column:izinkan_review_setelah_submit" json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *time.Time `gorm:"column:waktu_buka_review" json:"waktu_buka_review,omitempty"`
	Version                    int        `gorm:"not null;default:1" json:"version"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

// TableName binds the struct to the ulangan_bab_setting table.
func (UlanganBabSetting) TableName() string {
	return "ulangan_bab_setting"
}

// HasilSoalBab represents a single attempt instance: Latihan atau Ulangan.
//
// SoalIDsJSON adalah snapshot pool soal frozen per attempt (locked #79
// deterministic seed). DeadlineAt hanya untuk ulangan; cron 30s sweep
// auto-grade kalau lewat (locked #80). Latihan: NilaiTotal +
// JawabanBenarCount NULL (formative). Ulangan: diisi saat submit atau
// cron auto-grade.
//
// AttemptNo untuk audit remedial chain — soft cancel via Status =
// 'dibatalkan' tidak count.
type HasilSoalBab struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	BabID             uuid.UUID      `gorm:"type:uuid;not null;index;column:bab_id" json:"bab_id"`
	SiswaID           uuid.UUID      `gorm:"type:uuid;not null;index;column:siswa_id" json:"siswa_id"`
	Mode              HasilMode      `gorm:"not null" json:"mode"`
	Status            HasilStatus    `gorm:"not null;default:berlangsung" json:"status"`
	SoalIDsJSON       datatypes.JSON `gorm:"type:jsonb;not null;default:'[]';column:soal_ids_json" json:"soal_ids_json"`
	MulaiAt           time.Time      `gorm:"not null;default:now();column:mulai_at" json:"mulai_at"`
	DeadlineAt        *time.Time     `gorm:"column:deadline_at" json:"deadline_at,omitempty"`
	SelesaiAt         *time.Time     `gorm:"column:selesai_at" json:"selesai_at,omitempty"`
	NilaiTotal        *float64       `gorm:"type:numeric(6,2);column:nilai_total" json:"nilai_total,omitempty"`
	JawabanBenarCount *int16         `gorm:"column:jawaban_benar_count" json:"jawaban_benar_count,omitempty"`
	JawabanTotal      *int16         `gorm:"column:jawaban_total" json:"jawaban_total,omitempty"`
	AttemptNo         int16          `gorm:"not null;default:1;column:attempt_no" json:"attempt_no"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// TableName binds the struct to the hasil_soal_bab table.
func (HasilSoalBab) TableName() string {
	return "hasil_soal_bab"
}

// JawabanBab represents a siswa's answer for a single soal within an
// attempt. UNIQUE (hasil_id, soal_id) supaya UPSERT autosave aman dan
// re-answer overwrite.
//
// Latihan: IsBenar + PoinDapat diisi saat answer (immediate feedback).
// Ulangan: IsBenar NULL + PoinDapat 0 sampai submit, lalu di-grade batch
// dalam tx (locked #80).
type JawabanBab struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HasilID    uuid.UUID `gorm:"type:uuid;not null;index;column:hasil_id" json:"hasil_id"`
	SoalID     uuid.UUID `gorm:"type:uuid;not null;column:soal_id" json:"soal_id"`
	Jawaban    *string   `gorm:"column:jawaban" json:"jawaban,omitempty"`
	IsBenar    *bool     `gorm:"column:is_benar" json:"is_benar,omitempty"`
	PoinDapat  int16     `gorm:"not null;default:0;column:poin_dapat" json:"poin_dapat"`
	AnsweredAt time.Time `gorm:"not null;default:now();column:answered_at" json:"answered_at"`
}

// TableName binds the struct to the jawaban_bab table.
func (JawabanBab) TableName() string {
	return "jawaban_bab"
}

// EventBab is an anti-cheat audit ledger row per attempt. Action enum
// (string) covers soal_view, answer_save, submit, timer_expire, resume.
// Meta jsonb opaque (locked #55-style ledger pattern).
type EventBab struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HasilID   uuid.UUID      `gorm:"type:uuid;not null;index;column:hasil_id" json:"hasil_id"`
	Action    string         `gorm:"not null" json:"action"`
	Meta      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"meta"`
	CreatedAt time.Time      `json:"created_at"`
}

// TableName binds the struct to the event_bab table.
func (EventBab) TableName() string {
	return "event_bab"
}

// SoalAssignment audits a guru copying soal from one bab to another.
// UNIQUE (source_bab_id, target_bab_id) supaya idempotent. Out-of-scope
// MVP untuk endpoint; defer ke Fase 5+ kalau guru request fitur ini.
type SoalAssignment struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	SourceBabID  uuid.UUID `gorm:"type:uuid;not null;column:source_bab_id" json:"source_bab_id"`
	TargetBabID  uuid.UUID `gorm:"type:uuid;not null;column:target_bab_id" json:"target_bab_id"`
	CopiedCount  int       `gorm:"not null;default:0;column:copied_count" json:"copied_count"`
	CreatedByID  uuid.UUID `gorm:"type:uuid;not null;column:created_by_id" json:"created_by_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// TableName binds the struct to the soal_assignment table.
func (SoalAssignment) TableName() string {
	return "soal_assignment"
}
