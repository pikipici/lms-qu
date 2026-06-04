// Hasil + Review + Cancel (remedial) + Rekap service untuk Task 6.E.1.
// Mirror soalbab/hasil.go (commit 8c55651) adapted untuk Ujian.
//
// Key adaptations vs SoalBab:
//   - Review gating: Ujian punya field embedded (IzinkanReviewSetelahSubmit
//   - WaktuBukaReview) bukan separate Setting model. Ujian
//     SELALU graded (no latihan/ulangan mode discrimination).
//   - Scope: UjianID-based (per-instance) bukan BabID-based. Cross-kelas
//     siswa list pakai kelas_id JOIN ujian.kelas_id.
//   - Source soal: BankSoal (bank.FindSoalsByIDs) bukan SoalBab.
//   - Auth: guru pemilik kelas via ujian.KelasID → kelas.GuruID.
//
// Endpoints (consolidated):
//
//   - GET  /api/v1/siswa/hasil-ujian/:id/review
//     Review jawaban siswa setelah submit. Gated by ujian.
//     IzinkanReviewSetelahSubmit + ujian.WaktuBukaReview (locked #81).
//
//   - GET  /api/v1/siswa/kelas/:id/ujian/hasil
//     List semua hasil milik caller di kelas tertentu (cross-ujian).
//     Buat siswa lobby/history per kelas.
//
//   - POST /api/v1/hasil-ujian/:id/cancel
//     Guru/admin remedial soft-cancel. Status='dibatalkan' + DeletedAt
//     supaya partial-unique (ujian_id, siswa_id) WHERE deleted_at IS
//     NULL membebaskan slot — siswa bisa start fresh attempt.
//
//   - GET  /api/v1/ujian/:id/hasil-rekap
//     Guru/admin rekap dashboard. Per-siswa attempts ujian, dengan
//     nilai_terbaik + nilai_terakhir + jumlah_attempt + status.
package ujian

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors specific to Hasil flow (review/cancel/rekap).
var (
	// ErrReviewLocked — review belum dibuka (gated by ujian.WaktuBukaReview).
	// Beda dengan ErrReviewDisabled — yang ini "belum waktunya".
	ErrReviewLocked = errors.New("ujian: review belum dibuka")
	// ErrReviewDisabled — guru disable review setelah submit (locked #81).
	ErrReviewDisabled = errors.New("ujian: review dimatikan guru")
	// ErrHasilNotFinished — review attempt yang belum status=selesai.
	ErrHasilNotFinished = errors.New("ujian: hasil belum selesai")
	// ErrHasilAlreadyCancelled — guru coba cancel attempt yang sudah dibatalkan
	// (idempotent path returns existing snapshot, this sentinel jarang dipakai
	// — kept for symmetry with future strict-mode endpoints).
	ErrHasilAlreadyCancelled = errors.New("ujian: hasil sudah dibatalkan")
)

// HasilRepoAPI is the subset of *Repo Hasil service depends on.
type HasilRepoAPI interface {
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilUjian, error)
	FindUjianByID(ctx context.Context, id uuid.UUID) (*Ujian, error)
	ListHasilBySiswaAllKelas(ctx context.Context, kelasID, siswaID uuid.UUID) ([]HasilUjian, error)
	ListHasilByUjian(ctx context.Context, ujianID uuid.UUID) ([]HasilUjian, error)
	ListActiveRombelBySiswa(ctx context.Context, siswaIDs []uuid.UUID, sekolahID *uuid.UUID) ([]RombelLookupRow, error)
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanUjian, error)
	UpdateHasilStatus(ctx context.Context, hasilID uuid.UUID, status HasilStatus, selesaiAt *time.Time, deletedAt *time.Time) error
	AppendEvent(ctx context.Context, e *EventUjian) error
}

// userLookup hydrates user name/email untuk rekap dashboard.
type userLookup interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
}

// hasilBankLookup loads BankSoal by ids untuk review render.
type hasilBankLookup interface {
	FindSoalsByIDs(ctx context.Context, ids []uuid.UUID) ([]banksoal.BankSoal, error)
}

// HasilService implements review + list + cancel + rekap for Ujian.
type HasilService struct {
	repo  HasilRepoAPI
	bank  hasilBankLookup
	kelas kelasLookup
	enr   enrollmentLookup
	users userLookup
	audit auditLogger
	now   func() time.Time
}

// NewHasilService wires the hasil service. users + audit boleh nil
// (degrade — review tetap jalan, rekap tanpa nama siswa).
func NewHasilService(repo HasilRepoAPI, bank hasilBankLookup, k kelasLookup, enr enrollmentLookup, users userLookup, audit auditLogger) *HasilService {
	return &HasilService{repo: repo, bank: bank, kelas: k, enr: enr, users: users, audit: audit, now: time.Now}
}

// ---------------------------------------------------------------------------
// GET /siswa/hasil-ujian/:id/review
// ---------------------------------------------------------------------------

// ReviewItem represents one soal + jawaban siswa di review payload.
type ReviewItem struct {
	SoalID       uuid.UUID        `json:"soal_id"`
	Pertanyaan   string           `json:"pertanyaan"`
	OpsiA        string           `json:"opsi_a"`
	OpsiB        string           `json:"opsi_b"`
	OpsiC        string           `json:"opsi_c"`
	OpsiD        string           `json:"opsi_d"`
	OpsiE        string           `json:"opsi_e"`
	JawabanBenar banksoal.Jawaban `json:"jawaban_benar"`
	JawabanSiswa *string          `json:"jawaban_siswa,omitempty"`
	IsBenar      *bool            `json:"is_benar,omitempty"`
	PoinDapat    int16            `json:"poin_dapat"`
	PoinMaksimal int16            `json:"poin_maksimal"`
	Urutan       int              `json:"urutan"`
}

// ReviewResult is the full review payload.
type ReviewResult struct {
	HasilID           uuid.UUID    `json:"hasil_id"`
	UjianID           uuid.UUID    `json:"ujian_id"`
	Status            HasilStatus  `json:"status"`
	AttemptNo         int16        `json:"attempt_no"`
	NilaiTotal        *float64     `json:"nilai_total,omitempty"`
	JawabanBenarCount *int16       `json:"jawaban_benar_count,omitempty"`
	JawabanTotal      *int16       `json:"jawaban_total,omitempty"`
	MulaiAt           time.Time    `json:"mulai_at"`
	SelesaiAt         *time.Time   `json:"selesai_at,omitempty"`
	Items             []ReviewItem `json:"items"`
}

// Review returns the review payload for a finished attempt.
//
// Gating logic (Ujian SELALU graded, locked #85):
//   - status=selesai mandatory.
//   - ujian.IzinkanReviewSetelahSubmit must be true (default true)
//     → ErrReviewDisabled (403 review_disabled).
//   - ujian.WaktuBukaReview NULL or <= now → boleh review,
//     else ErrReviewLocked (403 review_locked).
func (s *HasilService) Review(ctx context.Context, hasilID, siswaID uuid.UUID) (*ReviewResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian review find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Status != HasilSelesai {
		return nil, ErrHasilNotFinished
	}

	// Load ujian for review-gating.
	u, uErr := s.repo.FindUjianByID(ctx, hasil.UjianID)
	if uErr != nil && !errors.Is(uErr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("ujian review ujian-load: %w", uErr)
	}
	if u != nil {
		if !u.IzinkanReviewSetelahSubmit {
			return nil, ErrReviewDisabled
		}
		if u.WaktuBukaReview != nil && s.now().Before(*u.WaktuBukaReview) {
			return nil, ErrReviewLocked
		}
	}
	// Ujian hilang post-submit → fail-soft default izinkan=true (rare:
	// guru hapus ujian setelah siswa submit; defensive).

	// Decode pool snapshot, hydrate soals + jawabans.
	pool, perr := decodeSoalIDsJSONUjian(hasil.SoalIDsJSON)
	if perr != nil {
		return nil, fmt.Errorf("ujian review pool decode: %w", perr)
	}
	soals, err := s.bank.FindSoalsByIDs(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("ujian review soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*banksoal.BankSoal, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	jawabans, err := s.repo.ListJawabanByHasil(ctx, hasilID)
	if err != nil {
		return nil, fmt.Errorf("ujian review jawabans: %w", err)
	}
	jawByID := make(map[uuid.UUID]*JawabanUjian, len(jawabans))
	for i := range jawabans {
		jawByID[jawabans[i].SoalID] = &jawabans[i]
	}

	items := make([]ReviewItem, 0, len(pool))
	for idx, sid := range pool {
		soal, ok := soalByID[sid]
		if !ok {
			// Soal soft-deleted post-snapshot → placeholder, NOT 500.
			items = append(items, ReviewItem{
				SoalID:     sid,
				Pertanyaan: "(soal sudah dihapus guru)",
				Urutan:     idx + 1,
			})
			continue
		}
		item := ReviewItem{
			SoalID:       soal.ID,
			Pertanyaan:   soal.Pertanyaan,
			OpsiA:        soal.OpsiA,
			OpsiB:        soal.OpsiB,
			OpsiC:        soal.OpsiC,
			OpsiD:        soal.OpsiD,
			OpsiE:        soal.OpsiE,
			JawabanBenar: soal.Jawaban,
			PoinMaksimal: soal.Poin,
			Urutan:       idx + 1,
		}
		if j, ok := jawByID[sid]; ok {
			item.JawabanSiswa = j.Jawaban
			item.IsBenar = j.IsBenar
			item.PoinDapat = j.PoinDapat
		}
		items = append(items, item)
	}

	return &ReviewResult{
		HasilID:           hasil.ID,
		UjianID:           hasil.UjianID,
		Status:            hasil.Status,
		AttemptNo:         hasil.AttemptNo,
		NilaiTotal:        hasil.NilaiTotal,
		JawabanBenarCount: hasil.JawabanBenarCount,
		JawabanTotal:      hasil.JawabanTotal,
		MulaiAt:           hasil.MulaiAt,
		SelesaiAt:         hasil.SelesaiAt,
		Items:             items,
	}, nil
}

// ---------------------------------------------------------------------------
// GET /siswa/kelas/:id/ujian/hasil
// ---------------------------------------------------------------------------

// HasilSummary is one attempt entry (untuk list endpoints).
type HasilSummary struct {
	HasilID           uuid.UUID   `json:"hasil_id"`
	UjianID           uuid.UUID   `json:"ujian_id"`
	Status            HasilStatus `json:"status"`
	AttemptNo         int16       `json:"attempt_no"`
	NilaiTotal        *float64    `json:"nilai_total,omitempty"`
	JawabanBenarCount *int16      `json:"jawaban_benar_count,omitempty"`
	JawabanTotal      *int16      `json:"jawaban_total,omitempty"`
	MulaiAt           time.Time   `json:"mulai_at"`
	DeadlineAt        *time.Time  `json:"deadline_at,omitempty"`
	SelesaiAt         *time.Time  `json:"selesai_at,omitempty"`
}

// SiswaHasilListResult is the response for siswa list hasil per kelas.
type SiswaHasilListResult struct {
	KelasID       uuid.UUID      `json:"kelas_id"`
	NilaiTerbaik  *float64       `json:"nilai_terbaik,omitempty"`
	NilaiTerakhir *float64       `json:"nilai_terakhir,omitempty"`
	AttemptCount  int            `json:"attempt_count"`
	Items         []HasilSummary `json:"items"`
}

// ListSiswaHasil returns all hasil milik caller di kelas (cross-ujian).
// Buat siswa lobby/resume/history. Aggregate (best/last/count) hanya
// dari attempts status=selesai (dibatalkan tidak count locked #76).
func (s *HasilService) ListSiswaHasil(ctx context.Context, kelasID, siswaID uuid.UUID) (*SiswaHasilListResult, error) {
	// Verify enrollment first.
	if _, err := s.kelas.FindByID(ctx, kelasID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("ujian list-siswa kelas: %w", err)
	}
	if _, err := s.enr.FindEnrollment(ctx, kelasID, siswaID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
		return nil, fmt.Errorf("ujian list-siswa enr: %w", err)
	}

	rows, err := s.repo.ListHasilBySiswaAllKelas(ctx, kelasID, siswaID)
	if err != nil {
		return nil, fmt.Errorf("ujian list siswa hasil: %w", err)
	}

	out := &SiswaHasilListResult{KelasID: kelasID, Items: make([]HasilSummary, 0, len(rows))}
	var bestNilai *float64
	var lastNilai *float64
	var lastMulai time.Time
	attemptCount := 0
	for i := range rows {
		r := &rows[i]
		out.Items = append(out.Items, HasilSummary{
			HasilID:           r.ID,
			UjianID:           r.UjianID,
			Status:            r.Status,
			AttemptNo:         r.AttemptNo,
			NilaiTotal:        r.NilaiTotal,
			JawabanBenarCount: r.JawabanBenarCount,
			JawabanTotal:      r.JawabanTotal,
			MulaiAt:           r.MulaiAt,
			DeadlineAt:        r.DeadlineAt,
			SelesaiAt:         r.SelesaiAt,
		})
		if r.Status != HasilSelesai || r.NilaiTotal == nil {
			continue
		}
		attemptCount++
		if bestNilai == nil || *r.NilaiTotal > *bestNilai {
			v := *r.NilaiTotal
			bestNilai = &v
		}
		if lastNilai == nil || r.MulaiAt.After(lastMulai) {
			v := *r.NilaiTotal
			lastNilai = &v
			lastMulai = r.MulaiAt
		}
	}
	out.NilaiTerbaik = bestNilai
	out.NilaiTerakhir = lastNilai
	out.AttemptCount = attemptCount
	return out, nil
}

// ---------------------------------------------------------------------------
// POST /hasil-ujian/:id/cancel  (guru/admin remedial)
// ---------------------------------------------------------------------------

// CancelResult is the response payload after soft-cancel.
type CancelResult struct {
	HasilID     uuid.UUID   `json:"hasil_id"`
	UjianID     uuid.UUID   `json:"ujian_id"`
	SiswaID     uuid.UUID   `json:"siswa_id"`
	Status      HasilStatus `json:"status"`
	AttemptNo   int16       `json:"attempt_no"`
	CancelledAt time.Time   `json:"cancelled_at"`
}

// Cancel soft-cancels an ujian attempt for remedial reset (locked #76).
// Status='dibatalkan' + DeletedAt — partial-unique (ujian_id, siswa_id)
// WHERE deleted_at IS NULL releases slot, siswa can start fresh attempt.
//
// Idempotent: kalau attempt sudah dibatalkan, return existing snapshot
// (NOT 409 — fast retry path on network jitter aman).
func (s *HasilService) Cancel(ctx context.Context, hasilID, callerID uuid.UUID, callerRole, ip, userAgent string) (*CancelResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian cancel find: %w", err)
	}

	// Auth: guru pemilik kelas (via ujian→kelas) atau admin.
	u, err := s.repo.FindUjianByID(ctx, hasil.UjianID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian cancel ujian-load: %w", err)
	}
	k, err := s.kelas.FindByID(ctx, u.KelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("ujian cancel kelas-load: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}

	// Idempotent fast-path: already cancelled → return existing snapshot.
	if hasil.Status == HasilDibatalkan {
		cancelledAt := hasil.UpdatedAt
		if hasil.SelesaiAt != nil {
			cancelledAt = *hasil.SelesaiAt
		}
		return &CancelResult{
			HasilID:     hasil.ID,
			UjianID:     hasil.UjianID,
			SiswaID:     hasil.SiswaID,
			Status:      hasil.Status,
			AttemptNo:   hasil.AttemptNo,
			CancelledAt: cancelledAt,
		}, nil
	}

	now := s.now()
	prevStatus := hasil.Status
	if err := s.repo.UpdateHasilStatus(ctx, hasil.ID, HasilDibatalkan, &now, &now); err != nil {
		return nil, fmt.Errorf("ujian cancel update: %w", err)
	}

	_ = s.repo.AppendEvent(ctx, &EventUjian{
		HasilID: hasil.ID,
		Action:  "cancelled",
		Meta: marshalMeta(map[string]any{
			"prev_status":  string(prevStatus),
			"reason":       "remedial",
			"cancelled_by": callerID.String(),
			"cancelled_at": now.UTC().Format(time.RFC3339Nano),
		}),
	})
	s.logAudit(ctx, "ujian_cancelled", &callerID, callerRole, &hasil.ID, &u.KelasID, ip, userAgent, map[string]any{
		"hasil_id":    hasil.ID.String(),
		"ujian_id":    hasil.UjianID.String(),
		"siswa_id":    hasil.SiswaID.String(),
		"prev_status": string(prevStatus),
		"attempt_no":  hasil.AttemptNo,
	})

	return &CancelResult{
		HasilID:     hasil.ID,
		UjianID:     hasil.UjianID,
		SiswaID:     hasil.SiswaID,
		Status:      HasilDibatalkan,
		AttemptNo:   hasil.AttemptNo,
		CancelledAt: now,
	}, nil
}

// ---------------------------------------------------------------------------
// GET /ujian/:id/hasil-rekap (guru/admin)
// ---------------------------------------------------------------------------

// SiswaRekap is one row in rekap dashboard — per-siswa aggregate.
type SiswaRekap struct {
	SiswaID           uuid.UUID  `json:"siswa_id"`
	SiswaName         string     `json:"siswa_name"`
	SiswaEmail        string     `json:"siswa_email"`
	RombelID          *uuid.UUID `json:"rombel_id,omitempty"`
	RombelNama        string     `json:"rombel_nama,omitempty"`
	AttemptCount      int        `json:"attempt_count"` // excl. dibatalkan
	CancelledCount    int        `json:"cancelled_count"`
	NilaiTerbaik      *float64   `json:"nilai_terbaik,omitempty"`
	NilaiTerakhir     *float64   `json:"nilai_terakhir,omitempty"`
	StatusTerakhir    string     `json:"status_terakhir,omitempty"`
	HasilTerakhirID   *uuid.UUID `json:"hasil_terakhir_id,omitempty"`
	MulaiTerakhirAt   *time.Time `json:"mulai_terakhir_at,omitempty"`
	SelesaiTerakhirAt *time.Time `json:"selesai_terakhir_at,omitempty"`
}

// RekapResult is the response payload for rekap dashboard.
type RekapResult struct {
	UjianID  uuid.UUID    `json:"ujian_id"`
	Total    int          `json:"total"`
	RataRata *float64     `json:"rata_rata,omitempty"` // avg(nilai_terbaik), skip nil
	Items    []SiswaRekap `json:"items"`
}

// Rekap returns per-siswa aggregate dashboard for an ujian.
// Auth: guru pemilik kelas atau admin. Sort: nilai_terbaik DESC nulls
// last + name ASC tiebreak. Rata-rata = avg(nilai_terbaik) over students
// who finished at least once.
func (s *HasilService) Rekap(ctx context.Context, ujianID, callerID uuid.UUID, callerRole string) (*RekapResult, error) {
	u, err := s.repo.FindUjianByID(ctx, ujianID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian rekap ujian-load: %w", err)
	}
	k, err := s.kelas.FindByID(ctx, u.KelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("ujian rekap kelas-load: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}

	rows, err := s.repo.ListHasilByUjian(ctx, ujianID)
	if err != nil {
		return nil, fmt.Errorf("ujian rekap rows: %w", err)
	}

	type acc struct {
		attemptCount, cancelledCount int
		best, last                   *float64
		lastStatus                   string
		lastMulai                    time.Time
		lastSelesai                  time.Time
		lastID                       uuid.UUID
	}
	bySiswa := make(map[uuid.UUID]*acc)
	siswaOrder := make([]uuid.UUID, 0)

	for i := range rows {
		r := &rows[i]
		a, ok := bySiswa[r.SiswaID]
		if !ok {
			a = &acc{}
			bySiswa[r.SiswaID] = a
			siswaOrder = append(siswaOrder, r.SiswaID)
		}
		if r.Status == HasilDibatalkan {
			a.cancelledCount++
		} else {
			a.attemptCount++
		}
		if r.Status == HasilSelesai && r.NilaiTotal != nil {
			if a.best == nil || *r.NilaiTotal > *a.best {
				v := *r.NilaiTotal
				a.best = &v
			}
		}
		if r.Status == HasilSelesai && r.SelesaiAt != nil && (a.lastSelesai.IsZero() || r.SelesaiAt.After(a.lastSelesai)) {
			a.lastSelesai = *r.SelesaiAt
		}
		// last_*: most-recent mulai_at regardless of status
		if a.lastMulai.IsZero() || r.MulaiAt.After(a.lastMulai) {
			a.lastMulai = r.MulaiAt
			a.lastStatus = string(r.Status)
			a.lastID = r.ID
			if r.NilaiTotal != nil {
				v := *r.NilaiTotal
				a.last = &v
			} else {
				a.last = nil
			}
		}
	}

	rombelBySiswa := make(map[uuid.UUID]RombelLookupRow)
	rombelRows, err := s.repo.ListActiveRombelBySiswa(ctx, siswaOrder, k.SekolahID)
	if err != nil {
		return nil, fmt.Errorf("ujian rekap rombel lookup: %w", err)
	}
	for _, rr := range rombelRows {
		rombelBySiswa[rr.SiswaID] = rr
	}

	out := &RekapResult{UjianID: ujianID, Items: make([]SiswaRekap, 0, len(siswaOrder))}
	for _, sid := range siswaOrder {
		a := bySiswa[sid]
		row := SiswaRekap{
			SiswaID:        sid,
			AttemptCount:   a.attemptCount,
			CancelledCount: a.cancelledCount,
			NilaiTerbaik:   a.best,
			NilaiTerakhir:  a.last,
			StatusTerakhir: a.lastStatus,
		}
		if !a.lastMulai.IsZero() {
			t := a.lastMulai
			row.MulaiTerakhirAt = &t
			id := a.lastID
			row.HasilTerakhirID = &id
		}
		if !a.lastSelesai.IsZero() {
			t := a.lastSelesai
			row.SelesaiTerakhirAt = &t
		}
		// Hydrate siswa name/email; nil userLookup → empty (degrade).
		if s.users != nil {
			if u, err := s.users.FindUserByID(ctx, sid); err == nil && u != nil {
				row.SiswaName = u.Name
				row.SiswaEmail = u.Email
			}
		}
		if rr, ok := rombelBySiswa[sid]; ok {
			id := rr.RombelID
			row.RombelID = &id
			row.RombelNama = rr.RombelNama
		}
		out.Items = append(out.Items, row)
	}
	out.Total = len(out.Items)

	// Sort: *float64 DESC nulls last + name ASC tiebreak.
	sort.SliceStable(out.Items, func(i, j int) bool {
		ai, aj := out.Items[i], out.Items[j]
		ni, nj := ai.NilaiTerbaik, aj.NilaiTerbaik
		switch {
		case ni != nil && nj == nil:
			return true
		case ni == nil && nj != nil:
			return false
		case ni != nil && nj != nil && *ni != *nj:
			return *ni > *nj
		}
		return ai.SiswaName < aj.SiswaName
	})

	// Rata-rata = avg over students who have nilai_terbaik (not over total).
	var sum float64
	var n int
	for _, it := range out.Items {
		if it.NilaiTerbaik != nil {
			sum += *it.NilaiTerbaik
			n++
		}
	}
	if n > 0 {
		avg := sum / float64(n)
		out.RataRata = &avg
	}

	return out, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// logAudit on HasilService — same pattern as FlowService.logAudit.
func (s *HasilService) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "hasil_ujian"
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

// underscore-import avoidance: uuid package referenced via uuid.UUID type usage above.
var _ = kelas.Kelas{}
