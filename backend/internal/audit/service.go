// Package audit serves the guru-side audit log endpoint (Task 7.E,
// locked #59).
//
// Scope: GET /api/v1/guru/kelas/:id/audit?action=<filter>&limit=&offset=
//   - Auth: guru/admin (RoleGuard); guru hanya kelas yang dia owner.
//   - Filter action: optional, harus salah satu dari ALLOWED_ACTIONS
//     (kalau invalid → 400). Kosong = semua action allowlisted.
//   - Hard scope: WHERE target_kelas_id = :id. Entry dengan
//     target_kelas_id NULL TIDAK muncul di endpoint ini (out-of-scope).
//   - Limit: clamp 1..100, default 50.
//   - Actor name: enrich dari users.name (bulk lookup) supaya FE bisa
//     render tanpa second round-trip.
//
// Berbeda dari admin /admin/audit-log: scope hard-bound ke target_kelas_id
// (tidak bisa lihat entry yang bukan kelas dia, walau actor_id sama).
package audit

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// AllowedActions adalah whitelist action filter yang diizinkan untuk guru
// (locked #59 + roadmap Task 7.E.1). Includes Fase 5/6 reset events +
// kelas lifecycle + grading actions yang relevant ke transparansi guru.
//
// Semua action yang ada di-emit oleh package lain dgn TargetKelasID set
// — tidak ada action di sini yang TargetKelasID NULL.
var AllowedActions = []string{
	"hasil_reset",            // soalbab/hasil — guru/admin reset ulangan attempt
	"ulangan_bab_cancelled",  // soalbab/hasil — alias hasil_reset
	"ujian_attempt_reset",    // ujian/hasil — guru/admin reset ujian attempt
	"bab_archived",           // bab/service — guru archive bab
	"bab_published",          // bab/service — guru publish bab (kalau emit)
	"siswa_kicked",           // kelas/admin — admin/guru kick siswa
	"tugas_deleted",          // tugas/service — guru delete tugas
	"submission_graded",      // submission/service — guru grade submission
	"ujian_auto_graded",      // ujian/timer_cron — auto-grade
	"ulangan_bab_auto_graded",// soalbab/timer_cron — auto-grade
	"ulangan_bab_submitted",  // soalbab/ulangan — siswa submit ulangan
	"ujian_started",          // ujian/start — siswa mulai ujian
	"ulangan_bab_started",    // soalbab/ulangan — siswa mulai ulangan
}

// IsAllowedAction reports whether action is in the allowlist.
func IsAllowedAction(action string) bool {
	for _, a := range AllowedActions {
		if a == action {
			return true
		}
	}
	return false
}

// Service errors (string keys mirror project convention).
var (
	ErrForbidden       = errors.New("forbidden")
	ErrNotFound        = errors.New("kelas not found")
	ErrInvalidAction   = errors.New("invalid action filter")
	ErrInvalidPaginate = errors.New("invalid pagination")
)

// auditRepo is the subset of auth.Repo we need.
type auditRepo interface {
	ListAuditLogs(ctx context.Context, f auth.AuditLogFilter, limit, offset int) ([]auth.AuditLog, int64, error)
}

// kelasFinder fetches kelas by id for ownership check.
type kelasFinder interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelasMini, error)
}

// kelasMini is a minimal projection of kelas for ownership check (avoid
// tight import to internal/kelas; instead the caller adapts internal/kelas
// Repo into this interface via an adapter at wire site).
type kelasMini struct {
	ID      uuid.UUID
	GuruID  uuid.UUID
	Status  string
}

// userLookup resolves user IDs → names for actor enrichment.
type userLookup interface {
	BulkUserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error)
}

// Service exposes the guru audit log read flow.
type Service struct {
	repo  auditRepo
	kelas kelasFinder
	users userLookup
}

// NewService wires the deps.
func NewService(repo auditRepo, kelas kelasFinder, users userLookup) *Service {
	return &Service{repo: repo, kelas: kelas, users: users}
}

// ListByKelas returns enriched audit entries scoped to a single kelas
// (locked #59). Guru ownership-checked; admin sees any kelas.
//
// action: empty=no filter (all allowlisted actions), non-empty=must be
// in AllowedActions or returns ErrInvalidAction.
// limit: clamped 1..100, default 50 if ≤0. offset: ≥0.
func (s *Service) ListByKelas(
	ctx context.Context,
	kelasID, callerID uuid.UUID,
	callerRole, action string,
	limit, offset int,
) (*ListResponse, error) {
	if callerRole != string(auth.Admin) && callerRole != string(auth.Guru) {
		return nil, ErrForbidden
	}
	if action != "" && !IsAllowedAction(action) {
		return nil, ErrInvalidAction
	}
	if offset < 0 {
		return nil, ErrInvalidPaginate
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	// Ownership.
	k, err := s.kelas.FindByID(ctx, kelasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("audit kelas find: %w", err)
	}
	if callerRole == string(auth.Guru) && k.GuruID != callerID {
		return nil, ErrForbidden
	}

	filter := auth.AuditLogFilter{
		TargetKelasID: &kelasID,
		Action:        action,
	}
	rows, total, err := s.repo.ListAuditLogs(ctx, filter, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("audit list: %w", err)
	}

	// Enrich actor names.
	actorIDs := make([]uuid.UUID, 0, len(rows))
	seen := make(map[uuid.UUID]struct{})
	for _, r := range rows {
		if r.ActorID == nil {
			continue
		}
		if _, ok := seen[*r.ActorID]; ok {
			continue
		}
		seen[*r.ActorID] = struct{}{}
		actorIDs = append(actorIDs, *r.ActorID)
	}
	names := map[uuid.UUID]string{}
	if len(actorIDs) > 0 {
		names, err = s.users.BulkUserNames(ctx, actorIDs)
		if err != nil {
			// Soft-fail enrichment — don't break the list.
			names = map[uuid.UUID]string{}
		}
	}

	out := make([]Entry, 0, len(rows))
	for _, r := range rows {
		e := Entry{
			ID:            r.ID,
			ActorID:       r.ActorID,
			ActorRole:     r.ActorRole,
			Action:        r.Action,
			TargetType:    r.TargetType,
			TargetID:      r.TargetID,
			TargetKelasID: r.TargetKelasID,
			Meta:          r.Meta,
			At:            r.At,
		}
		if r.ActorID != nil {
			if n, ok := names[*r.ActorID]; ok && n != "" {
				name := n
				e.ActorName = &name
			}
		}
		out = append(out, e)
	}
	return &ListResponse{
		Events: out,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}
