// Service layer untuk kelas: input validation, ownership guard, optimistic
// concurrency, audit logging, dan side effects (regenerate kode invite saat
// duplicate). Handler stays thin and only translates HTTP <-> service.
package kelas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// Sentinel errors yang di-mapping ke HTTP status di handler.
//
// ErrVersionConflict diekspor dari repo.go dan dipakai handler langsung;
// service tidak men-deklarasi-ulang sentinel itu.
var (
	ErrInvalidInput     = errors.New("kelas: invalid input")
	ErrNotFound         = errors.New("kelas: not found")
	ErrForbidden        = errors.New("kelas: forbidden")
	ErrAlreadyArchived  = errors.New("kelas: already archived")
	ErrNotArchived      = errors.New("kelas: not archived")
	ErrBobotInvalid     = errors.New("kelas: bobot must sum to 100")
	ErrKodeInviteFailed = errors.New("kelas: failed to generate kode invite")
)

// kelasRepo subset yang dipakai service. Interface dipisah supaya gampang
// di-mock di test (project gak punya sqlite/sqlmock driver).
type kelasRepo interface {
	Create(ctx context.Context, k *Kelas) error
	FindByID(ctx context.Context, id uuid.UUID) (*Kelas, error)
	FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error)
	ListByGuru(ctx context.Context, guruID uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error)
	ListAll(ctx context.Context, includeArchived bool, limit, offset int) ([]Kelas, int64, error)
	UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, nama, deskripsi string, bobotSoalUlangan, bobotTugas int) error
	Archive(ctx context.Context, id uuid.UUID) error
	Unarchive(ctx context.Context, id uuid.UUID) error
	Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via JoinedVia) (bool, error)
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*Enrollment, error)
}

// auditLogger lets the service write audit rows without importing the full
// auth.Repo struct (avoid hard dep cycle when admin grows).
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles kelas business logic.
type Service struct {
	repo  kelasRepo
	audit auditLogger
	now   func() time.Time
}

// NewService wires the kelas Repo + audit logger.
func NewService(repo kelasRepo, audit auditLogger) *Service {
	return &Service{repo: repo, audit: audit, now: time.Now}
}

// CreateInput holds new-kelas fields supplied by the guru caller.
type CreateInput struct {
	Nama             string
	Deskripsi        string
	BobotSoalUlangan int
	BobotTugas       int
}

// Create membuat kelas baru milik guruID. Generates kode invite, validates
// bobot, persists, audits. Returns the persisted kelas (with kode invite +
// version=1).
func (s *Service) Create(ctx context.Context, guruID uuid.UUID, in CreateInput, ip, userAgent string) (*Kelas, error) {
	nama := strings.TrimSpace(in.Nama)
	if nama == "" {
		return nil, fmt.Errorf("%w: nama is required", ErrInvalidInput)
	}
	bobotSoalUlangan := in.BobotSoalUlangan
	bobotTugas := in.BobotTugas
	if bobotSoalUlangan == 0 && bobotTugas == 0 {
		// Nilai default kalau caller gak set.
		bobotSoalUlangan = 50
		bobotTugas = 50
	}
	if err := validateBobot(bobotSoalUlangan, bobotTugas); err != nil {
		return nil, err
	}

	kode, err := GenerateKodeInvite(ctx, s.repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKodeInviteFailed, err)
	}

	k := &Kelas{
		Nama:             nama,
		Deskripsi:        strings.TrimSpace(in.Deskripsi),
		KodeInvite:       kode,
		GuruID:           guruID,
		BobotSoalUlangan: bobotSoalUlangan,
		BobotTugas:       bobotTugas,
		Version:          1,
	}
	if err := s.repo.Create(ctx, k); err != nil {
		return nil, fmt.Errorf("kelas create: %w", err)
	}

	s.logAudit(ctx, "kelas_created", &guruID, &k.ID, &k.ID, ip, userAgent, map[string]any{
		"nama":               k.Nama,
		"kode_invite":        k.KodeInvite,
		"bobot_soal_ulangan": k.BobotSoalUlangan,
		"bobot_tugas":        k.BobotTugas,
	})

	return k, nil
}

// ListInput holds list filters/pagination.
type ListInput struct {
	IncludeArchived bool
	Limit           int
	Offset          int
}

// ListResult is the page returned by List endpoints.
type ListResult struct {
	Items []Kelas
	Total int64
}

// ListForGuru returns the kelas owned by guruID.
func (s *Service) ListForGuru(ctx context.Context, guruID uuid.UUID, in ListInput) (*ListResult, error) {
	limit, offset := normalizePagination(in.Limit, in.Offset)
	rows, total, err := s.repo.ListByGuru(ctx, guruID, in.IncludeArchived, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("kelas list: %w", err)
	}
	return &ListResult{Items: rows, Total: total}, nil
}

// ListAllAdmin returns every kelas across the system. Caller must be admin.
func (s *Service) ListAllAdmin(ctx context.Context, in ListInput) (*ListResult, error) {
	limit, offset := normalizePagination(in.Limit, in.Offset)
	rows, total, err := s.repo.ListAll(ctx, in.IncludeArchived, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("kelas list all: %w", err)
	}
	return &ListResult{Items: rows, Total: total}, nil
}

// Get returns a single kelas by id with ownership check (only guru owner OR
// admin role can read). Pass nil viewerID + admin role to bypass owner gate.
func (s *Service) Get(ctx context.Context, id, viewerID uuid.UUID, viewerRole string) (*Kelas, error) {
	k, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kelas get: %w", err)
	}
	if !canManage(k, viewerID, viewerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

// UpdateInput represents the patch payload (PATCH /kelas/:id). Pointer fields
// are optional: nil means "leave unchanged"; non-nil means "set to this value".
// ExpectedVersion + Nama tetap wajib (Nama selalu re-affirmed lewat PATCH body
// supaya audit log nge-capture nilai final yang konsisten).
type UpdateInput struct {
	ExpectedVersion  int
	Nama             string
	Deskripsi        *string
	BobotSoalUlangan *int
	BobotTugas       *int
}

// Update applies an optimistic-concurrency update. ExpectedVersion mismatch
// → ErrVersionConflict. Forbidden if caller isn't the kelas owner.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Kelas, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}
	nama := strings.TrimSpace(in.Nama)
	if nama == "" {
		return nil, fmt.Errorf("%w: nama is required", ErrInvalidInput)
	}

	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kelas update find: %w", err)
	}
	if !canManage(existing, callerID, callerRole) {
		return nil, ErrForbidden
	}

	// Resolve final values: caller-supplied wins, otherwise carry existing.
	deskripsi := existing.Deskripsi
	if in.Deskripsi != nil {
		deskripsi = strings.TrimSpace(*in.Deskripsi)
	}
	bobotSoalUlangan := existing.BobotSoalUlangan
	if in.BobotSoalUlangan != nil {
		bobotSoalUlangan = *in.BobotSoalUlangan
	}
	bobotTugas := existing.BobotTugas
	if in.BobotTugas != nil {
		bobotTugas = *in.BobotTugas
	}
	if err := validateBobot(bobotSoalUlangan, bobotTugas); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateBasic(ctx, id, in.ExpectedVersion, nama, deskripsi, bobotSoalUlangan, bobotTugas); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return nil, ErrNotFound
		case errors.Is(err, ErrVersionConflict):
			return nil, ErrVersionConflict
		default:
			return nil, fmt.Errorf("kelas update: %w", err)
		}
	}

	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("kelas update refetch: %w", err)
	}

	s.logAudit(ctx, "kelas_updated", &callerID, &id, &id, ip, userAgent, map[string]any{
		"old_nama":               existing.Nama,
		"new_nama":               fresh.Nama,
		"old_bobot_soal_ulangan": existing.BobotSoalUlangan,
		"new_bobot_soal_ulangan": fresh.BobotSoalUlangan,
		"old_bobot_tugas":        existing.BobotTugas,
		"new_bobot_tugas":        fresh.BobotTugas,
		"new_version":            fresh.Version,
	})

	return fresh, nil
}

// Archive soft-archives the kelas. Idempotent: ErrAlreadyArchived if archived
// before. Forbidden when caller isn't owner.
func (s *Service) Archive(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Kelas, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kelas archive find: %w", err)
	}
	if !canManage(existing, callerID, callerRole) {
		return nil, ErrForbidden
	}
	if existing.ArchivedAt != nil {
		return nil, ErrAlreadyArchived
	}

	if err := s.repo.Archive(ctx, id); err != nil {
		return nil, fmt.Errorf("kelas archive: %w", err)
	}

	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("kelas archive refetch: %w", err)
	}

	s.logAudit(ctx, "kelas_archived", &callerID, &id, &id, ip, userAgent, map[string]any{
		"nama": fresh.Nama,
	})

	return fresh, nil
}

// DuplicateInput allows the caller to override the new kelas name. Default:
// "<original> (Salinan)".
type DuplicateInput struct {
	NewNama string
}

// Duplicate copies a kelas row into a new one with regenerated kode invite,
// version=1, archived_at=NULL, and (Fase 2 reduced scope) NO Bab/Materi/Soal/
// Tugas/Ulangan carry-over. Those land in Fase 3 once those tables exist.
//
// Caller must be the kelas owner OR admin.
func (s *Service) Duplicate(ctx context.Context, id, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Kelas, error) {
	source, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kelas duplicate find: %w", err)
	}
	if !canManage(source, callerID, callerRole) {
		return nil, ErrForbidden
	}

	newNama := strings.TrimSpace(in.NewNama)
	if newNama == "" {
		newNama = source.Nama + " (Salinan)"
	}

	kode, err := GenerateKodeInvite(ctx, s.repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrKodeInviteFailed, err)
	}

	dup := &Kelas{
		Nama:             newNama,
		Deskripsi:        source.Deskripsi,
		KodeInvite:       kode,
		GuruID:           source.GuruID, // tetap dimiliki guru asal; admin-driven duplicate juga inherit
		BobotSoalUlangan: source.BobotSoalUlangan,
		BobotTugas:       source.BobotTugas,
		Version:          1,
	}
	if err := s.repo.Create(ctx, dup); err != nil {
		return nil, fmt.Errorf("kelas duplicate create: %w", err)
	}

	s.logAudit(ctx, "kelas_duplicated", &callerID, &dup.ID, &dup.ID, ip, userAgent, map[string]any{
		"source_id":   source.ID.String(),
		"source_nama": source.Nama,
		"new_nama":    dup.Nama,
		"kode_invite": dup.KodeInvite,
	})

	return dup, nil
}

// canManage decides whether viewerID/role may read/write the given kelas.
// Admin role can manage all kelas; guru role can manage only kelas where
// GuruID matches. Other roles get false.
func canManage(k *Kelas, viewerID uuid.UUID, viewerRole string) bool {
	if k == nil {
		return false
	}
	if viewerRole == string(auth.Admin) {
		return true
	}
	if viewerRole == string(auth.Guru) && k.GuruID == viewerID {
		return true
	}
	return false
}

func validateBobot(bobotSoalUlangan, bobotTugas int) error {
	if bobotSoalUlangan < 0 || bobotTugas < 0 {
		return fmt.Errorf("%w: bobot must be non-negative", ErrInvalidInput)
	}
	if bobotSoalUlangan+bobotTugas != 100 {
		return ErrBobotInvalid
	}
	return nil
}

func normalizePagination(limit, offset int) (int, int) {
	const (
		defaultLimit = 20
		maxLimit     = 100
	)
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// logAudit is best-effort: a logging failure must not poison the user-facing
// success response, so we swallow the error here (admin handler does the same).
func (s *Service) logAudit(ctx context.Context, action string, actorID, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	role := string(auth.Guru)
	if actorID == nil {
		role = ""
	}
	s.logAuditWithRole(ctx, action, role, actorID, targetID, targetKelasID, ip, userAgent, meta)
}

// logAuditWithRole writes an audit row with an explicit actor role. Use this
// when the actor is not a guru (e.g. siswa joining a kelas, admin assigning).
func (s *Service) logAuditWithRole(ctx context.Context, action, role string, actorID, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "kelas"
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
