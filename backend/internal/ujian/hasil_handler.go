// HTTP handler for Ujian Hasil flow (Task 6.E.1).
// Mirror soalbab/hasil_handler.go adapted untuk Ujian.
//
// Routes:
//   - GET  /api/v1/siswa/hasil-ujian/:id/review        → siswa review jawaban
//   - GET  /api/v1/siswa/kelas/:id/ujian/hasil         → siswa list hasil per kelas
//   - POST /api/v1/hasil-ujian/:id/cancel              → guru/admin cancel
//   - GET  /api/v1/ujian/:id/hasil-rekap               → guru/admin rekap
package ujian

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
	ListSiswaHasil(ctx context.Context, kelasID, siswaID uuid.UUID) (*SiswaHasilListResult, error)
	Cancel(ctx context.Context, hasilID, callerID uuid.UUID, callerRole, ip, userAgent string) (*CancelResult, error)
	Rekap(ctx context.Context, ujianID, callerID uuid.UUID, callerRole string) (*RekapResult, error)
}

// HasilHandler wires HTTP routes to HasilService.
type HasilHandler struct {
	svc hasilService
}

// NewHasilHandler constructs the handler.
func NewHasilHandler(svc *HasilService) *HasilHandler {
	return &HasilHandler{svc: svc}
}

// Review handles GET /siswa/hasil-ujian/:id/review.
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

// ListSiswa handles GET /siswa/kelas/:id/ujian/hasil.
func (h *HasilHandler) ListSiswa(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.ListSiswaHasil(c.UserContext(), kelasID, siswaID)
	if err != nil {
		return mapHasilErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"hasil": res})
}

// Cancel handles POST /hasil-ujian/:id/cancel (guru/admin).
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

// Rekap handles GET /ujian/:id/hasil-rekap (guru/admin).
func (h *HasilHandler) Rekap(c *fiber.Ctx) error {
	ujianID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	res, err := h.svc.Rekap(c.UserContext(), ujianID, callerID, role)
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
	case errors.Is(err, ErrHasilNotOwned):
		return errResp(c, fiber.StatusForbidden, "attempt bukan milik kamu", "forbidden")
	case errors.Is(err, ErrHasilAlreadyCancelled):
		return errResp(c, fiber.StatusConflict, "attempt sudah dibatalkan", "hasil_already_cancelled")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
	default:
		slog.Error("ujian hasil handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
