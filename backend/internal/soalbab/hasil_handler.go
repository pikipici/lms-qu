// HTTP handler for Hasil flow (Task 5.E.1 + Task 5.G.1).
//
// Routes:
//   - GET  /api/v1/siswa/hasil-soal-bab/:id/review  → siswa review jawaban
//   - GET  /api/v1/siswa/hasil-soal-bab/:id/items   → siswa live attempt items (Task 5.G.1)
//   - GET  /api/v1/siswa/bab/:id/hasil              → siswa list hasil sendiri di bab
//   - POST /api/v1/hasil-soal-bab/:id/cancel        → guru/admin cancel
//   - GET  /api/v1/bab/:id/hasil-rekap              → guru/admin rekap
package soalbab

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
)

// hasilService is the subset of *HasilService the handler uses.
type hasilService interface {
	Review(ctx context.Context, hasilID, siswaID uuid.UUID) (*ReviewResult, error)
	Items(ctx context.Context, hasilID, siswaID uuid.UUID) (*ItemsResult, error)
	ListSiswaHasil(ctx context.Context, babID, siswaID uuid.UUID) (*SiswaHasilListResult, error)
	Cancel(ctx context.Context, hasilID, callerID uuid.UUID, callerRole, ip, userAgent string) (*CancelResult, error)
	Rekap(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*RekapResult, error)
}

// HasilHandler wires HTTP routes to HasilService.
type HasilHandler struct {
	svc hasilService
}

// NewHasilHandler constructs the handler.
func NewHasilHandler(svc *HasilService) *HasilHandler {
	return &HasilHandler{svc: svc}
}

// Review handles GET /siswa/hasil-soal-bab/:id/review.
func (h *HasilHandler) Review(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.Review(c.UserContext(), hasilID, siswaID)
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"review": res})
}

// Items handles GET /siswa/hasil-soal-bab/:id/items.
//
// Returns live attempt items for the calling siswa's active (status=berlangsung)
// attempt. Anti-cheat: jawaban_benar di-strip out (locked #76). Image slots
// di-presign per call (TTL 15m). Latihan jawaban_siswa.is_benar
// surfaced untuk pre-fill banner; ulangan stays NULL until submit.
func (h *HasilHandler) Items(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.Items(c.UserContext(), hasilID, siswaID)
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"attempt": res})
}

// ListSiswa handles GET /siswa/bab/:id/hasil.
func (h *HasilHandler) ListSiswa(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.ListSiswaHasil(c.UserContext(), babID, siswaID)
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"hasil": res})
}

// Cancel handles POST /hasil-soal-bab/:id/cancel (guru/admin).
func (h *HasilHandler) Cancel(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	res, err := h.svc.Cancel(c.UserContext(), hasilID, callerID, role,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"hasil": res})
}

// Rekap handles GET /bab/:id/hasil-rekap (guru/admin).
func (h *HasilHandler) Rekap(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	res, err := h.svc.Rekap(c.UserContext(), babID, callerID, role)
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"rekap": res})
}

// mapHasilErr maps Hasil sentinels → HTTP status + stable code.
func mapHasilErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrReviewLocked):
		return errResp(c, fiber.StatusForbidden, "review belum dibuka guru", "review_locked")
	case errors.Is(err, ErrReviewDisabled):
		return errResp(c, fiber.StatusForbidden, "review dimatikan guru", "review_disabled")
	case errors.Is(err, ErrHasilNotFinished):
		return errResp(c, fiber.StatusConflict, "attempt belum selesai", "hasil_not_finished")
	case errors.Is(err, ErrHasilNotActive):
		return errResp(c, fiber.StatusConflict, "attempt sudah selesai atau dibatalkan; pakai endpoint review", "hasil_not_active")
	case errors.Is(err, ErrCancelLatihan):
		return errResp(c, fiber.StatusBadRequest, "latihan tidak perlu di-cancel", "cancel_latihan")
	case errors.Is(err, ErrHasilNotOwned):
		return errResp(c, fiber.StatusForbidden, "attempt bukan milik kamu", "forbidden")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
	default:
		slog.Error("soalbab hasil handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
