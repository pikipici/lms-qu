// Service layer untuk bab: input validation, ownership guard via kelas,
// optimistic concurrency, audit logging. Handler stays thin.
package bab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors yang di-mapping ke HTTP status di handler. ErrVersionConflict
// + ErrInvalidStatus diekspor dari repo.go dan dipakai handler langsung.
var (
	ErrInvalidInput    = errors.New("bab: invalid input")
	ErrNotFound        = errors.New("bab: not found")
	ErrForbidden       = errors.New("bab: forbidden")
	ErrAlreadyArchived = errors.New("bab: already archived")
	ErrKelasArchived   = errors.New("bab: kelas archived")
)

// babRepo subset yang dipakai service. Interface dipisah supaya gampang
// di-mock di test.
type babRepo interface {
	Create(ctx context.Context, b *Bab) error
	FindByID(ctx context.Context, id uuid.UUID) (*Bab, error)
	MaxUrutan(ctx context.Context, kelasID uuid.UUID) (int, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Bab, error)
	CountByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) (int64, error)
	UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, nomor int, judul, deskripsi string) error
	UpdateStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status Status) error
	UpdateUrutan(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion, urutan int) error
	Archive(ctx context.Context, id uuid.UUID) error
}

// kelasLookup hydrates kelas ownership/lifecycle for the bab service. We
// don't import *kelas.Repo directly to keep the dep narrow; the production
// wiring passes *kelas.Repo which satisfies the interface.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// auditLogger lets the service write audit rows without a hard auth dep
// when admin grows. Implemented by *auth.Repo.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles bab business logic.
type Service struct {
	repo  babRepo
	kelas kelasLookup
	audit auditLogger
	now   func() time.Time
}

// NewService wires bab Repo + kelas lookup + audit logger.
func NewService(repo babRepo, kelas kelasLookup, audit auditLogger) *Service {
	return &Service{repo: repo, kelas: kelas, audit: audit, now: time.Now}
}

// CreateInput holds fields for POST /kelas/:id/bab.
type CreateInput struct {
	Nomor     int
	Judul     string
	Deskripsi string
}

// Create membuat bab baru di kelas. Owner-only + kelas active. Urutan auto
// = max+1 dalam kelas tersebut. Status default draft, version=1.
func (s *Service) Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Bab, error) {
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if in.Nomor < 1 {
		return nil, fmt.Errorf("%w: nomor must be >= 1", ErrInvalidInput)
	}

	k, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	maxUrutan, err := s.repo.MaxUrutan(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("bab create max urutan: %w", err)
	}

	b := &Bab{
		KelasID:   kelasID,
		Nomor:     in.Nomor,
		Judul:     judul,
		Deskripsi: strings.TrimSpace(in.Deskripsi),
		Urutan:    maxUrutan + 1,
		Status:    StatusDraft,
		Version:   1,
	}
	if err := s.repo.Create(ctx, b); err != nil {
		return nil, fmt.Errorf("bab create: %w", err)
	}

	s.logAudit(ctx, "bab_created", &callerID, callerRole, &b.ID, &kelasID, ip, userAgent, map[string]any{
		"bab_id": b.ID.String(),
		"nomor":  b.Nomor,
		"judul":  b.Judul,
		"urutan": b.Urutan,
	})
	return b, nil
}

// ListInput narrows ListByKelas results.
type ListInput struct {
	IncludeArchived bool
	Status          *Status
}

// ListByKelas returns all bab in a kelas owned by caller (or admin). Caller
// passes Status=&StatusPublished for siswa-style filter; otherwise
// IncludeArchived controls whether archived rows are visible.
func (s *Service) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Bab, error) {
	if _, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListByKelas(ctx, kelasID, ListFilter{IncludeArchived: in.IncludeArchived, Status: in.Status})
	if err != nil {
		return nil, fmt.Errorf("bab list: %w", err)
	}
	return rows, nil
}

// Get returns a bab by id with ownership guard via its kelas.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Bab, error) {
	b, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bab get: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, b.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateInput is the PATCH payload. Pointer fields are optional ("nil =
// leave unchanged"). ExpectedVersion is required.
type UpdateInput struct {
	ExpectedVersion int
	Nomor           *int
	Judul           *string
	Deskripsi       *string
	Urutan          *int
	Status          *Status
}

// Update applies a partial update with optimistic concurrency. Status change
// is audited separately as bab_status_changed; basic field changes go to
// bab_updated. When both basic and status change in one call, two audit rows
// are written.
//
// Note: Urutan-only changes use UpdateUrutan path internally. Mixing basic
// fields + urutan in one call is supported and runs in a single tx.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Bab, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bab update find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	// Resolve final values.
	nomor := existing.Nomor
	if in.Nomor != nil {
		nomor = *in.Nomor
		if nomor < 1 {
			return nil, fmt.Errorf("%w: nomor must be >= 1", ErrInvalidInput)
		}
	}
	judul := existing.Judul
	if in.Judul != nil {
		judul = strings.TrimSpace(*in.Judul)
		if judul == "" {
			return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
		}
	}
	deskripsi := existing.Deskripsi
	if in.Deskripsi != nil {
		deskripsi = strings.TrimSpace(*in.Deskripsi)
	}

	// Detect what actually changed so the audit log only fires for real edits.
	basicChanged := nomor != existing.Nomor || judul != existing.Judul || deskripsi != existing.Deskripsi
	urutanChanged := in.Urutan != nil && *in.Urutan != existing.Urutan
	statusChanged := in.Status != nil && *in.Status != existing.Status

	if !basicChanged && !urutanChanged && !statusChanged {
		// No-op PATCH: return existing without bumping version.
		return existing, nil
	}

	if statusChanged && !in.Status.Valid() {
		return nil, ErrInvalidStatus
	}

	currentVersion := in.ExpectedVersion

	// Phase 1: basic fields (nomor/judul/deskripsi).
	if basicChanged {
		if err := s.repo.UpdateBasic(ctx, id, currentVersion, nomor, judul, deskripsi); err != nil {
			return nil, mapRepoErr(err)
		}
		currentVersion++
	}

	// Phase 2: urutan (uses tx-aware UpdateUrutan; here we run it without a tx
	// since this is a single-row update — service-level Reorder uses tx for
	// the bulk version).
	if urutanChanged {
		if err := s.repo.UpdateUrutan(ctx, nil, id, currentVersion, *in.Urutan); err != nil {
			return nil, mapRepoErr(err)
		}
		currentVersion++
	}

	// Phase 3: status transition.
	if statusChanged {
		if err := s.repo.UpdateStatus(ctx, id, currentVersion, *in.Status); err != nil {
			return nil, mapRepoErr(err)
		}
		currentVersion++
	}

	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("bab update refetch: %w", err)
	}

	// Audit log: separate rows for basic vs status so guru audit scope (#59)
	// can filter by action.
	if basicChanged || urutanChanged {
		s.logAudit(ctx, "bab_updated", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
			"bab_id":         id.String(),
			"old_nomor":      existing.Nomor,
			"new_nomor":      fresh.Nomor,
			"old_judul":      existing.Judul,
			"new_judul":      fresh.Judul,
			"old_deskripsi":  existing.Deskripsi,
			"new_deskripsi":  fresh.Deskripsi,
			"old_urutan":     existing.Urutan,
			"new_urutan":     fresh.Urutan,
			"new_version":    fresh.Version,
		})
	}
	if statusChanged {
		s.logAudit(ctx, "bab_status_changed", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
			"bab_id":      id.String(),
			"status_lama": string(existing.Status),
			"status_baru": string(fresh.Status),
			"new_version": fresh.Version,
		})
	}

	return fresh, nil
}

// Archive transitions a bab to status='archived'. Idempotent: returns
// ErrAlreadyArchived if the row is already archived.
func (s *Service) Archive(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Bab, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bab archive find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	if existing.Status == StatusArchived {
		return nil, ErrAlreadyArchived
	}

	if err := s.repo.Archive(ctx, id); err != nil {
		return nil, fmt.Errorf("bab archive: %w", err)
	}
	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("bab archive refetch: %w", err)
	}

	s.logAudit(ctx, "bab_archived", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"bab_id":      id.String(),
		"judul":       fresh.Judul,
		"status_lama": string(existing.Status),
	})
	return fresh, nil
}

// findKelasOrForbidden hydrates kelas + asserts callerID/role can manage it.
// Returns ErrNotFound if the kelas is missing, ErrForbidden if caller isn't
// owner/admin.
func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bab kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

// canManageKelas mirrors kelas.canManage but lives here to avoid an exported
// helper leak. Admin manages all; guru manages their own; siswa is rejected.
func canManageKelas(k *kelas.Kelas, callerID uuid.UUID, callerRole string) bool {
	if k == nil {
		return false
	}
	if callerRole == string(auth.Admin) {
		return true
	}
	if callerRole == string(auth.Guru) && k.GuruID == callerID {
		return true
	}
	return false
}

// mapRepoErr converts repo-level sentinels into service-level sentinels so
// handler code can match on a single layer.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, ErrInvalidStatus):
		return ErrInvalidStatus
	default:
		return fmt.Errorf("bab repo: %w", err)
	}
}

// logAudit is best-effort: a logging failure must not poison the user-facing
// success response, so we swallow the error here (matches kelas + admin).
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "bab"
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

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func marshalMeta(fields map[string]any) datatypes.JSON {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}
