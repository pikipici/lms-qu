// HTTP handler: nilai siswa endpoints (Task 7.A.1).
//
//   - GET /api/v1/siswa/kelas/:id/nilai
//   - GET /api/v1/siswa/nilai
package nilai

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

// Handler exposes the nilai HTTP endpoints.
type Handler struct {
	svc serviceAPI
}

type serviceAPI interface {
	SiswaKelasNilai(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) (*SiswaKelasNilaiResponse, error)
	SiswaList(ctx context.Context, siswaID uuid.UUID, callerRole string) (*SiswaListResponse, error)
}

// NewHandler wires the service into a Fiber handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SiswaKelasNilai handles GET /api/v1/siswa/kelas/:id/nilai.
func (h *Handler) SiswaKelasNilai(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.SiswaKelasNilai(c.UserContext(), kelasID, siswaID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// SiswaList handles GET /api/v1/siswa/nilai (cross-kelas).
func (h *Handler) SiswaList(c *fiber.Ctx) error {
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.SiswaList(c.UserContext(), siswaID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// mapErr translates Service sentinel errors to HTTP responses.
func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound):
		return errResp(c, fiber.StatusNotFound, "not found", "not_found")
	default:
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
