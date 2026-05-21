// Siswa-side mark-as-read flow untuk materi (Task 3.C.4).
//
// Endpoint: POST /api/v1/materi/:id/read (siswa-only).
//
// Service.MarkRead is idempotent — second call for the same (materi, siswa)
// pair is a no-op at the repo layer (ON CONFLICT DO NOTHING) and returns
// wasNew=false. Audit log is intentionally skipped (read events would be too
// chatty for the audit table); a slog debug entry tracks per-call activity
// instead.
package materi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
)

// MarkReadResult is the payload returned by Service.MarkRead and the
// handler. WasNew differentiates first-read from idempotent re-reads.
type MarkReadResult struct {
	MateriID uuid.UUID `json:"materi_id"`
	ReadAt   string    `json:"read_at"`
	WasNew   bool      `json:"was_new"`
}

// enrollmentLookup verifies the siswa is enrolled in the materi's kelas.
// Implemented by *kelas.Repo (FindEnrollment). Pass nil to disable the
// MarkRead path — used in 3.C.2-only test fixtures and in main.go before
// 3.C.4 wiring.
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// MarkRead inserts a materi_read row for the calling siswa idempotently.
// Authorization rules:
//   - Caller role must be siswa (handler enforces with RoleGuard, service
//     re-checks defensively so unit tests don't need a fiber app).
//   - Materi must exist (else ErrNotFound).
//   - Siswa must have an active enrollment in materi.KelasID (else
//     ErrForbidden — both "no enrollment row" and "status=removed" map
//     here).
//
// Repo.MarkRead is ON CONFLICT DO NOTHING; returns wasNew=true the first
// time and wasNew=false on subsequent calls (read_at preserved).
func (s *Service) MarkRead(ctx context.Context, materiID, siswaID uuid.UUID, callerRole string) (*MarkReadResult, error) {
	if callerRole != string(auth.Siswa) {
		return nil, ErrForbidden
	}
	if s.enroll == nil {
		return nil, fmt.Errorf("materi: enrollment lookup not configured")
	}

	m, err := s.repo.FindByID(ctx, materiID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("materi mark-read find: %w", err)
	}

	enr, err := s.enroll.FindEnrollment(ctx, m.KelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, fmt.Errorf("materi mark-read enrollment: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}

	row, wasNew, err := s.repo.MarkRead(ctx, materiID, siswaID)
	if err != nil {
		return nil, fmt.Errorf("materi mark-read insert: %w", err)
	}

	slog.Debug("materi_read",
		slog.String("materi_id", materiID.String()),
		slog.String("siswa_id", siswaID.String()),
		slog.Bool("was_new", wasNew),
	)
	return &MarkReadResult{
		MateriID: row.MateriID,
		ReadAt:   row.ReadAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		WasNew:   wasNew,
	}, nil
}

// MarkRead handles POST /api/v1/materi/:id/read.
//
// 200 { materi_id, read_at, was_new } — was_new=true on first call, false
// on subsequent calls (idempotent). 403 when caller isn't the enrolled
// siswa. 404 when materi missing.
func (h *Handler) MarkRead(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid materi id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.MarkRead(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}
