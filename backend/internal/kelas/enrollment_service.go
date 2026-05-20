// Enrollment-related service methods (Phase 2.C). Lives in the kelas package
// because it shares Repo + auditLogger plumbing; logically distinct from kelas
// CRUD so flows are isolated in this file.
package kelas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// Enrollment-domain sentinels (mapped to HTTP at the handler layer).
var (
	ErrKodeInviteEmpty     = errors.New("kelas: kode invite is empty")
	ErrKodeInviteNotFound  = errors.New("kelas: kode invite not found")
	ErrKelasArchived       = errors.New("kelas: archived, cannot join")
	ErrAlreadyEnrolled     = errors.New("kelas: already enrolled (active)")
	ErrEnrollmentRemoved   = errors.New("kelas: previously removed; ask guru/admin to re-enroll")
)

// JoinByKodeInput is the siswa-supplied payload for kode-invite join.
type JoinByKodeInput struct {
	KodeInvite string
}

// JoinByKodeResult bundles the freshly-resolved kelas + whether the row was
// newly written (false means "join idempotent — siswa was already enrolled").
type JoinByKodeResult struct {
	Kelas    *Kelas
	Inserted bool
}

// JoinByKode lets a siswa join a kelas by its 6-char kode invite. The flow:
//
//  1. Trim + UPPERCASE the input (charset is uppercase-only; tahan typo lower).
//  2. Find kelas by kode_invite. Missing → ErrKodeInviteNotFound.
//  3. Reject join when archived (kelas tutup → ErrKelasArchived).
//  4. Insert enrollment via Repo.Enroll (ON CONFLICT DO NOTHING). If a removed
//     row already exists, surface ErrEnrollmentRemoved so the siswa can ask the
//     guru to re-enroll explicitly (we do NOT silently re-activate).
//  5. Audit `siswa_joined_kelas` with target_kelas_id (locked decision #59).
//
// Returns Inserted=false when the siswa was already an active member —
// idempotent UX so the FE can show a friendly "udah di kelas ini" toast
// without erroring out.
func (s *Service) JoinByKode(ctx context.Context, siswaID uuid.UUID, in JoinByKodeInput, ip, userAgent string) (*JoinByKodeResult, error) {
	kode := strings.ToUpper(strings.TrimSpace(in.KodeInvite))
	if kode == "" {
		return nil, ErrKodeInviteEmpty
	}

	k, err := s.repo.FindByKodeInvite(ctx, kode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrKodeInviteNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("kelas join find: %w", err)
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	// Detect prior removed enrollment so we don't silently re-activate.
	if existing, ferr := s.repo.FindEnrollment(ctx, k.ID, siswaID); ferr == nil {
		if existing.Status == EnrollmentRemoved {
			return nil, ErrEnrollmentRemoved
		}
		// Active row already exists → idempotent join, audit the no-op.
		s.logAuditWithRole(ctx, "siswa_join_kelas_noop", string(auth.Siswa),
			&siswaID, &k.ID, &k.ID, ip, userAgent, map[string]any{
				"kode_invite": k.KodeInvite,
				"reason":      "already_enrolled",
			})
		return &JoinByKodeResult{Kelas: k, Inserted: false}, nil
	} else if !errors.Is(ferr, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("kelas join find enrollment: %w", ferr)
	}

	inserted, err := s.repo.Enroll(ctx, k.ID, siswaID, JoinedViaKode)
	if err != nil {
		return nil, fmt.Errorf("kelas enroll: %w", err)
	}

	if inserted {
		s.logAuditWithRole(ctx, "siswa_joined_kelas", string(auth.Siswa),
			&siswaID, &k.ID, &k.ID, ip, userAgent, map[string]any{
				"kode_invite": k.KodeInvite,
				"joined_via":  string(JoinedViaKode),
			})
	}

	return &JoinByKodeResult{Kelas: k, Inserted: inserted}, nil
}
