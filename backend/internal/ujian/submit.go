// Ujian flow service — Submit endpoint (Task 6.D.3).
//
// Mirror SoalBab UlanganService.Submit (commit d262ea3, locked #87)
// adapted untuk Ujian (review gating embedded di Ujian itself, not
// separate Setting).
//
// Behavior — locked decisions:
//   - Single-tx auto-grade: re-grade tiap jawaban dengan banksoal.Jawaban
//     sebagai source of truth. is_benar = (jawaban == soal.jawaban),
//     poin_dapat = is_benar ? soal.poin : 0.
//   - pg_advisory_xact_lock(sha256("hasil-submit:"||hasil_id)[:8]) —
//     race-safe vs cron auto-grade (6.D.4 reuse same key locked #87).
//   - Idempotent: kalau status=selesai & nilai_total != nil, balikin
//     existing rekap dengan already_submitted=true. No relock.
//   - Late-submit grace 5s past deadline. Beyond that → 410
//     submit_after_grace (cron 30s should have handled it by then).
//   - Cancelled (Status='dibatalkan') → 409 hasil_cancelled.
//   - Snapshot pool: hasil.SoalIDsJSON; jawaban yang skip → is_benar=
//     false, poin=0 (defensive — siswa tidak rugi kalau tidak jawab).
//   - Soal soft-deleted post-snapshot → treat as wrong (defensive).
//   - UPDATE HasilUjian (status=selesai, selesai_at, nilai_total,
//     jawaban_benar_count, jawaban_total) + Append EventUjian
//     (action='submit') in-tx.
//   - Audit log post-commit, best-effort.
//
// Endpoint:
//   - POST /api/v1/siswa/hasil-ujian/:id/submit
package ujian

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/banksoal"
)

// Sentinel errors specific to Submit flow (auto-grade race + idempotency).
var (
	// ErrUjianSubmitAfterGrace — siswa POST submit > deadline_at + 5s.
	// Cron auto-grade harusnya sudah handle by then (locked #87 cron
	// tick 30s); kita refuse di edge supaya FE refresh ke rekap GET.
	ErrUjianSubmitAfterGrace = errors.New("ujian: submit after grace")
	// ErrHasilCancelled — guru/admin cancel attempt → siswa tidak bisa
	// submit (HTTP 409). Reset path lewat 6.E.1 cancel endpoint.
	ErrHasilCancelled = errors.New("ujian: hasil cancelled")
)

// SubmitResult is the rekap nilai response after submit.
//
// DapatReviewAt populated dari Ujian.WaktuBukaReview (kalau set).
// IzinkanReview = Ujian.IzinkanReviewSetelahSubmit.
type SubmitResult struct {
	HasilID           uuid.UUID  `json:"hasil_id"`
	NilaiTotal        float64    `json:"nilai_total"`
	JawabanBenarCount int16      `json:"jawaban_benar_count"`
	JawabanTotal      int16      `json:"jawaban_total"`
	SelesaiAt         time.Time  `json:"selesai_at"`
	DapatReviewAt     *time.Time `json:"dapat_review_at,omitempty"`
	IzinkanReview     bool       `json:"izinkan_review"`
	AlreadySubmitted  bool       `json:"already_submitted"`
}

// Submit grades all jawaban for an in-flight ujian attempt, persists
// nilai_total, dan close attempt jadi status='selesai'. Idempotent —
// kalau attempt sudah selesai, balikin existing rekap dengan
// already_submitted=true.
//
// Behavior:
//   - tx + pg_advisory_xact_lock per hasil_id (sha256("hasil-submit:"
//     ||hasil_id)[:8]). Race vs cron auto-grade safe (6.D.4 sama key).
//   - now() ≤ deadline_at + 5s grace.
//   - Re-grade pakai banksoal.Jawaban as source of truth.
//   - Audit submit, append EventUjian.
func (s *FlowService) Submit(ctx context.Context, hasilID, siswaID uuid.UUID, ip, userAgent string) (*SubmitResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian submit find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Status == HasilDibatalkan {
		return nil, ErrHasilCancelled
	}
	if hasil.DeadlineAt == nil {
		return nil, errors.New("ujian submit: hasil missing deadline_at")
	}

	// Idempotent path BEFORE entering tx — kalau sudah selesai +
	// nilai_total set, return existing snapshot (no relock).
	if hasil.Status == HasilSelesai && hasil.NilaiTotal != nil {
		return s.buildSubmitResult(ctx, hasil, true)
	}

	// Late submit grace: 5s past deadline.
	grace := 5 * time.Second
	if s.now().After(hasil.DeadlineAt.Add(grace)) {
		return nil, ErrUjianSubmitAfterGrace
	}

	// Single-flight per hasil_id via pg advisory_xact_lock(bigint).
	// Cron 6.D.4 reuse this key supaya siswa & cron mutex pada row
	// yang sama (locked #87).
	key := submitLockKey(hasilID)

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("ujian submit tx begin: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", key).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit advisory lock: %w", err)
	}

	// Re-fetch hasil under lock — cron mungkin baru saja auto-grade.
	var locked HasilUjian
	if err := tx.Where("id = ?", hasilID).First(&locked).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ujian submit reload: %w", err)
	}
	if locked.Status == HasilDibatalkan {
		tx.Rollback()
		return nil, ErrHasilCancelled
	}
	if locked.Status == HasilSelesai && locked.NilaiTotal != nil {
		// Idempotent: cron beat us to it, atau user double-clicked.
		// Commit empty tx (lock release) and return existing.
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("ujian submit commit idempotent: %w", err)
		}
		return s.buildSubmitResult(ctx, &locked, true)
	}

	// Load Ujian for review-gating. Fail-soft (kalau guru hapus ujian
	// post-start, defaults masih reasonable).
	u, uErr := s.ujian.FindUjianByID(ctx, locked.UjianID)
	if uErr != nil && !errors.Is(uErr, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit ujian load: %w", uErr)
	}

	// Decode pool snapshot.
	pool, perr := decodeSoalIDsJSONUjian(locked.SoalIDsJSON)
	if perr != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit pool decode: %w", perr)
	}
	if len(pool) == 0 {
		tx.Rollback()
		return nil, errors.New("ujian submit: empty pool snapshot")
	}

	// Load soals (BankSoal) — source of truth for grading. Outside tx
	// karena BankSoal di domain berbeda, but data immutable per snapshot.
	soals, err := s.bank.FindSoalsByIDs(ctx, pool)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*banksoal.BankSoal, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	// Load jawabans in-tx for snapshot consistency.
	var jawabans []JawabanUjian
	if err := tx.Where("hasil_id = ?", hasilID).Find(&jawabans).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit jawabans: %w", err)
	}

	// Re-grade — shared helper used by Submit (siswa-driven) and
	// TimerCron.GradeExpiredHasil (cron-driven, 6.D.4 locked #87).
	benar, nilaiTotal, err := gradeAttemptInTx(tx, jawabans, soalByID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit: %w", err)
	}
	jumlahTotal := int16(len(pool))
	now := s.now()

	if err := tx.Model(&HasilUjian{}).
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
		return nil, fmt.Errorf("ujian submit update hasil: %w", err)
	}
	if err := tx.Create(&EventUjian{
		HasilID: hasilID,
		Action:  "submit",
		Meta: marshalMeta(map[string]any{
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"selesai_at":          now.UTC().Format(time.RFC3339Nano),
			"trigger":             "siswa",
		}),
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian submit event: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("ujian submit commit: %w", err)
	}

	// Audit (post-commit, best-effort).
	var kelasIDPtr *uuid.UUID
	if u != nil {
		kelasIDPtr = &u.KelasID
	}
	s.logAudit(ctx, "ujian_submitted", &siswaID, string(auth.Siswa), &hasilID, kelasIDPtr, ip, userAgent, map[string]any{
		"hasil_id":            hasilID.String(),
		"ujian_id":            locked.UjianID.String(),
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
	if u != nil {
		res.IzinkanReview = u.IzinkanReviewSetelahSubmit
		res.DapatReviewAt = u.WaktuBukaReview
	}
	return res, nil
}

// buildSubmitResult assembles the rekap response from a finalized hasil
// row. Re-fetches the Ujian separately so callers without ujian in
// scope can call this on the idempotent path.
func (s *FlowService) buildSubmitResult(ctx context.Context, h *HasilUjian, alreadySubmitted bool) (*SubmitResult, error) {
	if h == nil {
		return nil, errors.New("ujian submit result: nil hasil")
	}
	res := &SubmitResult{
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
	// Ujian fetch (review gating). Fail-soft default IzinkanReview=true
	// (sama dengan migration default IzinkanReviewSetelahSubmit=true).
	res.IzinkanReview = true
	if u, err := s.ujian.FindUjianByID(ctx, h.UjianID); err == nil {
		res.IzinkanReview = u.IzinkanReviewSetelahSubmit
		res.DapatReviewAt = u.WaktuBukaReview
	}
	return res, nil
}

// submitLockKey derives an int64 advisory-lock key for a single
// hasil_id submit/grade operation. Used by both Submit (6.D.3) and the
// cron auto-grade tick (6.D.4) so they're mutually exclusive on the
// same row (locked #87).
//
// Distinct domain prefix "hasil-submit:" supaya tidak collision dengan
// (ujian, siswa) start-key dari Start().
func submitLockKey(hasilID uuid.UUID) int64 {
	h := sha256.New()
	hb, _ := hasilID.MarshalBinary()
	h.Write([]byte("hasil-submit:"))
	h.Write(hb)
	d := h.Sum(nil)
	return int64(binary.LittleEndian.Uint64(d[:8])) //nolint:gosec
}
