// Flow handlers — Start + Items endpoints (Task 6.D.1).
//
// Routes (siswa role-guarded):
//   - POST /api/v1/siswa/ujian/:id/start
//   - GET  /api/v1/siswa/hasil-ujian/:id/items
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

// flowSvc is the subset of *FlowService the handler depends on.
type flowSvc interface {
	Start(ctx context.Context, ujianID, siswaID uuid.UUID, ip, userAgent string) (*StartResult, error)
}

// itemsSvc is the subset of *ItemsService the handler depends on.
type itemsSvc interface {
	Items(ctx context.Context, hasilID, siswaID uuid.UUID) (*ItemsResult, error)
}

// FlowHandler wires Start + Items endpoints.
type FlowHandler struct {
	flow  flowSvc
	items itemsSvc
}

// NewFlowHandler returns a flow handler.
func NewFlowHandler(flow *FlowService, items *ItemsService) *FlowHandler {
	return &FlowHandler{flow: flow, items: items}
}

// Start handles POST /api/v1/siswa/ujian/:id/start.
func (h *FlowHandler) Start(c *fiber.Ctx) error {
	ujianID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.flow.Start(c.UserContext(), ujianID, siswaID,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapFlowErr(c, err)
	}
	status := fiber.StatusCreated
	if res.Resume {
		status = fiber.StatusOK
	}
	return c.Status(status).JSON(fiber.Map{"hasil": res})
}

// Items handles GET /api/v1/siswa/hasil-ujian/:id/items.
func (h *FlowHandler) Items(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.items.Items(c.UserContext(), hasilID, siswaID)
	if err != nil {
		return mapFlowErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func mapFlowErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrUjianNotPublished):
		return errResp(c, fiber.StatusForbidden, "ujian belum dipublikasi guru", "ujian_not_published")
	case errors.Is(err, ErrUjianSourceMissing):
		return errResp(c, fiber.StatusConflict, "guru belum konfigurasi sumber soal", "ujian_source_missing")
	case errors.Is(err, ErrUjianSourcePoolEmpty):
		return errResp(c, fiber.StatusConflict, "pool soal kosong (guru hapus bank soal post-setup)", "ujian_pool_empty")
	case errors.Is(err, ErrUjianAlreadyAttempted):
		return errResp(c, fiber.StatusConflict, "kamu sudah pernah ikut ujian ini", "ujian_already_attempted")
	case errors.Is(err, ErrUjianTimerExpired):
		return errResp(c, fiber.StatusGone, "waktu ujian sudah habis", "ujian_timer_expired")
	case errors.Is(err, ErrUjianWindowClosed):
		return errResp(c, fiber.StatusGone, "jadwal ujian sudah lewat", "ujian_window_closed")
	case errors.Is(err, ErrUjianWindowNotOpen):
		return errResp(c, fiber.StatusConflict, "ujian belum dimulai sesuai jadwal", "ujian_window_not_open")
	case errors.Is(err, ErrHasilNotOwned):
		return errResp(c, fiber.StatusForbidden, "attempt bukan milikmu", "hasil_not_owned")
	case errors.Is(err, ErrHasilNotActive):
		return errResp(c, fiber.StatusGone, "attempt sudah selesai/dibatalkan", "hasil_not_active")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "kamu tidak terdaftar di kelas ujian ini", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "ujian tidak ditemukan", "not_found")
	default:
		slog.Error("ujian flow handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
