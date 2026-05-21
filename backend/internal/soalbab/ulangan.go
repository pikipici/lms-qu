// Ulangan Bab flow service for Task 5.D.1 (start endpoint only).
//
// Ulangan adalah graded attempt — siswa mengerjakan N soal random dari
// pool mode IN ('ulangan','keduanya') dengan timer + nilai persist.
// Anti-cheat utama: pool snapshot deterministic per attempt (locked #79)
// dan single in-flight attempt per (bab, siswa) via pg advisory lock.
//
// Endpoint Task 5.D.1:
//   - POST /api/v1/siswa/bab/:id/ulangan/start
//
// Endpoint answer/submit/cron deferred ke 5.D.2-5.D.4.
//
// Locked decisions:
//   - #56 optimistic concurrency (setting wajib valid).
//   - #76 sub-fase split + batas_attempt enforcement.
//   - #79 deterministic seed sha256(mulai_at_unix_micro || siswa_id ||
//     bab_id)[:8] LE → int64 → math/rand source. Snapshot disimpan di
//     hasil.SoalIDsJSON (frozen). Resume bawa pool yang sama, refresh
//     tidak shuffle ulang.
//   - #80 auto-grade tx + advisory lock (auto-grade itu sendiri di
//     5.D.4; di sini kita lock untuk single in-flight attempt).
package soalbab

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors specific to ulangan flow.
var (
	// ErrUlanganSettingMissing — siswa start ulangan tapi guru belum PUT
	// UlanganBabSetting (Task 5.C.1). FE harus block tombol Mulai sampai
	// guru publish setting.
	ErrUlanganSettingMissing = errors.New("soalbab: ulangan setting belum diset guru")
	// ErrBatasAttemptExceeded — siswa habis kuota attempt.
	ErrBatasAttemptExceeded = errors.New("soalbab: batas attempt terlampaui")
	// ErrUlanganPoolInsufficient — pool soal mode IN ('ulangan','keduanya')
	// kurang dari Setting.JumlahSoal. Beda dengan ErrSettingPoolExceeded
	// (validate saat PUT setting): di sini guru pernah valid setting
	// tapi kemudian delete soal sehingga pool shrunk.
	ErrUlanganPoolInsufficient = errors.New("soalbab: ulangan pool insufficient")
)

// UlanganRepoAPI is the subset of *Repo Ulangan service depends on.
//
// Service-layer transactions need direct DB() access for the advisory
// lock (pg_advisory_xact_lock). Repo exposes DB() already.
type UlanganRepoAPI interface {
	DB() *gorm.DB
	GetSettingByBab(ctx context.Context, babID uuid.UUID) (*UlanganBabSetting, error)
	ListSoalByBab(ctx context.Context, babID uuid.UUID, f SoalListFilter) ([]SoalBab, error)
	FindActiveHasil(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (*HasilSoalBab, error)
	CountHasilByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (int64, error)
	CreateHasil(ctx context.Context, h *HasilSoalBab) error
	AppendEvent(ctx context.Context, e *EventBab) error
}

// UlanganService implements ulangan-bab start (5.D.1) — answer/submit/cron
// land in subsequent tasks but share the same service struct.
type UlanganService struct {
	repo  UlanganRepoAPI
	bab   babLookup
	enr   enrollmentLookup
	audit auditLogger
	now   func() time.Time
}

// NewUlanganService wires the ulangan service. enr verifies enrollment.
func NewUlanganService(repo UlanganRepoAPI, b babLookup, enr enrollmentLookup, audit auditLogger) *UlanganService {
	return &UlanganService{repo: repo, bab: b, enr: enr, audit: audit, now: time.Now}
}

// UlanganStartResult is the response payload for POST start. AttemptNo
// surfaced supaya FE bisa tampilin "Attempt 2 dari 3" di lobby.
type UlanganStartResult struct {
	HasilID      uuid.UUID   `json:"hasil_id"`
	SoalIDs      []uuid.UUID `json:"soal_ids"`
	Total        int         `json:"total"`
	MulaiAt      time.Time   `json:"mulai_at"`
	DeadlineAt   time.Time   `json:"deadline_at"`
	DurasiDetik  int         `json:"durasi_detik"`
	AttemptNo    int16       `json:"attempt_no"`
	BatasAttempt int16       `json:"batas_attempt"`
	Resume       bool        `json:"resume"`
}

// Start either resumes an in-progress ulangan attempt for (siswa, bab)
// or creates a new one with a deterministically-seeded snapshot of N
// random soal from the ulangan-eligible pool. Race-safe via pg advisory
// transaction lock keyed on (bab_id, siswa_id).
func (s *UlanganService) Start(ctx context.Context, babID, siswaID uuid.UUID, ip, userAgent string) (*UlanganStartResult, error) {
	b, err := s.requireSiswaBabAccess(ctx, babID, siswaID)
	if err != nil {
		return nil, err
	}

	setting, err := s.repo.GetSettingByBab(ctx, b.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUlanganSettingMissing
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab ulangan setting: %w", err)
	}

	// Single-flight via pg advisory_xact_lock keyed on (bab,siswa).
	// Two int64 keys derived from sha256(bab_id || siswa_id) split-half.
	// pg_advisory_xact_lock(int8, int8) takes 2 keys to dual-namespace
	// lock; release on tx commit/rollback automatically.
	k1, k2 := pairLockKeys(b.ID, siswaID)

	var result *UlanganStartResult
	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("soalbab ulangan tx begin: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Single-arg pg_advisory_xact_lock(bigint) — postgres' 2-arg form
	// takes (int4, int4) which can't fit a sha256-derived key. Use the
	// single int64 form with k1 (k2 ignored, kept for sanity).
	_ = k2
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", k1).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan advisory lock: %w", err)
	}

	// Re-check under lock: an active attempt for (siswa, bab, ulangan).
	var active HasilSoalBab
	err = tx.Where("bab_id = ? AND siswa_id = ? AND mode = ? AND status = ?",
		b.ID, siswaID, HasilModeUlangan, HasilBerlangsung).
		Order("mulai_at DESC").
		First(&active).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan active find: %w", err)
	}
	if err == nil {
		ids, perr := decodeSoalIDsJSON(active.SoalIDsJSON)
		if perr != nil {
			tx.Rollback()
			return nil, fmt.Errorf("soalbab ulangan resume decode: %w", perr)
		}
		if active.DeadlineAt == nil {
			tx.Rollback()
			return nil, errors.New("soalbab ulangan: berlangsung row missing deadline_at")
		}
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("soalbab ulangan commit resume: %w", err)
		}
		result = &UlanganStartResult{
			HasilID:      active.ID,
			SoalIDs:      ids,
			Total:        len(ids),
			MulaiAt:      active.MulaiAt,
			DeadlineAt:   *active.DeadlineAt,
			DurasiDetik:  int(active.DeadlineAt.Sub(active.MulaiAt).Seconds()),
			AttemptNo:    active.AttemptNo,
			BatasAttempt: setting.BatasAttempt,
			Resume:       true,
		}
		return result, nil
	}

	// New attempt path under lock: count attempts excluding dibatalkan
	// (locked #76 — soft cancel tidak count terhadap batas_attempt).
	var n int64
	if err := tx.Model(&HasilSoalBab{}).
		Where("bab_id = ? AND siswa_id = ? AND mode = ? AND status <> ?",
			b.ID, siswaID, HasilModeUlangan, HasilDibatalkan).
		Count(&n).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan count: %w", err)
	}
	attemptNo := int16(n + 1)
	if attemptNo > setting.BatasAttempt {
		tx.Rollback()
		return nil, fmt.Errorf("%w: attempt_no=%d, batas=%d",
			ErrBatasAttemptExceeded, attemptNo, setting.BatasAttempt)
	}

	// Build pool. List ulangan-eligible soal (mode IN
	// ('ulangan','keduanya')) and seed-shuffle then take JumlahSoal.
	var pool []SoalBab
	if err := tx.Where("bab_id = ? AND mode IN ?", b.ID,
		[]Mode{ModeUlangan, ModeKeduanya}).
		Order("id ASC"). // deterministic baseline before shuffle
		Find(&pool).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan pool: %w", err)
	}
	if int16(len(pool)) < setting.JumlahSoal {
		tx.Rollback()
		return nil, fmt.Errorf("%w: pool=%d, butuh=%d",
			ErrUlanganPoolInsufficient, len(pool), setting.JumlahSoal)
	}

	mulaiAt := s.now()
	seed := deriveSeed(mulaiAt, siswaID, b.ID)
	rng := rand.New(rand.NewSource(seed))
	// Fisher-Yates shuffle in place over pool's id list.
	ids := make([]uuid.UUID, len(pool))
	for i, p := range pool {
		ids[i] = p.ID
	}
	for i := len(ids) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}
	pickedIDs := ids[:setting.JumlahSoal]

	encoded, err := json.Marshal(pickedIDs)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan encode: %w", err)
	}
	deadline := mulaiAt.Add(time.Duration(setting.DurasiMenit) * time.Minute)
	hasil := &HasilSoalBab{
		BabID:       b.ID,
		SiswaID:     siswaID,
		Mode:        HasilModeUlangan,
		Status:      HasilBerlangsung,
		SoalIDsJSON: datatypes.JSON(encoded),
		MulaiAt:     mulaiAt,
		DeadlineAt:  &deadline,
		AttemptNo:   attemptNo,
	}
	if err := tx.Create(hasil).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan create hasil: %w", err)
	}
	if err := tx.Create(&EventBab{
		HasilID: hasil.ID,
		Action:  "ulangan_bab_started",
		Meta: marshalMeta(map[string]any{
			"bab_id":      b.ID.String(),
			"attempt_no":  attemptNo,
			"durasi_min":  setting.DurasiMenit,
			"jumlah_soal": setting.JumlahSoal,
		}),
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan event: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("soalbab ulangan commit: %w", err)
	}

	// Audit ledger (best-effort, post-commit).
	s.logAudit(ctx, "ulangan_bab_started", &siswaID, string(auth.Siswa), &hasil.ID, &b.KelasID, ip, userAgent, map[string]any{
		"hasil_id":    hasil.ID.String(),
		"bab_id":      b.ID.String(),
		"attempt_no":  attemptNo,
		"durasi_min":  setting.DurasiMenit,
		"jumlah_soal": setting.JumlahSoal,
		"deadline_at": deadline.UTC().Format(time.RFC3339Nano),
	})

	result = &UlanganStartResult{
		HasilID:      hasil.ID,
		SoalIDs:      pickedIDs,
		Total:        int(setting.JumlahSoal),
		MulaiAt:      mulaiAt,
		DeadlineAt:   deadline,
		DurasiDetik:  int(setting.DurasiMenit) * 60,
		AttemptNo:    attemptNo,
		BatasAttempt: setting.BatasAttempt,
		Resume:       false,
	}
	return result, nil
}

// requireSiswaBabAccess enforces (siswa enrolled active) ∩ (bab published).
// Mirrors LatihanService.requireSiswaBabAccess but lives here so the two
// flows can evolve independently.
func (s *UlanganService) requireSiswaBabAccess(ctx context.Context, babID, siswaID uuid.UUID) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab ulangan bab find: %w", err)
	}
	if b.Status != bab.StatusPublished {
		return nil, ErrNotFound
	}
	if s.enr == nil {
		return nil, ErrForbidden
	}
	enr, err := s.enr.FindEnrollment(ctx, b.KelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab ulangan enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}
	return b, nil
}

// deriveSeed implements locked #79: sha256(mulai_at_unix_micro_bytes ||
// siswa_id_bytes || bab_id_bytes)[:8] LE → int64. Deterministic per
// (mulai_at, siswa, bab) → resume bawa pool yang sama. Multi-attempt
// remedial bikin mulai_at baru → seed baru → pool baru.
func deriveSeed(mulaiAt time.Time, siswaID, babID uuid.UUID) int64 {
	h := sha256.New()
	var mb [8]byte
	binary.LittleEndian.PutUint64(mb[:], uint64(mulaiAt.UnixMicro()))
	h.Write(mb[:])
	sb, _ := siswaID.MarshalBinary()
	bb, _ := babID.MarshalBinary()
	h.Write(sb)
	h.Write(bb)
	digest := h.Sum(nil)
	u := binary.LittleEndian.Uint64(digest[:8])
	return int64(u) //nolint:gosec // intentional bit reinterpret
}

// pairLockKeys returns two int64 advisory-lock keys derived from
// sha256(bab_id || siswa_id). pg_advisory_xact_lock(int8, int8) accepts
// 2 keys; we split sha256 into two halves so siswa A starting on bab X
// doesn't block siswa B on the same bab X.
func pairLockKeys(babID, siswaID uuid.UUID) (int64, int64) {
	h := sha256.New()
	bb, _ := babID.MarshalBinary()
	sb, _ := siswaID.MarshalBinary()
	h.Write(bb)
	h.Write(sb)
	d := h.Sum(nil)
	a := int64(binary.LittleEndian.Uint64(d[:8]))   //nolint:gosec
	b := int64(binary.LittleEndian.Uint64(d[8:16])) //nolint:gosec
	return a, b
}

// logAudit mirrors Service.logAudit pattern but lives on UlanganService.
func (s *UlanganService) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "hasil_soal_bab"
	role := actorRole
	if actorID == nil {
		role = ""
	}
	entry := &auth.AuditLog{
		ActorID:       actorID,
		ActorRole:     ptrString(role),
		Action:        action,
		TargetType:    &targetType,
		TargetID:      targetID,
		TargetKelasID: targetKelasID,
		Meta:          marshalMeta(meta),
		IP:            ptrString(ip),
		UserAgent:     ptrString(userAgent),
		At:            s.now(),
	}
	_ = s.audit.LogAudit(ctx, entry)
}
