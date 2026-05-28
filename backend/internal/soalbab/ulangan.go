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
	"strings"
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
	// ErrUlanganTimerExpired — siswa POST answer setelah deadline_at.
	// Cron auto-grade (5.D.4) bakal mark hasil selesai eventually, tapi
	// ujung autosave 5s harus refuse update biar nggak ada last-second
	// race dengan grade. HTTP 410 Gone.
	ErrUlanganTimerExpired = errors.New("soalbab: ulangan timer expired")
	// ErrUlanganAlreadySubmitted — siswa POST submit pada hasil yang sudah
	// selesai. Idempotent — service balikin existing rekap, tapi sentinel
	// dipakai handler kalau caller eksplisit minta error response. Kita
	// pakai 200 idempotent return existing kebanyakan kasus, sentinel cuma
	// kalau row dibatalkan/race weird.
	ErrUlanganAlreadySubmitted = errors.New("soalbab: ulangan already submitted")
	// ErrUlanganSubmitAfterGrace — siswa POST submit > deadline_at + 5s.
	// Cron auto-grade harusnya udah handle by then, tapi kalau cron belom
	// keburu siswa boleh submit dalam grace 5s untuk hindari race UI.
	// HTTP 410 Gone.
	ErrUlanganSubmitAfterGrace = errors.New("soalbab: ulangan submit after grace")
)

// UlanganRepoAPI is the subset of *Repo Ulangan service depends on.
//
// Service-layer transactions need direct DB() access for the advisory
// lock (pg_advisory_xact_lock). Repo exposes DB() already.
type UlanganRepoAPI interface {
	DB() *gorm.DB
	GetSettingByBab(ctx context.Context, babID uuid.UUID) (*UlanganBabSetting, error)
	ListSoalByBab(ctx context.Context, babID uuid.UUID, f SoalListFilter) ([]SoalBab, error)
	ListSoalByIDs(ctx context.Context, ids []uuid.UUID) ([]SoalBab, error)
	FindActiveHasil(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (*HasilSoalBab, error)
	CountHasilByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (int64, error)
	CreateHasil(ctx context.Context, h *HasilSoalBab) error
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilSoalBab, error)
	UpsertJawaban(ctx context.Context, j *JawabanBab) error
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanBab, error)
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
	HasilID          uuid.UUID   `json:"hasil_id"`
	SoalIDs          []uuid.UUID `json:"soal_ids"`
	Total            int         `json:"total"`
	MulaiAt          time.Time   `json:"mulai_at"`
	DeadlineAt       time.Time   `json:"deadline_at"`
	DurasiDetik      int         `json:"durasi_detik"`
	AttemptNo        int16       `json:"attempt_no"`
	BatasAttempt     int16       `json:"batas_attempt"`
	AttemptUnlimited bool        `json:"attempt_unlimited"`
	Resume           bool        `json:"resume"`
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
			HasilID:          active.ID,
			SoalIDs:          ids,
			Total:            len(ids),
			MulaiAt:          active.MulaiAt,
			DeadlineAt:       *active.DeadlineAt,
			DurasiDetik:      int(active.DeadlineAt.Sub(active.MulaiAt).Seconds()),
			AttemptNo:        active.AttemptNo,
			BatasAttempt:     setting.BatasAttempt,
			AttemptUnlimited: setting.AttemptUnlimited,
			Resume:           true,
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
	if !setting.AttemptUnlimited && attemptNo > setting.BatasAttempt {
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
		HasilID:          hasil.ID,
		SoalIDs:          pickedIDs,
		Total:            int(setting.JumlahSoal),
		MulaiAt:          mulaiAt,
		DeadlineAt:       deadline,
		DurasiDetik:      int(setting.DurasiMenit) * 60,
		AttemptNo:        attemptNo,
		BatasAttempt:     setting.BatasAttempt,
		AttemptUnlimited: setting.AttemptUnlimited,
		Resume:           false,
	}
	return result, nil
}

// Answer upserts a jawaban for an in-flight ulangan attempt without
// revealing is_benar/jawaban_benar to the caller — locked #76 — siswa
// hanya tahu jawabannya tersimpan; grading dilakukan saat submit
// (5.D.3) atau cron auto-grade (5.D.4) on timer expire.
//
// Behavior:
//   - Validates ownership + mode=ulangan + status=berlangsung.
//   - Checks now() ≤ deadline_at; otherwise returns ErrUlanganTimerExpired
//     (HTTP 410). Cron auto-grade akan eventually mark hasil 'selesai',
//     tapi kita tetap refuse di edge biar nggak ada race save-after-grade.
//   - Anti-cheat: soal_id MUST be in hasil.SoalIDsJSON snapshot.
//   - UPSERT JawabanBab: jawaban=letter, is_benar=NULL, poin_dapat=0.
//     Late-grade nanti bakal recompute is_benar+poin saat submit/cron.
//   - Appends EventBab(action=answer_save, mode=ulangan).
func (s *UlanganService) Answer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) error {
	if !in.Jawaban.Valid() {
		return fmt.Errorf("%w: jawaban must be a|b|c|d|e", ErrInvalidInput)
	}

	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("soalbab ulangan answer find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return ErrHasilNotOwned
	}
	if hasil.Mode != HasilModeUlangan {
		return ErrHasilModeInvalid
	}
	if hasil.Status == HasilSelesai {
		return ErrHasilAlreadyFinished
	}
	if hasil.Status == HasilDibatalkan {
		return ErrHasilCancelled
	}
	if hasil.DeadlineAt == nil {
		return errors.New("soalbab ulangan: hasil missing deadline_at")
	}
	if !s.now().Before(*hasil.DeadlineAt) {
		return ErrUlanganTimerExpired
	}

	pool, perr := decodeSoalIDsJSON(hasil.SoalIDsJSON)
	if perr != nil {
		return fmt.Errorf("soalbab ulangan answer pool decode: %w", perr)
	}
	if !containsUUID(pool, in.SoalID) {
		return ErrSoalNotInPool
	}

	jawabanLower := Jawaban(strings.ToLower(string(in.Jawaban)))
	jawabanStr := string(jawabanLower)
	now := s.now()
	row := &JawabanBab{
		HasilID: hasilID,
		SoalID:  in.SoalID,
		Jawaban: &jawabanStr,
		// IsBenar nil → grading delayed sampai submit/auto-grade (locked #76).
		IsBenar:    nil,
		PoinDapat:  0,
		AnsweredAt: now,
	}
	if err := s.repo.UpsertJawaban(ctx, row); err != nil {
		return fmt.Errorf("soalbab ulangan answer upsert: %w", err)
	}
	_ = s.repo.AppendEvent(ctx, &EventBab{
		HasilID: hasilID,
		Action:  "answer_save",
		Meta: marshalMeta(map[string]any{
			"soal_id": in.SoalID.String(),
			"jawaban": jawabanStr,
			"mode":    "ulangan",
		}),
	})
	return nil
}

// UlanganSubmitResult is the rekap nilai yang siswa dapat lihat setelah
// submit. dapat_review_at populated dari Setting.WaktuBukaReview kalau
// ada (locked #81 review gating); kalau nil = review available langsung
// (kecuali izinkan_review_setelah_submit=false → caller FE handle).
type UlanganSubmitResult struct {
	HasilID           uuid.UUID  `json:"hasil_id"`
	NilaiTotal        float64    `json:"nilai_total"`
	JawabanBenarCount int16      `json:"jawaban_benar_count"`
	JawabanTotal      int16      `json:"jawaban_total"`
	SelesaiAt         time.Time  `json:"selesai_at"`
	DapatReviewAt     *time.Time `json:"dapat_review_at,omitempty"`
	IzinkanReview     bool       `json:"izinkan_review"`
	AlreadySubmitted  bool       `json:"already_submitted"`
}

// Submit grades all jawaban for an in-flight ulangan attempt, persists
// nilai_total, dan close attempt jadi status='selesai'. Idempotent —
// kalau attempt sudah selesai, balikin existing rekap dengan
// already_submitted=true.
//
// Behavior:
//   - tx + pg_advisory_xact_lock per hasil_id (sha256(hasil_id)[:8]).
//     Race vs cron auto-grade (5.D.4) safe — siapa duluan dapet lock,
//     yang lain re-check status di dalam tx setelah lock acquire.
//   - now() ≤ deadline_at + 5s grace (locked policy: cron tick 30s,
//     siswa boleh submit late dalam 5s untuk hindari UI race).
//   - Re-grade tiap jawaban: is_benar = (jawaban == soal.jawaban),
//     poin_dapat = is_benar ? soal.poin : 0. Snapshot soal_ids dari
//     hasil.SoalIDsJSON, jawaban yang skip → is_benar=false poin=0.
//   - Audit ulangan_bab_submitted, append EventBab.
func (s *UlanganService) Submit(ctx context.Context, hasilID, siswaID uuid.UUID, ip, userAgent string) (*UlanganSubmitResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab ulangan submit find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Mode != HasilModeUlangan {
		return nil, ErrHasilModeInvalid
	}
	if hasil.Status == HasilDibatalkan {
		return nil, ErrHasilCancelled
	}
	if hasil.DeadlineAt == nil {
		return nil, errors.New("soalbab ulangan: hasil missing deadline_at")
	}
	// Idempotent path BEFORE entering tx — kalau status=selesai &
	// nilai_total != nil, return existing snapshot (no relock).
	if hasil.Status == HasilSelesai && hasil.NilaiTotal != nil {
		return s.buildSubmitResult(ctx, hasil, true)
	}

	// Late submit grace: 5s past deadline. Beyond that, cron auto-grade
	// kemungkinan udah handle (kalau belum, FE refresh trigger 410 yang
	// nanti diganti dengan rekap GET endpoint at 5.E).
	grace := time.Duration(5) * time.Second
	if s.now().After(hasil.DeadlineAt.Add(grace)) {
		return nil, ErrUlanganSubmitAfterGrace
	}

	// Single-flight per hasil_id via pg advisory_xact_lock(bigint).
	// Cron 5.D.4 menggunakan key yang sama supaya siswa & cron mutually
	// exclusive on the same hasil row.
	key := hasilLockKey(hasilID)

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("soalbab ulangan submit tx begin: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", key).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit advisory lock: %w", err)
	}

	// Re-fetch hasil under lock — cron mungkin baru saja auto-grade.
	var locked HasilSoalBab
	if err := tx.Where("id = ?", hasilID).First(&locked).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("soalbab ulangan submit reload: %w", err)
	}
	if locked.Status == HasilDibatalkan {
		tx.Rollback()
		return nil, ErrHasilCancelled
	}
	if locked.Status == HasilSelesai && locked.NilaiTotal != nil {
		// Idempotent: cron beat us to it, atau user double-clicked.
		// Commit empty tx (lock release) and return existing.
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("soalbab ulangan submit commit idempotent: %w", err)
		}
		return s.buildSubmitResult(ctx, &locked, true)
	}

	// Load setting for review-gating populating + final assertion that
	// pool ids masih bisa di-grade. Setting fetch oke fail-soft (kalau
	// guru hapus setting setelah start, kita tetap grade dengan snapshot
	// yang ada — start sudah enforce setting waktu pool dipilih).
	var setting *UlanganBabSetting
	if cfg, err := s.repo.GetSettingByBab(ctx, locked.BabID); err == nil {
		setting = cfg
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit setting: %w", err)
	}

	// Decode pool snapshot.
	pool, perr := decodeSoalIDsJSON(locked.SoalIDsJSON)
	if perr != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit pool decode: %w", perr)
	}
	if len(pool) == 0 {
		tx.Rollback()
		return nil, errors.New("soalbab ulangan submit: empty pool snapshot")
	}

	// Load answers (snapshot via tx). Soals dimuat di gradeAttemptInTx
	// supaya cron auto-grade (5.D.4) bisa pakai helper sama.
	var jawabans []JawabanBab
	if err := tx.Where("hasil_id = ?", hasilID).Find(&jawabans).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit jawabans: %w", err)
	}

	// Re-grade. UPDATE per row dalam tx — pool max 200 (locked bound).
	benar, nilaiTotal, err := gradeAttemptInTx(tx, hasilID, pool, jawabans)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	jumlahTotal := int16(len(pool))
	now := s.now()

	if err := tx.Model(&HasilSoalBab{}).
		Where("id = ?", hasilID).
		Updates(map[string]any{
			"status":              HasilSelesai,
			"selesai_at":          now,
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"updated_at":          gorm.Expr("now()"),
		}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit update hasil: %w", err)
	}
	if err := tx.Create(&EventBab{
		HasilID: hasilID,
		Action:  "ulangan_bab_submitted",
		Meta: marshalMeta(map[string]any{
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"selesai_at":          now.UTC().Format(time.RFC3339Nano),
		}),
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("soalbab ulangan submit event: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("soalbab ulangan submit commit: %w", err)
	}

	// Audit (post-commit, best-effort).
	s.logAudit(ctx, "ulangan_bab_submitted", &siswaID, string(auth.Siswa), &hasilID, &locked.BabID, ip, userAgent, map[string]any{
		"hasil_id":            hasilID.String(),
		"bab_id":              locked.BabID.String(),
		"attempt_no":          locked.AttemptNo,
		"nilai_total":         nilaiTotal,
		"jawaban_benar_count": benar,
		"jawaban_total":       jumlahTotal,
		"selesai_at":          now.UTC().Format(time.RFC3339Nano),
	})

	// Build result. Hydrate locked dengan recently-set fields supaya
	// buildSubmitResult lihat consistent state.
	locked.Status = HasilSelesai
	locked.SelesaiAt = &now
	locked.NilaiTotal = &nilaiTotal
	locked.JawabanBenarCount = &benar
	jt := jumlahTotal
	locked.JawabanTotal = &jt
	res, err := s.buildSubmitResult(ctx, &locked, false)
	if err != nil {
		return nil, err
	}
	if setting != nil {
		res.IzinkanReview = setting.IzinkanReviewSetelahSubmit
		res.DapatReviewAt = setting.WaktuBukaReview
	}
	return res, nil
}

// buildSubmitResult assembles the rekap response from a finalized hasil
// row. Re-fetches the setting separately so callers without a setting
// in scope can call this on the idempotent path.
func (s *UlanganService) buildSubmitResult(ctx context.Context, h *HasilSoalBab, alreadySubmitted bool) (*UlanganSubmitResult, error) {
	if h == nil {
		return nil, errors.New("soalbab ulangan submit result: nil hasil")
	}
	res := &UlanganSubmitResult{
		HasilID:          h.ID,
		AlreadySubmitted: alreadySubmitted,
	}
	if h.NilaiTotal != nil {
		res.NilaiTotal = *h.NilaiTotal
	}
	if h.JawabanBenarCount != nil {
		res.JawabanBenarCount = *h.JawabanBenarCount
	}
	if h.JawabanTotal != nil {
		res.JawabanTotal = *h.JawabanTotal
	}
	if h.SelesaiAt != nil {
		res.SelesaiAt = *h.SelesaiAt
	}
	// Setting fetch (review gating). Fail-soft: kalau setting hilang,
	// default IzinkanReview=true (sama dengan migration default).
	res.IzinkanReview = true
	if cfg, err := s.repo.GetSettingByBab(ctx, h.BabID); err == nil {
		res.IzinkanReview = cfg.IzinkanReviewSetelahSubmit
		res.DapatReviewAt = cfg.WaktuBukaReview
	}
	return res, nil
}

// hasilLockKey derives an int64 advisory-lock key for a single hasil_id.
// Used by both Submit (5.D.3) and the cron auto-grade tick (5.D.4) so
// they're mutually exclusive on the same row.
func hasilLockKey(hasilID uuid.UUID) int64 {
	h := sha256.New()
	hb, _ := hasilID.MarshalBinary()
	// Distinct domain prefix so this key never collides with the
	// (bab,siswa) start-key used by Start().
	h.Write([]byte("hasil-submit:"))
	h.Write(hb)
	d := h.Sum(nil)
	return int64(binary.LittleEndian.Uint64(d[:8])) //nolint:gosec
}

// gradeAttemptInTx loads soals for the snapshot pool and grades each
// jawaban in-place inside the supplied tx. Returns (benar_count,
// nilai_total). Shared helper antara Submit (5.D.3) and cron
// auto-grade (5.D.4) supaya logika sama persis.
//
// Behavior:
//   - Loads SoalBab rows by `pool` ids using the tx (snapshot consistency).
//   - For each jawaban: missing/empty jawaban string OR soal removed
//     post-snapshot → is_benar=false, poin=0 (defensive).
//   - Otherwise: is_benar = jawaban == soal.jawaban,
//     poin = soal.poin if benar else 0.
//   - UPDATE per row in tx; max 200 rows per locked bound.
func gradeAttemptInTx(tx *gorm.DB, hasilID uuid.UUID, pool []uuid.UUID, jawabans []JawabanBab) (int16, float64, error) {
	var soals []SoalBab
	if err := tx.Where("id IN ?", pool).Find(&soals).Error; err != nil {
		return 0, 0, fmt.Errorf("soalbab grade: load soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*SoalBab, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	var benar int16
	var nilaiTotal float64
	falseVal := false
	for i := range jawabans {
		j := &jawabans[i]
		soal, ok := soalByID[j.SoalID]
		if !ok || j.Jawaban == nil || *j.Jawaban == "" {
			j.IsBenar = &falseVal
			j.PoinDapat = 0
		} else {
			isBenar := Jawaban(*j.Jawaban) == soal.Jawaban
			j.IsBenar = &isBenar
			if isBenar {
				j.PoinDapat = soal.Poin
				benar++
				nilaiTotal += float64(soal.Poin)
			} else {
				j.PoinDapat = 0
			}
		}
		if err := tx.Model(&JawabanBab{}).
			Where("id = ?", j.ID).
			Updates(map[string]any{
				"is_benar":   j.IsBenar,
				"poin_dapat": j.PoinDapat,
			}).Error; err != nil {
			return 0, 0, fmt.Errorf("soalbab grade jawaban: %w", err)
		}
	}
	return benar, nilaiTotal, nil
}

// requireSiswaBabAccess enforces (siswa enrolled active) ∩ (bab published).
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
