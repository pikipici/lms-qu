// Ujian flow service — Answer save endpoint (Task 6.D.2).
//
// Mirror SoalBab UlanganService.Answer (commit 5067f0a, locked #76)
// adapted untuk Ujian (single-mode, single-attempt, no remedial chain).
//
// Behavior — locked decisions:
//   - Delayed grade (locked #76 mirror, locked #87 cron auto-grade):
//     UPSERT JawabanUjian dengan IsBenar=NULL, PoinDapat=0.
//     Grading dilakukan saat submit (6.D.3) atau timer-expire cron
//     (6.D.4) dalam single-tx batch dengan advisory lock.
//   - Anti-cheat: soal_id MUST be in hasil.SoalIDsJSON snapshot
//     (frozen at start). Kalau guru ubah pool post-start, attempt
//     sudah punya snapshot beku jadi siswa nggak bisa exploit.
//   - Timer guard: now() ≤ deadline_at; otherwise refuse with
//     ErrUjianTimerExpired (HTTP 410 Gone). Cron auto-grade akan
//     eventually mark hasil 'selesai', tapi kita tetap reject di edge
//     biar nggak ada save-after-grade race.
//   - Status guard: HasilBerlangsung only. Selesai/dibatalkan reject
//     dengan ErrHasilNotActive (HTTP 410).
//   - Ownership: hasil.SiswaID == caller, otherwise ErrHasilNotOwned.
//   - Audit: best-effort EventUjian(action=answer_save).
//
// Endpoint:
//   - POST /api/v1/siswa/hasil-ujian/:id/answer
//     body: { "soal_id": "<uuid>", "jawaban": "a|b|c|d|e" }
package ujian

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/banksoal"
)

// ErrSoalNotInPool — siswa post answer untuk soal yang tidak masuk
// snapshot pool. Bisa terjadi kalau client bug atau attempt tampering.
var ErrSoalNotInPool = errors.New("ujian: soal not in attempt pool")

// AnswerInput is the resolved payload for the answer endpoint.
type AnswerInput struct {
	SoalID  uuid.UUID
	Jawaban banksoal.Jawaban
}

// SaveAnswer upserts a jawaban for an in-flight ujian attempt without
// revealing is_benar/jawaban_benar to the caller (locked #76 mirror).
// Grading dilakukan saat submit (6.D.3) atau cron auto-grade (6.D.4).
//
// Behavior:
//   - Validates ownership + status=berlangsung.
//   - Checks now() ≤ deadline_at; otherwise ErrUjianTimerExpired (HTTP 410).
//   - Anti-cheat: soal_id MUST be in hasil.SoalIDsJSON snapshot.
//   - UPSERT JawabanUjian: jawaban=letter, is_benar=NULL, poin_dapat=0.
//   - Appends EventUjian(action=answer_save).
func (s *FlowService) SaveAnswer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) error {
	if !in.Jawaban.Valid() {
		return fmt.Errorf("%w: jawaban must be a|b|c|d|e", ErrInvalidInput)
	}

	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("ujian answer find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return ErrHasilNotOwned
	}
	if hasil.Status != HasilBerlangsung {
		return ErrHasilNotActive
	}
	if hasil.DeadlineAt == nil {
		return errors.New("ujian answer: hasil missing deadline_at")
	}
	if !s.now().Before(*hasil.DeadlineAt) {
		return ErrUjianTimerExpired
	}

	pool, perr := decodeSoalIDsJSONUjian(hasil.SoalIDsJSON)
	if perr != nil {
		return fmt.Errorf("ujian answer pool decode: %w", perr)
	}
	if !containsUUIDUjian(pool, in.SoalID) {
		return ErrSoalNotInPool
	}

	jawabanLower := banksoal.Jawaban(strings.ToLower(string(in.Jawaban)))
	jawabanStr := string(jawabanLower)
	now := s.now()
	row := &JawabanUjian{
		HasilID: hasil.ID,
		SoalID:  in.SoalID,
		Jawaban: &jawabanStr,
		// IsBenar nil → grading delayed sampai submit/auto-grade.
		IsBenar:    nil,
		PoinDapat:  0,
		AnsweredAt: now,
	}
	if err := s.repo.UpsertJawaban(ctx, row); err != nil {
		return fmt.Errorf("ujian answer upsert: %w", err)
	}
	_ = s.repo.AppendEvent(ctx, &EventUjian{
		HasilID: hasil.ID,
		Action:  "answer_save",
		Meta: marshalMeta(map[string]any{
			"soal_id": in.SoalID.String(),
			"jawaban": jawabanStr,
		}),
	})
	return nil
}
