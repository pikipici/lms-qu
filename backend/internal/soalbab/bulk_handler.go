// HTTP handler untuk SoalBab bulk paste pipe-delimited (Task 5.B.3).
//
// Endpoint:
//   POST /api/v1/bab/:id/soal/bulk
//   body { rows: string, mode_default?: latihan|ulangan|keduanya }
//
// Hard preconditions (4xx):
//   - 400 invalid_id / invalid_body / rows_required / too_many / invalid_mode_default
//   - 403 forbidden (not kelas owner / admin)
//   - 404 not_found (bab missing)
//   - 409 bab_archived
//
// Response 200 partial-success:
//   { created: int, errors: [{line: int, reason: string, raw: string}] }
//
// Reason codes (locked Task 5.B.3 + skill bulk-partial-success-classify-endpoint):
//   invalid_columns, empty_pertanyaan, empty_jawaban_option, invalid_jawaban,
//   invalid_poin, invalid_mode, pertanyaan_too_long, opsi_too_long.
package soalbab

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

type bulkRequest struct {
	Rows        string `json:"rows"`
	ModeDefault string `json:"mode_default"`
}

// BulkCreate handles POST /api/v1/bab/:id/soal/bulk.
func (h *Handler) BulkCreate(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req bulkRequest
	if err := c.BodyParser(&req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := BulkCreateInput{
		Rows: req.Rows,
	}
	if md := strings.TrimSpace(req.ModeDefault); md != "" {
		in.ModeDefault = Mode(strings.ToLower(md))
	}

	res, err := h.svc.BulkCreate(c.UserContext(), babID, callerID, role, in,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapBulkErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

func mapBulkErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrBulkRowsRequired):
		return errResp(c, fiber.StatusBadRequest, "rows is required", "rows_required")
	case errors.Is(err, ErrBulkTooMany):
		return errResp(c, fiber.StatusBadRequest,
			fmt.Sprintf("rows exceeds max %d lines", MaxBulkLines),
			"too_many")
	default:
		return mapServiceErr(c, err)
	}
}
