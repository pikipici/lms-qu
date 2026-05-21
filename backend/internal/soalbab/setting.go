// UlanganBabSetting service + handler for Task 5.C.1.
//
// Endpoints:
//
//   - GET  /api/v1/bab/:id/ulangan-setting
//     Guru/admin owner → full payload (incl. version + waktu_buka_review +
//     soal pool counters).
//     Siswa enrolled    → subset payload {durasi_menit, batas_attempt,
//     izinkan_review_setelah_submit, waktu_buka_review} for the lobby +
//     remaining attempt count. Soal counters NOT exposed to siswa.
//
//   - PUT  /api/v1/bab/:id/ulangan-setting
//     Body: {jumlah_soal, durasi_menit, batas_attempt,
//     izinkan_review_setelah_submit, waktu_buka_review?, version?}.
//     Owner-only. Validates `jumlah_soal <= count(soal mode IN
//     ('ulangan','keduanya'))`. Returns 400 `jumlah_soal_exceeds_pool`
//     when over-budget. Audit `ulangan_setting_updated`.
//
// Locked decisions referenced:
//   - #56 optimistic concurrency on update path; insert auto-creates v=1.
//   - #76 sub-fase split — lobby payload kept minimal so siswa can prep
//     mentally (durasi + attempt cap) without leaking pool size.
//   - #81 review gating uses izinkan_review_setelah_submit + waktu_buka_review.
//   - #82 coverage gate.
package soalbab

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors for setting endpoints.
var (
	// ErrSettingPoolExceeded is returned when jumlah_soal > pool size.
	ErrSettingPoolExceeded = errors.New("soalbab: jumlah_soal exceeds soal pool")
	// ErrSettingPoolEmpty is returned when caller wants to publish a
	// setting before any ulangan-eligible soal exists.
	ErrSettingPoolEmpty = errors.New("soalbab: ulangan pool empty")
)

// Setting bounds — mirrors migration CHECK constraints to give the API
// stable validation messages instead of leaking 23xxx Postgres codes.
const (
	SettingMinJumlahSoal   = 1
	SettingMaxJumlahSoal   = 200
	SettingMinDurasiMenit  = 1
	SettingMaxDurasiMenit  = 300
	SettingMinBatasAttempt = 1
	SettingMaxBatasAttempt = 10
)

// UpsertSettingInput is the resolved payload for PUT
// /api/v1/bab/:id/ulangan-setting.
type UpsertSettingInput struct {
	// ExpectedVersion is required ONLY when an existing row is being
	// updated. For first-time insert (no row yet) caller passes 0.
	ExpectedVersion            int
	JumlahSoal                 int16
	DurasiMenit                int16
	BatasAttempt               int16
	IzinkanReviewSetelahSubmit bool
	WaktuBukaReview            *time.Time
}

// SettingView is the full guru/admin payload returned by GET + PUT.
type SettingView struct {
	BabID                      uuid.UUID  `json:"bab_id"`
	JumlahSoal                 int16      `json:"jumlah_soal"`
	DurasiMenit                int16      `json:"durasi_menit"`
	BatasAttempt               int16      `json:"batas_attempt"`
	IzinkanReviewSetelahSubmit bool       `json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *time.Time `json:"waktu_buka_review,omitempty"`
	Version                    int        `json:"version"`
	CreatedAt                  *time.Time `json:"created_at,omitempty"`
	UpdatedAt                  *time.Time `json:"updated_at,omitempty"`
	// PoolSize is the count of soal eligible for ulangan (mode IN
	// ('ulangan','keduanya')). Surfaced so guru editor can warn before
	// raising jumlah_soal beyond pool.
	PoolSize int64 `json:"pool_size"`
	// Configured indicates whether a row already exists. Default-only
	// payload (no DB row) carries Configured=false so FE can show "belum
	// disetel" hint.
	Configured bool `json:"configured"`
}

// SiswaLobbyView is the trimmed payload for siswa lobby (locked #76).
type SiswaLobbyView struct {
	BabID                      uuid.UUID  `json:"bab_id"`
	DurasiMenit                int16      `json:"durasi_menit"`
	BatasAttempt               int16      `json:"batas_attempt"`
	IzinkanReviewSetelahSubmit bool       `json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *time.Time `json:"waktu_buka_review,omitempty"`
	// Configured: false → siswa lihat "guru belum mengaktifkan ulangan",
	// FE block tombol Mulai. Default values muncul tapi siswa tahu it's
	// not yet published.
	Configured bool `json:"configured"`
}

// settingRepoAPI is the subset of *Repo the setting service depends on.
// Kept private so external mocks can satisfy without pulling in CRUD ops.
type settingRepoAPI interface {
	GetSettingByBab(ctx context.Context, babID uuid.UUID) (*UlanganBabSetting, error)
	UpsertSetting(ctx context.Context, s *UlanganBabSetting, expectedVersion int) error
	CountSoalByBabMode(ctx context.Context, babID uuid.UUID, m Mode) (int64, error)
}

// enrollmentLookup verifies a siswa is enrolled in the kelas (active).
// Implemented by *kelas.Repo.FindEnrollment.
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// SettingService handles UlanganBabSetting GET/PUT.
//
// Lives separate from the soal CRUD service so the wiring stays auditable
// (different concerns: per-soal vs. per-bab config).
type SettingService struct {
	repo  settingRepoAPI
	bab   babLookup
	kelas kelasLookup
	enr   enrollmentLookup
	audit auditLogger
	now   func() time.Time
}

// NewSettingService wires the setting service. enr may be nil if the
// caller doesn't expose a siswa-facing GET (then GetForSiswa returns
// ErrForbidden for any siswa caller).
func NewSettingService(repo settingRepoAPI, b babLookup, k kelasLookup, enr enrollmentLookup, audit auditLogger) *SettingService {
	return &SettingService{repo: repo, bab: b, kelas: k, enr: enr, audit: audit, now: time.Now}
}

// GetForGuru returns the full setting view for a guru/admin owner of the
// kelas containing the bab. Returns ErrForbidden when caller can't manage.
//
// When no row exists yet, returns a default view (Configured=false) so
// the editor can render the form pre-populated with safe defaults.
func (s *SettingService) GetForGuru(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*SettingView, error) {
	b, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	pool, err := s.repo.CountSoalByBabMode(ctx, b.ID, ModeUlangan)
	if err != nil {
		return nil, fmt.Errorf("soalbab setting pool: %w", err)
	}
	row, err := s.repo.GetSettingByBab(ctx, b.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultSettingView(b.ID, pool), nil
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting get: %w", err)
	}
	return settingViewFromRow(row, pool), nil
}

// GetForSiswa returns the trimmed lobby view. Caller must be a siswa
// with active enrollment in the kelas owning the bab.
func (s *SettingService) GetForSiswa(ctx context.Context, babID, siswaID uuid.UUID) (*SiswaLobbyView, error) {
	if s.enr == nil {
		return nil, ErrForbidden
	}
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting siswa bab: %w", err)
	}
	enr, err := s.enr.FindEnrollment(ctx, b.KelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}
	row, err := s.repo.GetSettingByBab(ctx, b.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultLobbyView(b.ID), nil
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting siswa: %w", err)
	}
	return &SiswaLobbyView{
		BabID:                      b.ID,
		DurasiMenit:                row.DurasiMenit,
		BatasAttempt:               row.BatasAttempt,
		IzinkanReviewSetelahSubmit: row.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            row.WaktuBukaReview,
		Configured:                 true,
	}, nil
}

// Upsert handles PUT — owner-only. Validates input bounds, jumlah_soal vs.
// pool, and expectedVersion (when row exists). Returns the fresh full
// view post-write.
func (s *SettingService) Upsert(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in UpsertSettingInput, ip, userAgent string) (*SettingView, error) {
	b, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
	}

	if err := validateSettingBounds(in); err != nil {
		return nil, err
	}

	pool, err := s.repo.CountSoalByBabMode(ctx, b.ID, ModeUlangan)
	if err != nil {
		return nil, fmt.Errorf("soalbab setting pool: %w", err)
	}
	if pool == 0 {
		return nil, ErrSettingPoolEmpty
	}
	if int64(in.JumlahSoal) > pool {
		return nil, fmt.Errorf("%w: jumlah_soal=%d, pool=%d", ErrSettingPoolExceeded, in.JumlahSoal, pool)
	}

	// Pre-fetch existing row for audit diff + to confirm caller's
	// version actually matches what's persisted.
	existing, err := s.repo.GetSettingByBab(ctx, b.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("soalbab setting prefetch: %w", err)
	}
	isInsert := errors.Is(err, gorm.ErrRecordNotFound)

	if !isInsert {
		// Update path requires a positive version that matches.
		if in.ExpectedVersion <= 0 {
			return nil, fmt.Errorf("%w: version is required when updating an existing setting", ErrInvalidInput)
		}
		if in.ExpectedVersion != existing.Version {
			return nil, ErrVersionConflict
		}
	}

	row := &UlanganBabSetting{
		BabID:                      b.ID,
		JumlahSoal:                 in.JumlahSoal,
		DurasiMenit:                in.DurasiMenit,
		BatasAttempt:               in.BatasAttempt,
		IzinkanReviewSetelahSubmit: in.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            in.WaktuBukaReview,
	}

	expected := in.ExpectedVersion
	if isInsert {
		expected = 0
	}
	if err := s.repo.UpsertSetting(ctx, row, expected); err != nil {
		if errors.Is(err, ErrVersionConflict) {
			return nil, ErrVersionConflict
		}
		return nil, fmt.Errorf("soalbab setting upsert: %w", err)
	}

	view := settingViewFromRow(row, pool)

	meta := map[string]any{
		"bab_id":                       b.ID.String(),
		"jumlah_soal":                  row.JumlahSoal,
		"durasi_menit":                 row.DurasiMenit,
		"batas_attempt":                row.BatasAttempt,
		"izinkan_review":               row.IzinkanReviewSetelahSubmit,
		"pool_size":                    pool,
		"insert":                       isInsert,
	}
	if row.WaktuBukaReview != nil {
		meta["waktu_buka_review"] = row.WaktuBukaReview.UTC().Format(time.RFC3339)
	}
	if !isInsert && existing != nil {
		meta["old_version"] = existing.Version
		meta["new_version"] = row.Version
	} else {
		meta["new_version"] = row.Version
	}

	// Audit logging best-effort (mirror Service.logAudit pattern but
	// inlined since SettingService doesn't share helpers).
	s.logAudit(ctx, "ulangan_setting_updated", &callerID, callerRole, &b.ID, &b.KelasID, ip, userAgent, meta)

	return view, nil
}

// validateSettingBounds enforces field-level CHECK constraints with
// stable error messages. Bounds mirror migration 000010.
func validateSettingBounds(in UpsertSettingInput) error {
	if in.JumlahSoal < SettingMinJumlahSoal || in.JumlahSoal > SettingMaxJumlahSoal {
		return fmt.Errorf("%w: jumlah_soal must be between %d and %d",
			ErrInvalidInput, SettingMinJumlahSoal, SettingMaxJumlahSoal)
	}
	if in.DurasiMenit < SettingMinDurasiMenit || in.DurasiMenit > SettingMaxDurasiMenit {
		return fmt.Errorf("%w: durasi_menit must be between %d and %d",
			ErrInvalidInput, SettingMinDurasiMenit, SettingMaxDurasiMenit)
	}
	if in.BatasAttempt < SettingMinBatasAttempt || in.BatasAttempt > SettingMaxBatasAttempt {
		return fmt.Errorf("%w: batas_attempt must be between %d and %d",
			ErrInvalidInput, SettingMinBatasAttempt, SettingMaxBatasAttempt)
	}
	return nil
}

func defaultSettingView(babID uuid.UUID, pool int64) *SettingView {
	return &SettingView{
		BabID:                      babID,
		JumlahSoal:                 10,
		DurasiMenit:                30,
		BatasAttempt:               1,
		IzinkanReviewSetelahSubmit: true,
		Version:                    0,
		PoolSize:                   pool,
		Configured:                 false,
	}
}

func defaultLobbyView(babID uuid.UUID) *SiswaLobbyView {
	return &SiswaLobbyView{
		BabID:                      babID,
		DurasiMenit:                30,
		BatasAttempt:               1,
		IzinkanReviewSetelahSubmit: true,
		Configured:                 false,
	}
}

func settingViewFromRow(s *UlanganBabSetting, pool int64) *SettingView {
	created := s.CreatedAt
	updated := s.UpdatedAt
	view := &SettingView{
		BabID:                      s.BabID,
		JumlahSoal:                 s.JumlahSoal,
		DurasiMenit:                s.DurasiMenit,
		BatasAttempt:               s.BatasAttempt,
		IzinkanReviewSetelahSubmit: s.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            s.WaktuBukaReview,
		Version:                    s.Version,
		PoolSize:                   pool,
		Configured:                 true,
	}
	if !created.IsZero() {
		view.CreatedAt = &created
	}
	if !updated.IsZero() {
		view.UpdatedAt = &updated
	}
	return view
}

// findBabAndOwnership mirrors Service.findBabAndOwnership but kept local
// so SettingService doesn't depend on the larger soal CRUD Service.
func (s *SettingService) findBabAndOwnership(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting bab find: %w", err)
	}
	k, err := s.kelas.FindByID(ctx, b.KelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab setting kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return b, nil
}

// logAudit mirrors Service.logAudit but lives on SettingService.
func (s *SettingService) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "ulangan_bab_setting"
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
