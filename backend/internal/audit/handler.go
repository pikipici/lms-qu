package audit

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

// Handler bundles HTTP handlers untuk Task 7.E guru audit endpoints.
type Handler struct {
	svc *Service
}

// NewHandler wires the Handler against a Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// errResp keeps response shape consistent with sibling packages
// (nilai/feed/submission): {error, code, request_id}.
func errResp(c *fiber.Ctx, code int, msg, errKey string) error {
	return c.Status(code).JSON(fiber.Map{
		"error":      msg,
		"code":       errKey,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}

// ListByKelas handles GET /api/v1/guru/kelas/:id/audit?action=&limit=&offset=.
func (h *Handler) ListByKelas(c *fiber.Ctx) error {
	idStr := strings.TrimSpace(c.Params("id"))
	kelasID, err := uuid.Parse(idStr)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}

	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusUnauthorized, "missing user", "unauthorized")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	action := strings.TrimSpace(c.Query("action"))

	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return errResp(c, fiber.StatusBadRequest, "invalid limit", "invalid_limit")
		}
		limit = n
	}
	offset := 0
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return errResp(c, fiber.StatusBadRequest, "invalid offset", "invalid_offset")
		}
		offset = n
	}

	res, err := h.svc.ListByKelas(c.UserContext(), kelasID, callerID, role, action, limit, offset)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
		case errors.Is(err, ErrNotFound):
			return errResp(c, fiber.StatusNotFound, "kelas not found", "kelas_not_found")
		case errors.Is(err, ErrInvalidAction):
			return errResp(c, fiber.StatusBadRequest, "invalid action filter", "invalid_action")
		case errors.Is(err, ErrInvalidPaginate):
			return errResp(c, fiber.StatusBadRequest, "invalid offset", "invalid_offset")
		default:
			return errResp(c, fiber.StatusInternalServerError, "internal error", "internal_error")
		}
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// ListActions handles GET /api/v1/guru/audit-actions — returns the
// allowlisted action set for the FE filter dropdown.
func (h *Handler) ListActions(c *fiber.Ctx) error {
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	if role != string(auth.Admin) && role != string(auth.Guru) {
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	}
	return c.Status(fiber.StatusOK).JSON(ActionsResponse{Actions: AllowedActions})
}
