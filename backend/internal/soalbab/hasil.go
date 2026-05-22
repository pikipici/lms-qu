// Hasil + Review + Cancel (remedial) + Rekap service untuk Task 5.E.1.
//
// Endpoints (consolidated):
//
//   - GET  /api/v1/siswa/hasil-soal-bab/:id/review
//     Review jawaban siswa setelah submit. Gated locked #81:
//     mode=ulangan + status=selesai + setting.IzinkanReviewSetelahSubmit
//     + (setting.WaktuBukaReview NULL OR <= now). Latihan attempt
//     (mode=latihan) selalu boleh review (formative, no gating).
//
//   - GET  /api/v1/siswa/bab/:id/hasil
//     List semua hasil milik caller di bab tertentu (latihan + ulangan,
//     semua status). Buat siswa lobby/resume hint.
//
//   - POST /api/v1/hasil-soal-bab/:id/cancel
//     Guru/admin remedial reset. Soft-cancel mark Status='dibatalkan'
//     (locked #76 — tidak count terhadap batas_attempt). Hanya boleh
//     untuk attempt mode=ulangan (latihan tidak count).
//
//   - GET  /api/v1/bab/:id/hasil-rekap
//     Guru/admin rekap dashboard. List per-siswa attempts ulangan,
//     dengan nilai_terbaik + nilai_terakhir + jumlah_attempt + status.
//
// Auth matrix:
//   - Review/list-hasil: caller siswa, must own attempt + active enrollment.
//   - Cancel/rekap: caller guru (own kelas) atau admin.
package soalbab

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors for Hasil flow.
var (
	// ErrReviewLocked — review belum dibuka (gated by setting WaktuBukaReview).
	// Beda dengan ErrReviewDisabled — yang ini "belum waktunya".
	ErrReviewLocked = errors.New("soalbab: review belum dibuka")
	// ErrReviewDisabled — guru disable review setelah submit (locked #81).
	ErrReviewDisabled = errors.New("soalbab: review dimatikan guru")
	// ErrHasilNotFinished — review attempt yang belum status=selesai.
	ErrHasilNotFinished = errors.New("soalbab: hasil belum selesai")
	// ErrCancelLatihan — guru coba cancel attempt mode=latihan. Latihan
	// tidak counted sehingga cancel tidak meaningful.
	ErrCancelLatihan = errors.New("soalbab: latihan tidak perlu di-cancel")
)

// HasilRepoAPI is the subset of *Repo Hasil service depends on.
type HasilRepoAPI interface {
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilSoalBab, error)
	ListHasilBySiswaBab(ctx context.Context, siswaID, babID uuid.UUID) ([]HasilSoalBab, error)
	ListHasilByBab(ctx context.Context, babID uuid.UUID, f HasilListFilter) ([]HasilSoalBab, error)
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanBab, error)
	ListSoalByIDs(ctx context.Context, ids []uuid.UUID) ([]SoalBab, error)
	UpdateHasilStatus(ctx context.Context, hasilID uuid.UUID, status HasilStatus, selesaiAt *time.Time, nilaiTotal *float64, benar, total *int16) error
	GetSettingByBab(ctx context.Context, babID uuid.UUID) (*UlanganBabSetting, error)
	AppendEvent(ctx context.Context, e *EventBab) error
}

// userLookup hydrates user name/email untuk rekap dashboard.
type userLookup interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
}

// HasilService implements review + list + cancel + rekap.
type HasilService struct {
	repo  HasilRepoAPI
	bab   babLookup
	kelas kelasLookup
	enr   enrollmentLookup
	users userLookup
	audit auditLogger
	now   func() time.Time
}

// NewHasilService wires the hasil service. users + audit boleh nil
// (degrade — review tetap jalan, rekap tanpa nama siswa).
func NewHasilService(repo HasilRepoAPI, b babLookup, k kelasLookup, enr enrollmentLookup, users userLookup, audit auditLogger) *HasilService {
	return &HasilService{repo: repo, bab: b, kelas: k, enr: enr, users: users, audit: audit, now: time.Now}
}

// ---------------------------------------------------------------------------
// GET /siswa/hasil-soal-bab/:id/review
// ---------------------------------------------------------------------------

// ReviewItem represents one soal + jawaban siswa di review payload.
type ReviewItem struct {
	SoalID         uuid.UUID `json:"soal_id"`
	Pertanyaan     string    `json:"pertanyaan"`
	OpsiA          string    `json:"opsi_a"`
	OpsiB          string    `json:"opsi_b"`
	OpsiC          string    `json:"opsi_c"`
	OpsiD          string    `json:"opsi_d"`
	OpsiE          string    `json:"opsi_e"`
	JawabanBenar   Jawaban   `json:"jawaban_benar"`
	JawabanSiswa   *string   `json:"jawaban_siswa,omitempty"`
	IsBenar        *bool     `json:"is_benar,omitempty"`
	PoinDapat      int16     `json:"poin_dapat"`
	PoinMaksimal   int16     `json:"poin_maksimal"`
	Urutan         int       `json:"urutan"`
}

// ReviewResult is the full review payload.
type ReviewResult struct {
	HasilID           uuid.UUID    `json:"hasil_id"`
	BabID             uuid.UUID    `json:"bab_id"`
	Mode              HasilMode    `json:"mode"`
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
// Gating logic:
//   - Latihan: always reviewable once status=selesai (locked #81).
//   - Ulangan: status=selesai AND setting.IzinkanReviewSetelahSubmit
//     AND (setting.WaktuBukaReview is nil OR <= now).
func (s *HasilService) Review(ctx context.Context, hasilID, siswaID uuid.UUID) (*ReviewResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab review find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Status != HasilSelesai {
		return nil, ErrHasilNotFinished
	}

	// Ulangan: enforce review gating per setting.
	if hasil.Mode == HasilModeUlangan {
		setting, err := s.repo.GetSettingByBab(ctx, hasil.BabID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("soalbab review setting: %w", err)
		}
		if setting != nil {
			if !setting.IzinkanReviewSetelahSubmit {
				return nil, ErrReviewDisabled
			}
			if setting.WaktuBukaReview != nil && s.now().Before(*setting.WaktuBukaReview) {
				return nil, ErrReviewLocked
			}
		}
		// Setting hilang post-submit → fail-soft default izinkan=true,
		// no waktu_buka_review constraint (sama dengan migration default).
	}

	// Decode pool snapshot, hydrate soals + jawabans.
	pool, perr := decodeSoalIDsJSON(hasil.SoalIDsJSON)
	if perr != nil {
		return nil, fmt.Errorf("soalbab review pool decode: %w", perr)
	}
	soals, err := s.repo.ListSoalByIDs(ctx, pool)
	if err != nil {
		return nil, fmt.Errorf("soalbab review soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*SoalBab, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	jawabans, err := s.repo.ListJawabanByHasil(ctx, hasilID)
	if err != nil {
		return nil, fmt.Errorf("soalbab review jawabans: %w", err)
	}
	jawByID := make(map[uuid.UUID]*JawabanBab, len(jawabans))
	for i := range jawabans {
		jawByID[jawabans[i].SoalID] = &jawabans[i]
	}

	items := make([]ReviewItem, 0, len(pool))
	for idx, sid := range pool {
		soal, ok := soalByID[sid]
		if !ok {
			// Soal deleted post-snapshot — skip with placeholder.
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
		BabID:             hasil.BabID,
		Mode:              hasil.Mode,
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
// GET /siswa/bab/:id/hasil
// ---------------------------------------------------------------------------

// HasilSummary is one attempt entry (untuk list endpoints).
type HasilSummary struct {
	HasilID           uuid.UUID   `json:"hasil_id"`
	Mode              HasilMode   `json:"mode"`
	Status            HasilStatus `json:"status"`
	AttemptNo         int16       `json:"attempt_no"`
	NilaiTotal        *float64    `json:"nilai_total,omitempty"`
	JawabanBenarCount *int16      `json:"jawaban_benar_count,omitempty"`
	JawabanTotal      *int16      `json:"jawaban_total,omitempty"`
	MulaiAt           time.Time   `json:"mulai_at"`
	DeadlineAt        *time.Time  `json:"deadline_at,omitempty"`
	SelesaiAt         *time.Time  `json:"selesai_at,omitempty"`
}

// SiswaHasilListResult is the response for siswa list hasil.
type SiswaHasilListResult struct {
	BabID         uuid.UUID      `json:"bab_id"`
	NilaiTerbaik  *float64       `json:"nilai_terbaik,omitempty"`
	NilaiTerakhir *float64       `json:"nilai_terakhir,omitempty"`
	AttemptCount  int            `json:"attempt_count"`
	Items         []HasilSummary `json:"items"`
}

// ListSiswaHasil returns all hasil milik caller di bab. Buat siswa lobby
// / resume / lihat history attempt sendiri.
func (s *HasilService) ListSiswaHasil(ctx context.Context, babID, siswaID uuid.UUID) (*SiswaHasilListResult, error) {
	b, err := s.requireSiswaBabAccess(ctx, babID, siswaID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.ListHasilBySiswaBab(ctx, siswaID, b.ID)
	if err != nil {
		return nil, fmt.Errorf("soalbab list siswa hasil: %w", err)
	}

	out := &SiswaHasilListResult{BabID: b.ID, Items: make([]HasilSummary, 0, len(rows))}
	var bestNilai *float64
	var lastNilaiUlangan *float64
	var lastUlanganMulai time.Time
	attemptUlangan := 0
	for i := range rows {
		r := &rows[i]
		out.Items = append(out.Items, HasilSummary{
			HasilID:           r.ID,
			Mode:              r.Mode,
			Status:            r.Status,
			AttemptNo:         r.AttemptNo,
			NilaiTotal:        r.NilaiTotal,
			JawabanBenarCount: r.JawabanBenarCount,
			JawabanTotal:      r.JawabanTotal,
			MulaiAt:           r.MulaiAt,
			DeadlineAt:        r.DeadlineAt,
			SelesaiAt:         r.SelesaiAt,
		})
		// Hitung nilai_terbaik + nilai_terakhir hanya untuk ulangan
		// status=selesai (locked: dibatalkan + latihan tidak count).
		if r.Mode != HasilModeUlangan || r.Status != HasilSelesai || r.NilaiTotal == nil {
			continue
		}
		attemptUlangan++
		if bestNilai == nil || *r.NilaiTotal > *bestNilai {
			v := *r.NilaiTotal
			bestNilai = &v
		}
		if lastNilaiUlangan == nil || r.MulaiAt.After(lastUlanganMulai) {
			v := *r.NilaiTotal
			lastNilaiUlangan = &v
			lastUlanganMulai = r.MulaiAt
		}
	}
	out.NilaiTerbaik = bestNilai
	out.NilaiTerakhir = lastNilaiUlangan
	out.AttemptCount = attemptUlangan
	return out, nil
}

// ---------------------------------------------------------------------------
// POST /hasil-soal-bab/:id/cancel  (guru/admin remedial)
// ---------------------------------------------------------------------------

// CancelResult is the response payload after soft-cancel.
type CancelResult struct {
	HasilID    uuid.UUID   `json:"hasil_id"`
	BabID      uuid.UUID   `json:"bab_id"`
	SiswaID    uuid.UUID   `json:"siswa_id"`
	Status     HasilStatus `json:"status"`
	AttemptNo  int16       `json:"attempt_no"`
	CancelledAt time.Time  `json:"cancelled_at"`
}

// Cancel soft-cancels an ulangan attempt for remedial reset (locked #76).
// Status='dibatalkan' tidak count terhadap batas_attempt — siswa boleh
// start fresh attempt dengan attempt_no = sebelumnya - cancelled_count + 1.
//
// Idempotent: kalau attempt sudah dibatalkan, return existing dengan ok=true.
// Tidak boleh cancel attempt mode=latihan (latihan tidak count anyway →
// remedial tidak meaningful; return ErrCancelLatihan 400).
func (s *HasilService) Cancel(ctx context.Context, hasilID, callerID uuid.UUID, callerRole, ip, userAgent string) (*CancelResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab cancel find: %w", err)
	}
	if hasil.Mode != HasilModeUlangan {
		return nil, ErrCancelLatihan
	}

	// Auth: guru pemilik kelas (via bab→kelas) atau admin.
	b, err := s.bab.FindByID(ctx, hasil.BabID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab cancel bab: %w", err)
	}
	k, err := s.kelas.FindByID(ctx, b.KelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab cancel kelas: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}

	// Idempotent: status sudah dibatalkan → return existing.
	if hasil.Status == HasilDibatalkan {
		cancelledAt := time.Time{}
		if hasil.SelesaiAt != nil {
			cancelledAt = *hasil.SelesaiAt
		} else {
			cancelledAt = hasil.UpdatedAt
		}
		return &CancelResult{
			HasilID:     hasil.ID,
			BabID:       hasil.BabID,
			SiswaID:     hasil.SiswaID,
			Status:      hasil.Status,
			AttemptNo:   hasil.AttemptNo,
			CancelledAt: cancelledAt,
		}, nil
	}

	now := s.now()
	if err := s.repo.UpdateHasilStatus(ctx, hasil.ID, HasilDibatalkan, &now, nil, nil, nil); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("soalbab cancel update: %w", err)
	}

	// Append EventBab + audit (best-effort).
	prevStatus := string(hasil.Status)
	_ = s.repo.AppendEvent(ctx, &EventBab{
		HasilID: hasil.ID,
		Action:  "ulangan_bab_cancelled",
		Meta: marshalMeta(map[string]any{
			"prev_status": prevStatus,
			"reason":      "remedial",
			"cancelled_by": callerID.String(),
			"cancelled_at": now.UTC().Format(time.RFC3339Nano),
		}),
	})
	s.logAudit(ctx, "ulangan_bab_cancelled", &callerID, callerRole, &hasil.ID, &b.KelasID, ip, userAgent, map[string]any{
		"hasil_id":     hasil.ID.String(),
		"bab_id":       hasil.BabID.String(),
		"siswa_id":     hasil.SiswaID.String(),
		"prev_status":  prevStatus,
		"prev_attempt": hasil.AttemptNo,
		"cancelled_at": now.UTC().Format(time.RFC3339Nano),
	})

	return &CancelResult{
		HasilID:     hasil.ID,
		BabID:       hasil.BabID,
		SiswaID:     hasil.SiswaID,
		Status:      HasilDibatalkan,
		AttemptNo:   hasil.AttemptNo,
		CancelledAt: now,
	}, nil
}

// ---------------------------------------------------------------------------
// GET /bab/:id/hasil-rekap  (guru/admin dashboard)
// ---------------------------------------------------------------------------

// SiswaRekap is one row in rekap dashboard (per-siswa aggregate).
type SiswaRekap struct {
	SiswaID         uuid.UUID  `json:"siswa_id"`
	SiswaName       string     `json:"siswa_name"`
	SiswaEmail      string     `json:"siswa_email"`
	AttemptCount    int        `json:"attempt_count"`     // excluding dibatalkan
	CancelledCount  int        `json:"cancelled_count"`
	NilaiTerbaik    *float64   `json:"nilai_terbaik,omitempty"`
	NilaiTerakhir   *float64   `json:"nilai_terakhir,omitempty"`
	StatusTerakhir  string     `json:"status_terakhir,omitempty"`
	MulaiTerakhirAt *time.Time `json:"mulai_terakhir_at,omitempty"`
	HasilTerakhirID *uuid.UUID `json:"hasil_terakhir_id,omitempty"`
}

// RekapResult is the response for guru rekap.
type RekapResult struct {
	BabID    uuid.UUID    `json:"bab_id"`
	Total    int          `json:"total"`
	RataRata *float64     `json:"rata_rata,omitempty"` // average nilai_terbaik per siswa
	Items    []SiswaRekap `json:"items"`
}

// Rekap aggregates per-siswa attempts ulangan untuk guru dashboard.
// Hanya count mode=ulangan; latihan dipisahkan (formative).
func (s *HasilService) Rekap(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*RekapResult, error) {
	b, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole)
	if err != nil {
		return nil, err
	}

	rows, err := s.repo.ListHasilByBab(ctx, b.ID, HasilListFilter{Mode: HasilModeUlangan})
	if err != nil {
		return nil, fmt.Errorf("soalbab rekap list: %w", err)
	}

	type acc struct {
		attemptCount   int
		cancelledCount int
		best           *float64
		last           *float64
		lastStatus     string
		lastMulai      time.Time
		lastID         uuid.UUID
	}
	bySiswa := make(map[uuid.UUID]*acc, 16)
	siswaOrder := make([]uuid.UUID, 0, 16)

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

	// Hydrate names via userLookup (best-effort — kalau nil, kosongkan).
	out := &RekapResult{BabID: b.ID, Items: make([]SiswaRekap, 0, len(siswaOrder))}
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
		if s.users != nil {
			if u, err := s.users.FindUserByID(ctx, sid); err == nil && u != nil {
				row.SiswaName = u.Name
				row.SiswaEmail = u.Email
			}
		}
		out.Items = append(out.Items, row)
	}
	// Sort: nilai_terbaik DESC nulls last, then siswa_name ASC.
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
	out.Total = len(out.Items)

	// Rata-rata kelas dari nilai_terbaik per siswa (skip nil).
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

// requireSiswaBabAccess mirrors UlanganService.requireSiswaBabAccess.
func (s *HasilService) requireSiswaBabAccess(ctx context.Context, babID, siswaID uuid.UUID) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab hasil bab: %w", err)
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
		return nil, fmt.Errorf("soalbab hasil enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}
	return b, nil
}

// findBabAndOwnership mirrors Service.findBabAndOwnership.
func (s *HasilService) findBabAndOwnership(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab hasil bab: %w", err)
	}
	k, err := s.kelas.FindByID(ctx, b.KelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab hasil kelas: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return b, nil
}

// logAudit mirrors UlanganService.logAudit.
func (s *HasilService) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
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
