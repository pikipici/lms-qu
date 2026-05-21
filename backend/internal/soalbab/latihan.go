// Latihan flow service for Task 5.C.2.
//
// Latihan adalah formative practice — siswa enrolled mengerjakan soal
// mode IN ('latihan','keduanya') tanpa nilai persist. Cocok buat warm-up
// sebelum ulangan: tiap jawaban langsung dapat is_benar feedback,
// re-attempt unlimited (locked #81).
//
// Endpoints:
//   - POST /api/v1/siswa/bab/:id/latihan/start  → bikin atau resume HasilSoalBab(mode=latihan)
//   - POST /api/v1/siswa/hasil-soal-bab/:id/answer → upsert JawabanBab + immediate is_benar
//   - POST /api/v1/siswa/hasil-soal-bab/:id/finish → close attempt, status=selesai, nilai NULL
//
// Authorization:
//   - Caller MUST be siswa with active enrollment in kelas pemilik bab.
//   - Bab MUST be status='published' (siswa-side hidden lain).
//   - Hasil MUST belong to caller siswa + mode='latihan' (cross-mode reject).
package soalbab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors for latihan flow. Maps to HTTP at handler boundary.
var (
	// ErrLatihanPoolEmpty — bab tidak punya soal mode IN ('latihan','keduanya').
	ErrLatihanPoolEmpty = errors.New("soalbab: latihan pool empty")
	// ErrBabNotPublished — siswa-side guard. Bab status draft/archived
	// looks "not found" from siswa POV (locked policy: hindari leak).
	ErrBabNotPublished = errors.New("soalbab: bab not published")
	// ErrHasilNotOwned — hasil id valid but caller bukan owner.
	ErrHasilNotOwned = errors.New("soalbab: hasil not owned by caller")
	// ErrHasilModeInvalid — caller hit answer/finish on a mode mismatch
	// (e.g. ulangan attempt via latihan endpoint or vice versa).
	ErrHasilModeInvalid = errors.New("soalbab: hasil mode mismatch")
	// ErrHasilAlreadyFinished — answer/finish on hasil already selesai.
	ErrHasilAlreadyFinished = errors.New("soalbab: hasil already finished")
	// ErrHasilCancelled — finish/answer on dibatalkan attempt (remedial reset).
	ErrHasilCancelled = errors.New("soalbab: hasil cancelled")
	// ErrSoalNotInPool — caller submitted a soal_id not part of the
	// snapshot for this attempt. Anti-cheat (locked #79).
	ErrSoalNotInPool = errors.New("soalbab: soal not in attempt pool")
)

// LatihanRepoAPI is the subset of *Repo Latihan service depends on.
type LatihanRepoAPI interface {
	ListSoalByBab(ctx context.Context, babID uuid.UUID, f SoalListFilter) ([]SoalBab, error)
	FindSoalByID(ctx context.Context, id uuid.UUID) (*SoalBab, error)
	ListSoalByIDs(ctx context.Context, ids []uuid.UUID) ([]SoalBab, error)

	CreateHasil(ctx context.Context, h *HasilSoalBab) error
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilSoalBab, error)
	FindActiveHasil(ctx context.Context, babID, siswaID uuid.UUID, mode HasilMode) (*HasilSoalBab, error)
	UpdateHasilStatus(ctx context.Context, hasilID uuid.UUID, status HasilStatus, selesaiAt *time.Time, nilaiTotal *float64, benar, total *int16) error

	UpsertJawaban(ctx context.Context, j *JawabanBab) error
	FindJawabanByHasilSoal(ctx context.Context, hasilID, soalID uuid.UUID) (*JawabanBab, error)
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanBab, error)

	AppendEvent(ctx context.Context, e *EventBab) error
}

// LatihanService implements the Latihan formative flow.
type LatihanService struct {
	repo  LatihanRepoAPI
	bab   babLookup
	enr   enrollmentLookup
	now   func() time.Time
	randN func(n int) int // injection seam for shuffle determinism in tests
}

// NewLatihanService wires the latihan service. enr verifies enrollment.
func NewLatihanService(repo LatihanRepoAPI, b babLookup, enr enrollmentLookup) *LatihanService {
	return &LatihanService{
		repo:  repo,
		bab:   b,
		enr:   enr,
		now:   time.Now,
		randN: rand.Intn,
	}
}

// StartResult is returned by Start. Resume=true berarti siswa baru saja
// melanjutkan attempt yang sudah ada.
type StartResult struct {
	HasilID  uuid.UUID   `json:"hasil_id"`
	SoalIDs  []uuid.UUID `json:"soal_ids"`
	Total    int         `json:"total"`
	MulaiAt  time.Time   `json:"mulai_at"`
	Resume   bool        `json:"resume"`
	// Jawaban: hasil_id-keyed map of { soal_id → answered jawaban }, supaya
	// resume bisa langsung pre-fill UI tanpa round-trip extra.
	Jawaban map[uuid.UUID]string `json:"jawaban,omitempty"`
}

// Start either resumes an in-progress latihan attempt for (siswa, bab)
// or creates a new one with a freshly shuffled snapshot of all
// latihan-eligible soal. Idempotent across refresh — concurrent Start
// calls return the same hasil row (FindActiveHasil first wins).
func (s *LatihanService) Start(ctx context.Context, babID, siswaID uuid.UUID) (*StartResult, error) {
	b, err := s.requireSiswaBabAccess(ctx, babID, siswaID)
	if err != nil {
		return nil, err
	}

	// Resume path: an active berlangsung attempt for this siswa+bab+latihan.
	active, err := s.repo.FindActiveHasil(ctx, b.ID, siswaID, HasilModeLatihan)
	if err == nil && active != nil {
		ids, perr := decodeSoalIDsJSON(active.SoalIDsJSON)
		if perr != nil {
			return nil, fmt.Errorf("soalbab latihan resume decode: %w", perr)
		}
		jawabanMap, _ := s.collectAnswered(ctx, active.ID)
		return &StartResult{
			HasilID: active.ID,
			SoalIDs: ids,
			Total:   len(ids),
			MulaiAt: active.MulaiAt,
			Resume:  true,
			Jawaban: jawabanMap,
		}, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("soalbab latihan find active: %w", err)
	}

	// New attempt path. Pull all latihan-eligible soal in this bab and
	// shuffle locally — latihan tidak perlu deterministic seed (locked
	// #79 only applies to ulangan anti-cheat).
	soals, err := s.repo.ListSoalByBab(ctx, b.ID, SoalListFilter{Mode: ModeLatihan})
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan pool: %w", err)
	}
	if len(soals) == 0 {
		return nil, ErrLatihanPoolEmpty
	}

	ids := make([]uuid.UUID, len(soals))
	for i, s := range soals {
		ids[i] = s.ID
	}
	// Fisher-Yates shuffle using injected randN seam.
	for i := len(ids) - 1; i > 0; i-- {
		j := s.randN(i + 1)
		ids[i], ids[j] = ids[j], ids[i]
	}

	encoded, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan encode: %w", err)
	}
	hasil := &HasilSoalBab{
		BabID:       b.ID,
		SiswaID:     siswaID,
		Mode:        HasilModeLatihan,
		Status:      HasilBerlangsung,
		SoalIDsJSON: datatypes.JSON(encoded),
		MulaiAt:     s.now(),
		AttemptNo:   1, // latihan attempt_no tidak dipakai (re-attempt unlimited)
	}
	if err := s.repo.CreateHasil(ctx, hasil); err != nil {
		return nil, fmt.Errorf("soalbab latihan create hasil: %w", err)
	}
	_ = s.repo.AppendEvent(ctx, &EventBab{
		HasilID: hasil.ID,
		Action:  "latihan_started",
		Meta:    marshalMeta(map[string]any{"total": len(ids), "bab_id": b.ID.String()}),
	})

	return &StartResult{
		HasilID: hasil.ID,
		SoalIDs: ids,
		Total:   len(ids),
		MulaiAt: hasil.MulaiAt,
		Resume:  false,
	}, nil
}

// AnswerInput is the resolved payload for the answer endpoint.
type AnswerInput struct {
	SoalID  uuid.UUID
	Jawaban Jawaban
}

// AnswerResult is the formative-feedback response. JawabanBenar is the
// canonical answer letter so the FE can highlight the correct option.
type AnswerResult struct {
	IsBenar       bool    `json:"is_benar"`
	JawabanBenar  Jawaban `json:"jawaban_benar"`
	PoinDapat     int16   `json:"poin_dapat"`
	JawabanTersi  Jawaban `json:"jawaban_tersimpan"`
}

// Answer upserts the jawaban for (hasil, soal) and returns immediate
// formative feedback (latihan only — locked #81). Refuses ulangan mode
// hasil to keep the surface tight.
func (s *LatihanService) Answer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) (*AnswerResult, error) {
	if !in.Jawaban.Valid() {
		return nil, fmt.Errorf("%w: jawaban must be a|b|c|d|e", ErrInvalidInput)
	}

	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan answer find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Mode != HasilModeLatihan {
		return nil, ErrHasilModeInvalid
	}
	if hasil.Status == HasilSelesai {
		return nil, ErrHasilAlreadyFinished
	}
	if hasil.Status == HasilDibatalkan {
		return nil, ErrHasilCancelled
	}

	// Anti-cheat: soal_id must be in the snapshot pool.
	pool, perr := decodeSoalIDsJSON(hasil.SoalIDsJSON)
	if perr != nil {
		return nil, fmt.Errorf("soalbab latihan answer pool decode: %w", perr)
	}
	if !containsUUID(pool, in.SoalID) {
		return nil, ErrSoalNotInPool
	}

	soal, err := s.repo.FindSoalByID(ctx, in.SoalID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Snapshot diverged from current soal table (deleted soal).
		// Surface as "soal not in pool" — siswa-friendly + reproducible.
		return nil, ErrSoalNotInPool
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan answer find soal: %w", err)
	}

	isBenar := soal.Jawaban == in.Jawaban
	var poinDapat int16
	if isBenar {
		poinDapat = soal.Poin
	}
	jawabanLower := Jawaban(strings.ToLower(string(in.Jawaban)))
	jawabanStr := string(jawabanLower)
	now := s.now()
	row := &JawabanBab{
		HasilID:    hasilID,
		SoalID:     in.SoalID,
		Jawaban:    &jawabanStr,
		IsBenar:    &isBenar,
		PoinDapat:  poinDapat,
		AnsweredAt: now,
	}
	if err := s.repo.UpsertJawaban(ctx, row); err != nil {
		return nil, fmt.Errorf("soalbab latihan answer upsert: %w", err)
	}
	_ = s.repo.AppendEvent(ctx, &EventBab{
		HasilID: hasilID,
		Action:  "answer_save",
		Meta: marshalMeta(map[string]any{
			"soal_id":  in.SoalID.String(),
			"jawaban":  jawabanStr,
			"is_benar": isBenar,
			"mode":     "latihan",
		}),
	})

	return &AnswerResult{
		IsBenar:       isBenar,
		JawabanBenar:  soal.Jawaban,
		PoinDapat:     poinDapat,
		JawabanTersi:  jawabanLower,
	}, nil
}

// FinishResult is the latihan summary — formative, no nilai persist.
type FinishResult struct {
	HasilID uuid.UUID  `json:"hasil_id"`
	Total   int        `json:"total"`
	Benar   int        `json:"benar"`
	Salah   int        `json:"salah"`
	Skip    int        `json:"skip"`
	Status  HasilStatus `json:"status"`
}

// Finish marks the latihan attempt selesai. Nilai_total stays NULL —
// latihan is formative (locked #81). Idempotent on already-finished.
func (s *LatihanService) Finish(ctx context.Context, hasilID, siswaID uuid.UUID) (*FinishResult, error) {
	hasil, err := s.repo.FindHasilByID(ctx, hasilID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan finish find: %w", err)
	}
	if hasil.SiswaID != siswaID {
		return nil, ErrHasilNotOwned
	}
	if hasil.Mode != HasilModeLatihan {
		return nil, ErrHasilModeInvalid
	}
	if hasil.Status == HasilDibatalkan {
		return nil, ErrHasilCancelled
	}

	pool, perr := decodeSoalIDsJSON(hasil.SoalIDsJSON)
	if perr != nil {
		return nil, fmt.Errorf("soalbab latihan finish pool decode: %w", perr)
	}
	jawabans, err := s.repo.ListJawabanByHasil(ctx, hasilID)
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan finish list jawaban: %w", err)
	}

	benar, salah := 0, 0
	for _, j := range jawabans {
		if j.IsBenar != nil && *j.IsBenar {
			benar++
		} else if j.Jawaban != nil && *j.Jawaban != "" {
			salah++
		}
	}
	skip := len(pool) - benar - salah
	if skip < 0 {
		skip = 0
	}

	if hasil.Status == HasilBerlangsung {
		now := s.now()
		if err := s.repo.UpdateHasilStatus(ctx, hasilID, HasilSelesai, &now, nil, nil, nil); err != nil {
			return nil, fmt.Errorf("soalbab latihan finish update: %w", err)
		}
		_ = s.repo.AppendEvent(ctx, &EventBab{
			HasilID: hasilID,
			Action:  "latihan_finished",
			Meta: marshalMeta(map[string]any{
				"total": len(pool),
				"benar": benar,
				"salah": salah,
				"skip":  skip,
			}),
		})
	}

	return &FinishResult{
		HasilID: hasilID,
		Total:   len(pool),
		Benar:   benar,
		Salah:   salah,
		Skip:    skip,
		Status:  HasilSelesai,
	}, nil
}

// requireSiswaBabAccess enforces (siswa enrolled active) ∩ (bab published).
func (s *LatihanService) requireSiswaBabAccess(ctx context.Context, babID, siswaID uuid.UUID) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab latihan bab find: %w", err)
	}
	if b.Status != bab.StatusPublished {
		// Siswa POV: hide draft/archived as not_found.
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
		return nil, fmt.Errorf("soalbab latihan enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}
	return b, nil
}

// collectAnswered returns map[soal_id] → jawaban letter for the resume
// payload. Best-effort; on error we return nil so caller can omit.
func (s *LatihanService) collectAnswered(ctx context.Context, hasilID uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := s.repo.ListJawabanByHasil(ctx, hasilID)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		if r.Jawaban != nil && *r.Jawaban != "" {
			out[r.SoalID] = *r.Jawaban
		}
	}
	return out, nil
}

func decodeSoalIDsJSON(raw datatypes.JSON) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func containsUUID(list []uuid.UUID, target uuid.UUID) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
