// HTTP handler for Ulangan Bab flow (Task 5.D.1 — start endpoint only).
//
// Routes:
//   - POST /api/v1/siswa/bab/:id/ulangan/start
//
// Answer/submit/cron land in Task 5.D.2-5.D.4.
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

// ulanganService is the subset of *UlanganService the handler uses.
type ulanganService interface {
	Start(ctx context.Context, babID, siswaID uuid.UUID, ip, userAgent string) (*UlanganStartResult, error)
}

// UlanganHandler wires HTTP routes to UlanganService.
type UlanganHandler struct {
	svc ulanganService
}

// NewUlanganHandler returns the HTTP handler for ulangan endpoints.
func NewUlanganHandler(svc *UlanganService) *UlanganHandler {
	return &UlanganHandler{svc: svc}
}

// Start handles POST /api/v1/siswa/bab/:id/ulangan/start.
func (h *UlanganHandler) Start(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.Start(c.UserContext(), babID, siswaID,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapUlanganErr(c, err)
	}
	status := fiber.StatusCreated
	if res.Resume {
		status = fiber.StatusOK
	}
	return c.Status(status).JSON(fiber.Map{"hasil": res})
}

// mapUlanganErr maps Ulangan sentinels → HTTP status + stable code.
func mapUlanganErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrUlanganSettingMissing):
		return errResp(c, fiber.StatusConflict, "guru belum mengaktifkan ulangan untuk bab ini", "ulangan_setting_missing")
	case errors.Is(err, ErrBatasAttemptExceeded):
		return errResp(c, fiber.StatusForbidden, "kamu sudah mencapai batas attempt", "batas_attempt_exceeded")
	case errors.Is(err, ErrUlanganPoolInsufficient):
		return errResp(c, fiber.StatusConflict, "jumlah soal ulangan kurang dari kebutuhan setting; minta guru tambah soal", "ulangan_pool_insufficient")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
	default:
		slog.Error("soalbab ulangan handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
