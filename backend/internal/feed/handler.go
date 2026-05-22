// HTTP handler: GET /api/v1/guru/feed (Task 7.C activity feed).
package feed

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

// Handler exposes the feed endpoint.
type Handler struct {
	svc serviceAPI
}

type serviceAPI interface {
	List(ctx context.Context, guruID uuid.UUID, callerRole string, cursor string, limit int) (*ListResponse, error)
}

// NewHandler wires the service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// List handles GET /api/v1/guru/feed.
func (h *Handler) List(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	cursor := strings.TrimSpace(c.Query("cursor"))
	limit := 0
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		n, perr := strconv.Atoi(v)
		if perr != nil || n < 1 {
			return errResp(c, fiber.StatusBadRequest, "invalid limit", "invalid_limit")
		}
		limit = n
	}

	res, err := h.svc.List(c.UserContext(), userID, role, cursor, limit)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
		case errors.Is(err, ErrInvalidCursor):
			return errResp(c, fiber.StatusBadRequest, "invalid cursor", "invalid_cursor")
		default:
			return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
		}
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
