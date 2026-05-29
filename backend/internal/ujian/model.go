// Package ujian holds the Ujian (Ulangan Harian) + UjianSoal +
// HasilUjian + JawabanUjian + EventUjian domain models + repository.
//
// Ujian (Fase 6, locked #83-#88) covers cross-bab graded assessment
// drawn from Bank Soal (per-guru pribadi pool, lihat package banksoal).
// Beda dari Soal Bab (Fase 5):
//
//   - Cross-bab: Ujian per-kelas, soal lintas-bab via Bank Soal.
//   - Source mode (locked #85): manual pick (UjianSoal junction) atau
//     random N dari Bank Soal filter (deterministic seed snapshot).
//   - Single attempt per (Ujian, Siswa) dengan partial unique
//     (deleted_at IS NULL) supaya remedial reset (locked #45 mirror)
//     tetap bisa bikin attempt baru.
//   - Auto-grade tx + cron 30s timer-expire (locked #87 reuse goroutine
//     SoalBab timer_cron).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #83 sub-fase split 6.A-6.G.
//   - #84 Bank Soal scope per-guru pribadi (linked dari ujian.guru_id).
//   - #85 source mode discriminated SourceConfigJSON.
//   - #86 random pool deterministic seed sha256(mulai_unix_micro
//     || siswa_id || ujian_id).
//   - #87 timer expire cron 30s + advisory lock auto-grade tx,
//     reuse goroutine soalbab.TimerCron with second sweep block.
//   - #88 backend coverage gate 70% untuk paket ini + banksoal.
package ujian

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

// SourceMode enumerates Ujian source dispatch (locked #85).
type SourceMode string

const (
	// SourceManual — guru pick soal_ids[] explicit, cached di UjianSoal.
	SourceManual SourceMode = "manual"
	// SourceRandom — filter (mapel?,tingkat?,topik?) + jumlah_soal apply
	// ke Bank Soal owner=Ujian.GuruID, snapshot saat siswa start.
	SourceRandom SourceMode = "random"
)

// Valid reports whether m is a recognised source mode.
func (m SourceMode) Valid() bool {
	switch m {
	case SourceManual, SourceRandom:
		return true
	}
	return false
}

// Status enumerates Ujian lifecycle. Mirror Bab.Status (locked Fase 3).
type Status string

const (
	// StatusDraft — guru sedang setup, siswa tidak lihat.
	StatusDraft Status = "draft"
	// StatusPublished — siswa enrolled bisa lihat + start.
	StatusPublished Status = "published"
	// StatusArchived — guru tutup, siswa tidak bisa start lagi (history
	// + review tetap accessible kalau gating allow).
	StatusArchived Status = "archived"
)

// Valid reports whether s is a recognised ujian status.
func (s Status) Valid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusArchived:
		return true
	}
	return false
}

// HasilStatus enumerates HasilUjian attempt lifecycle. Mirror SoalBab
// (locked #76).
type HasilStatus string

const (
	// HasilBerlangsung — siswa sedang mengerjakan attempt.
	HasilBerlangsung HasilStatus = "berlangsung"
	// HasilSelesai — siswa selesai (manual submit atau cron auto-grade)
	// dan nilai final tercatat.
	HasilSelesai HasilStatus = "selesai"
	// HasilDibatalkan — guru/admin reset attempt (remedial). Soft-deleted
	// via DeletedAt — tidak count terhadap remedial chain.
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

// Ujian represents an Ulangan Harian instance per kelas. SourceConfigJSON
// is a discriminated jsonb (locked #85):
//
//	{mode:"manual", soal_ids:[uuid, ...]}
//	{mode:"random", filter:{mapel?,tingkat?,topik?}, jumlah_soal:N}
//
// Edit Ujian dengan attempt aktif (HasilUjian.Status='berlangsung') di
// handler-level akan ditolak 409 ujian_active_attempts — cancel attempt
// dulu.
type Ujian struct {
	ID                         uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID                    uuid.UUID      `gorm:"type:uuid;not null;index;column:kelas_id" json:"kelas_id"`
	GuruID                     uuid.UUID      `gorm:"type:uuid;not null;index;column:guru_id" json:"guru_id"`
	Judul                      string         `gorm:"not null;default:''" json:"judul"`
	Deskripsi                  string         `gorm:"not null;default:''" json:"deskripsi"`
	DurasiMenit                int16          `gorm:"not null;default:60;column:durasi_menit" json:"durasi_menit"`
	WaktuMulai                 *time.Time     `gorm:"column:waktu_mulai" json:"waktu_mulai,omitempty"`
	WaktuSelesai               *time.Time     `gorm:"column:waktu_selesai" json:"waktu_selesai,omitempty"`
	SourceConfigJSON           datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:source_config_json" json:"source_config_json"`
	IzinkanReviewSetelahSubmit bool           `gorm:"not null;default:true;column:izinkan_review_setelah_submit" json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *time.Time     `gorm:"column:waktu_buka_review" json:"waktu_buka_review,omitempty"`
	BatasAttempt               int16          `gorm:"not null;default:1;column:batas_attempt" json:"batas_attempt"`
	AttemptUnlimited           bool           `gorm:"not null;default:false;column:attempt_unlimited" json:"attempt_unlimited"`
	Bobot                      int            `gorm:"not null;default:100;column:bobot" json:"bobot"`
	Status                     Status         `gorm:"not null;default:draft" json:"status"`
	Version                    int            `gorm:"not null;default:1" json:"version"`
	CreatedAt                  time.Time      `json:"created_at"`
	UpdatedAt                  time.Time      `json:"updated_at"`
}

// TableName binds the struct to the ujian table.
func (Ujian) TableName() string { return "ujian" }

// UjianSoal junctions an Ujian × BankSoal pair for source mode=manual
// (locked #85). Random mode TIDAK populate junction.
type UjianSoal struct {
	UjianID uuid.UUID `gorm:"type:uuid;primaryKey;column:ujian_id" json:"ujian_id"`
	SoalID  uuid.UUID `gorm:"type:uuid;primaryKey;column:soal_id" json:"soal_id"`
	Urutan  int16     `gorm:"not null;default:0" json:"urutan"`
}

// TableName binds the struct to the ujian_soal table.
func (UjianSoal) TableName() string { return "ujian_soal" }

// UjianAccessOverride opens a susulan window for one siswa without changing
// the main ujian schedule for everyone else.
type UjianAccessOverride struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UjianID      uuid.UUID  `gorm:"type:uuid;not null;index;column:ujian_id" json:"ujian_id"`
	SiswaID      uuid.UUID  `gorm:"type:uuid;not null;index;column:siswa_id" json:"siswa_id"`
	WaktuMulai   *time.Time `gorm:"column:waktu_mulai" json:"waktu_mulai,omitempty"`
	WaktuSelesai time.Time  `gorm:"not null;column:waktu_selesai" json:"waktu_selesai"`
	DurasiMenit  *int16     `gorm:"column:durasi_menit" json:"durasi_menit,omitempty"`
	MaxAttempt   int16      `gorm:"not null;default:1;column:max_attempt" json:"max_attempt"`
	Reason       string     `gorm:"not null;default:''" json:"reason"`
	CreatedBy    uuid.UUID  `gorm:"type:uuid;not null;column:created_by" json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName binds the struct to the ujian_access_override table.
func (UjianAccessOverride) TableName() string { return "ujian_access_override" }

// HasilUjian represents a single attempt instance per (Ujian, Siswa).
// Partial unique (ujian_id, siswa_id) WHERE deleted_at IS NULL — remedial
// reset soft-deletes attempt, baru attempt valid.
//
// SoalIDsJSON adalah snapshot pool soal frozen per attempt (locked #86
// deterministic seed). DeadlineAt = MulaiAt + Ujian.DurasiMenit; cron
// 30s sweep auto-grade kalau lewat (locked #87). NilaiTotal +
// JawabanBenarCount diisi saat submit atau cron auto-grade.
//
// AttemptNo untuk audit remedial chain — soft cancel via
// Status='dibatalkan' + DeletedAt tidak count.
type HasilUjian struct {
	ID                uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UjianID           uuid.UUID      `gorm:"type:uuid;not null;index;column:ujian_id" json:"ujian_id"`
	SiswaID           uuid.UUID      `gorm:"type:uuid;not null;index;column:siswa_id" json:"siswa_id"`
	Status            HasilStatus    `gorm:"not null;default:berlangsung" json:"status"`
	SoalIDsJSON       datatypes.JSON `gorm:"type:jsonb;not null;default:'[]';column:soal_ids_json" json:"soal_ids_json"`
	MulaiAt           time.Time      `gorm:"not null;default:now();column:mulai_at" json:"mulai_at"`
	DeadlineAt        *time.Time     `gorm:"column:deadline_at" json:"deadline_at,omitempty"`
	SelesaiAt         *time.Time     `gorm:"column:selesai_at" json:"selesai_at,omitempty"`
	NilaiTotal        *float64       `gorm:"type:numeric(6,2);column:nilai_total" json:"nilai_total,omitempty"`
	JawabanBenarCount *int16         `gorm:"column:jawaban_benar_count" json:"jawaban_benar_count,omitempty"`
	JawabanTotal      *int16         `gorm:"column:jawaban_total" json:"jawaban_total,omitempty"`
	AttemptNo         int16          `gorm:"not null;default:1;column:attempt_no" json:"attempt_no"`
	DeletedAt         *time.Time     `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// TableName binds the struct to the hasil_ujian table.
func (HasilUjian) TableName() string { return "hasil_ujian" }

// JawabanUjian represents a siswa's answer for a single soal within an
// attempt. UNIQUE (hasil_id, soal_id) supaya UPSERT autosave aman dan
// re-answer overwrite. Mirror JawabanBab (Ulangan delayed grade pattern):
// IsBenar NULL + PoinDapat 0 sampai submit, lalu di-grade batch dalam tx
// (locked #87).
type JawabanUjian struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HasilID    uuid.UUID `gorm:"type:uuid;not null;index;column:hasil_id" json:"hasil_id"`
	SoalID     uuid.UUID `gorm:"type:uuid;not null;column:soal_id" json:"soal_id"`
	Jawaban    *string   `gorm:"column:jawaban" json:"jawaban,omitempty"`
	IsBenar    *bool     `gorm:"column:is_benar" json:"is_benar,omitempty"`
	PoinDapat  int16     `gorm:"not null;default:0;column:poin_dapat" json:"poin_dapat"`
	AnsweredAt time.Time `gorm:"not null;default:now();column:answered_at" json:"answered_at"`
}

// TableName binds the struct to the jawaban_ujian table.
func (JawabanUjian) TableName() string { return "jawaban_ujian" }

// EventUjian is an anti-cheat audit ledger row per attempt. Action enum
// (string) covers soal_view, answer_save, submit, timer_expire, resume,
// cancel. Meta jsonb opaque (locked #55-style ledger pattern). Mirror
// EventBab.
type EventUjian struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	HasilID   uuid.UUID      `gorm:"type:uuid;not null;index;column:hasil_id" json:"hasil_id"`
	Action    string         `gorm:"not null" json:"action"`
	Meta      datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'" json:"meta"`
	CreatedAt time.Time      `json:"created_at"`
}

// TableName binds the struct to the event_ujian table.
func (EventUjian) TableName() string { return "event_ujian" }
