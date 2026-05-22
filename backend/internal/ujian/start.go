// Ujian flow service — Start endpoint (Task 6.D.1).
//
// Mirror SoalBab UlanganService.Start (locked #79) with these deltas:
//   - Pool source = Ujian.SourceConfigJSON discriminated (locked #85):
//       manual: hydrate dari ujian_soal junction (cached at 6.C.2 PATCH).
//       random: ListIDsByOwnerFilter di BankSoal owner=Ujian.GuruID,
//               then deterministic shuffle + take JumlahSoal.
//   - Seed (locked #86): sha256(mulai_at_unix_micro || siswa_id ||
//                                ujian_id)[:8] LE → int64.
//   - Single-flight: pg_advisory_xact_lock(sha256(ujian||siswa)[:8]).
//   - Active attempt vs cancelled: deleted_at IS NULL guard.
//   - Single-attempt-per (Ujian, Siswa) — locked decision:
//       Ujian default tidak punya remedial (#84/#85 — guru bikin Ujian
//       baru kalau mau remedial). HasilUjian partial-unique (ujian,
//       siswa) WHERE deleted_at IS NULL menjamin satu attempt valid;
//       guru cancel via 6.E.1 → soft-delete supaya siswa start lagi.
//
// Endpoint Task 6.D.1:
//   - POST /api/v1/siswa/ujian/:id/start  (sub-fase under siswaGroup)
//
// Endpoint answer/submit/cron deferred ke 6.D.2-6.D.4.
package ujian

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
	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors specific to Ujian flow (start/answer/submit).
var (
	// ErrUjianNotPublished — siswa start ujian yang belum published
	// (status=draft). FE block tombol Mulai sampai guru publish.
	ErrUjianNotPublished = errors.New("ujian: not published")
	// ErrUjianSourceMissing — Ujian.SourceConfigJSON kosong/`{}`. Guru
	// belum pilih source mode di 6.C.2.
	ErrUjianSourceMissing = errors.New("ujian: source not configured")
	// ErrUjianSourcePoolEmpty — random-mode pool kosong setelah filter
	// (guru hapus bank soal post-setup) atau manual junction kosong.
	ErrUjianSourcePoolEmpty = errors.New("ujian: source pool empty at start")
	// ErrUjianAlreadyAttempted — siswa sudah punya HasilUjian aktif/
	// selesai (deleted_at IS NULL). Single-attempt by default.
	ErrUjianAlreadyAttempted = errors.New("ujian: already attempted")
	// ErrUjianTimerExpired — siswa POST answer setelah deadline_at.
	// HTTP 410 Gone (locked #87 timer expire).
	ErrUjianTimerExpired = errors.New("ujian: timer expired")
	// ErrUjianWindowClosed — siswa start setelah Ujian.WaktuSelesai
	// (kalau diset). 410 Gone.
	ErrUjianWindowClosed = errors.New("ujian: schedule window closed")
	// ErrUjianWindowNotOpen — siswa start sebelum Ujian.WaktuMulai
	// (kalau diset). 425 Too Early kalau spec, kita pakai 409.
	ErrUjianWindowNotOpen = errors.New("ujian: schedule window not open")
)

// flowRepoAPI is the subset of *Repo Start service depends on. Extends
// service.go's repoAPI dengan tx + jawaban + event helpers.
type flowRepoAPI interface {
	repoAPI
	DB() *gorm.DB
	CreateHasil(ctx context.Context, h *HasilUjian) error
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilUjian, error)
	FindActiveHasil(ctx context.Context, ujianID, siswaID uuid.UUID) (*HasilUjian, error)
	CountAttempts(ctx context.Context, ujianID, siswaID uuid.UUID) (int64, error)
	ListHasilBySiswa(ctx context.Context, ujianID, siswaID uuid.UUID) ([]HasilUjian, error)
	ScanExpiredHasilIDs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
	UpsertJawaban(ctx context.Context, j *JawabanUjian) error
	ListJawabanByHasil(ctx context.Context, hasilID uuid.UUID) ([]JawabanUjian, error)
	AppendEvent(ctx context.Context, e *EventUjian) error
}

// enrollmentLookup verifies siswa enrolment di kelas ujian.
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// bankSoalListLookup expands the read-side dependency for Start (random
// pool hydration) + Items (full soal payload). banksoal.Repo already
// satisfies this.
type bankSoalListLookup interface {
	bankSoalLookup
	FindSoalsByIDs(ctx context.Context, ids []uuid.UUID) ([]banksoal.BankSoal, error)
}

// FlowService implements Ujian start/answer/submit.
type FlowService struct {
	repo  flowRepoAPI
	ujian repoAPI // re-use service-level repo for FindUjianByID + filter
	bank  bankSoalListLookup
	enr   enrollmentLookup
	audit auditLogger
	now   func() time.Time
}

// NewFlowService wires Ujian flow service. enr verifies siswa enrolment.
func NewFlowService(repo flowRepoAPI, bank bankSoalListLookup, enr enrollmentLookup, audit auditLogger) *FlowService {
	return &FlowService{repo: repo, ujian: repo, bank: bank, enr: enr, audit: audit, now: time.Now}
}

// StartResult is the response payload for POST start.
type StartResult struct {
	HasilID     uuid.UUID   `json:"hasil_id"`
	UjianID     uuid.UUID   `json:"ujian_id"`
	SoalIDs     []uuid.UUID `json:"soal_ids"`
	Total       int         `json:"total"`
	MulaiAt     time.Time   `json:"mulai_at"`
	DeadlineAt  time.Time   `json:"deadline_at"`
	DurasiDetik int         `json:"durasi_detik"`
	AttemptNo   int16       `json:"attempt_no"`
	Resume      bool        `json:"resume"`
}

// Start either resumes an in-progress ujian attempt for (siswa, ujian)
// or creates a new one with a deterministically-seeded snapshot per
// SourceConfigJSON. Race-safe via pg advisory transaction lock keyed
// on (ujian_id, siswa_id).
func (s *FlowService) Start(ctx context.Context, ujianID, siswaID uuid.UUID, ip, userAgent string) (*StartResult, error) {
	u, err := s.ujian.FindUjianByID(ctx, ujianID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ujian start find: %w", err)
	}

	// Enrolment + status published guard.
	if err := s.assertSiswaCanStart(ctx, u, siswaID); err != nil {
		return nil, err
	}

	// Schedule window guard (optional fields).
	now := s.now()
	if u.WaktuMulai != nil && now.Before(*u.WaktuMulai) {
		return nil, ErrUjianWindowNotOpen
	}
	if u.WaktuSelesai != nil && now.After(*u.WaktuSelesai) {
		return nil, ErrUjianWindowClosed
	}

	// Source must be configured before siswa can start.
	srcMode := PeekSourceMode(u.SourceConfigJSON)
	if srcMode == "" {
		return nil, ErrUjianSourceMissing
	}

	// Single-flight via pg advisory_xact_lock(bigint) on (ujian, siswa).
	lockKey := startLockKey(ujianID, siswaID)

	tx := s.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("ujian start tx begin: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", lockKey).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start advisory lock: %w", err)
	}

	// Re-check under lock: active berlangsung attempt? Resume.
	var active HasilUjian
	err = tx.Where("ujian_id = ? AND siswa_id = ? AND status = ? AND deleted_at IS NULL",
		ujianID, siswaID, HasilBerlangsung).
		Order("mulai_at DESC").
		First(&active).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start active find: %w", err)
	}
	if err == nil {
		ids, perr := decodeSoalIDsJSONUjian(active.SoalIDsJSON)
		if perr != nil {
			tx.Rollback()
			return nil, fmt.Errorf("ujian start resume decode: %w", perr)
		}
		if active.DeadlineAt == nil {
			tx.Rollback()
			return nil, errors.New("ujian start: berlangsung row missing deadline_at")
		}
		if err := tx.Commit().Error; err != nil {
			return nil, fmt.Errorf("ujian start commit resume: %w", err)
		}
		return &StartResult{
			HasilID:     active.ID,
			UjianID:     ujianID,
			SoalIDs:     ids,
			Total:       len(ids),
			MulaiAt:     active.MulaiAt,
			DeadlineAt:  *active.DeadlineAt,
			DurasiDetik: int(active.DeadlineAt.Sub(active.MulaiAt).Seconds()),
			AttemptNo:   active.AttemptNo,
			Resume:      true,
		}, nil
	}

	// New attempt path under lock — single-attempt enforcement.
	// HasilUjian partial-unique (ujian_id, siswa_id) WHERE deleted_at
	// IS NULL menjamin: kalau ada row selesai/dibatalkan, deleted_at
	// must be NOT NULL (cancel path) atau status=selesai (sudah
	// selesai). Kita reject siswa retry kecuali guru cancel attempt.
	var n int64
	if err := tx.Model(&HasilUjian{}).
		Where("ujian_id = ? AND siswa_id = ? AND deleted_at IS NULL", ujianID, siswaID).
		Count(&n).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start count: %w", err)
	}
	if n > 0 {
		tx.Rollback()
		return nil, ErrUjianAlreadyAttempted
	}

	// Build pool dari source config.
	pool, err := s.buildPoolUnderTx(ctx, tx, u)
	if err != nil {
		tx.Rollback()
		return nil, err
	}
	if len(pool) == 0 {
		tx.Rollback()
		return nil, ErrUjianSourcePoolEmpty
	}

	// Deterministic shuffle (locked #86): seed = sha256(mulai_unix_micro
	// || siswa_id || ujian_id)[:8] LE → int64.
	mulaiAt := now
	seed := deriveSeedUjian(mulaiAt, siswaID, u.ID)
	rng := rand.New(rand.NewSource(seed))

	// Random mode: shuffle pool then take jumlah_soal.
	// Manual mode: pool is the full junction; we still shuffle order
	// supaya siswa gak dapet urutan identik dengan FE editor — anti-
	// cheat ringan.
	pickedIDs := make([]uuid.UUID, len(pool))
	copy(pickedIDs, pool)
	for i := len(pickedIDs) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		pickedIDs[i], pickedIDs[j] = pickedIDs[j], pickedIDs[i]
	}
	if srcMode == SourceRandom {
		var rcfg RandomSourceConfig
		if err := json.Unmarshal(u.SourceConfigJSON, &rcfg); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("ujian start random unmarshal: %w", err)
		}
		if rcfg.JumlahSoal > 0 && rcfg.JumlahSoal < len(pickedIDs) {
			pickedIDs = pickedIDs[:rcfg.JumlahSoal]
		}
	}

	encoded, err := json.Marshal(pickedIDs)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start encode: %w", err)
	}
	deadline := mulaiAt.Add(time.Duration(u.DurasiMenit) * time.Minute)
	hasil := &HasilUjian{
		UjianID:     u.ID,
		SiswaID:     siswaID,
		Status:      HasilBerlangsung,
		SoalIDsJSON: datatypes.JSON(encoded),
		MulaiAt:     mulaiAt,
		DeadlineAt:  &deadline,
		AttemptNo:   1, // single-attempt MVP; remedial = guru bikin ujian baru
	}
	if err := tx.Create(hasil).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start create hasil: %w", err)
	}
	if err := tx.Create(&EventUjian{
		HasilID: hasil.ID,
		Action:  "ujian_started",
		Meta: marshalMeta(map[string]any{
			"ujian_id":     u.ID.String(),
			"source_mode":  string(srcMode),
			"jumlah_soal":  len(pickedIDs),
			"durasi_menit": u.DurasiMenit,
		}),
	}).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("ujian start event: %w", err)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("ujian start commit: %w", err)
	}

	// Audit ledger (best-effort, post-commit).
	s.logAudit(ctx, "ujian_started", &siswaID, string(auth.Siswa), &hasil.ID, &u.KelasID, ip, userAgent, map[string]any{
		"hasil_id":     hasil.ID.String(),
		"ujian_id":     u.ID.String(),
		"source_mode":  string(srcMode),
		"jumlah_soal":  len(pickedIDs),
		"durasi_menit": u.DurasiMenit,
		"deadline_at":  deadline.UTC().Format(time.RFC3339Nano),
	})

	return &StartResult{
		HasilID:     hasil.ID,
		UjianID:     u.ID,
		SoalIDs:     pickedIDs,
		Total:       len(pickedIDs),
		MulaiAt:     mulaiAt,
		DeadlineAt:  deadline,
		DurasiDetik: int(u.DurasiMenit) * 60,
		AttemptNo:   1,
		Resume:      false,
	}, nil
}

// buildPoolUnderTx returns the soal-id pool for a given ujian inside an
// active tx. Manual: read from ujian_soal junction. Random: query
// BankSoal filter (bypass tx — read-only across tables, OK).
func (s *FlowService) buildPoolUnderTx(ctx context.Context, tx *gorm.DB, u *Ujian) ([]uuid.UUID, error) {
	mode := PeekSourceMode(u.SourceConfigJSON)
	switch mode {
	case SourceManual:
		var rows []UjianSoal
		if err := tx.Where("ujian_id = ?", u.ID).
			Order("urutan ASC, soal_id ASC").
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("ujian start manual junction: %w", err)
		}
		out := make([]uuid.UUID, 0, len(rows))
		for _, r := range rows {
			out = append(out, r.SoalID)
		}
		return out, nil
	case SourceRandom:
		var cfg RandomSourceConfig
		if err := json.Unmarshal(u.SourceConfigJSON, &cfg); err != nil {
			return nil, fmt.Errorf("ujian start random unmarshal: %w", err)
		}
		ids, err := s.bank.ListIDsByOwnerFilter(ctx, u.GuruID, banksoal.ListFilter{
			Mapel:   cfg.Filter.Mapel,
			Tingkat: cfg.Filter.Tingkat,
			Topik:   cfg.Filter.Topik,
		})
		if err != nil {
			return nil, fmt.Errorf("ujian start random pool: %w", err)
		}
		return ids, nil
	default:
		return nil, ErrUjianSourceMissing
	}
}

// assertSiswaCanStart enforces (siswa enrolled active) ∩ (ujian published).
func (s *FlowService) assertSiswaCanStart(ctx context.Context, u *Ujian, siswaID uuid.UUID) error {
	if u.Status != StatusPublished {
		return ErrUjianNotPublished
	}
	if s.enr == nil {
		return ErrForbidden
	}
	enr, err := s.enr.FindEnrollment(ctx, u.KelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("ujian start enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return ErrForbidden
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// startLockKey derives an int64 advisory-lock key for (ujian, siswa).
// Distinct domain prefix supaya tidak collide dengan submitLockKey.
func startLockKey(ujianID, siswaID uuid.UUID) int64 {
	h := sha256.New()
	h.Write([]byte("ujian-start:"))
	ub, _ := ujianID.MarshalBinary()
	sb, _ := siswaID.MarshalBinary()
	h.Write(ub)
	h.Write(sb)
	d := h.Sum(nil)
	return int64(binary.LittleEndian.Uint64(d[:8])) //nolint:gosec
}

// deriveSeedUjian implements locked #86: sha256(mulai_at_unix_micro_bytes
// || siswa_id_bytes || ujian_id_bytes)[:8] LE → int64. Deterministic per
// (mulai_at, siswa, ujian) → resume bawa pool yang sama.
func deriveSeedUjian(mulaiAt time.Time, siswaID, ujianID uuid.UUID) int64 {
	h := sha256.New()
	var mb [8]byte
	binary.LittleEndian.PutUint64(mb[:], uint64(mulaiAt.UnixMicro()))
	h.Write(mb[:])
	sb, _ := siswaID.MarshalBinary()
	ub, _ := ujianID.MarshalBinary()
	h.Write(sb)
	h.Write(ub)
	digest := h.Sum(nil)
	u := binary.LittleEndian.Uint64(digest[:8])
	return int64(u) //nolint:gosec
}

// decodeSoalIDsJSONUjian — decode datatypes.JSON to []uuid.UUID.
func decodeSoalIDsJSONUjian(raw datatypes.JSON) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var ids []uuid.UUID
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// containsUUIDUjian — slice membership check.
func containsUUIDUjian(list []uuid.UUID, target uuid.UUID) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// logAudit on FlowService — same pattern as Service.logAudit.
func (s *FlowService) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
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
