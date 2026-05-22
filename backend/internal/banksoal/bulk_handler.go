// HTTP handler untuk BankSoal bulk paste pipe-delimited (Task 6.B.3).
//
// Endpoint:
//   POST /api/v1/bank-soal/bulk
//   body { rows: string, mapel?: string, tingkat?: string, topik?: string }
//
// Tag default (mapel/tingkat/topik) di-apply ke setiap soal hasil parse —
// hemat user input. Bisa ubah per-soal via PATCH endpoint setelahnya.
//
// Hard preconditions (4xx):
//   - 400 invalid_body / rows_required / too_many / invalid_input (tag too long)
//   - 401 unauthorized (handled di middleware)
//   - 403 forbidden (siswa BLOCKED via RoleGuard)
//
// Response 200 partial-success:
//   { created: int, errors: [{line: int, reason: string, raw: string}] }
//
// Reason codes (locked + skill bulk-partial-success-classify-endpoint):
//   invalid_columns, empty_pertanyaan, empty_jawaban_option, invalid_jawaban,
//   invalid_poin, pertanyaan_too_long, opsi_too_long.
//
// Mirror SoalBab Task 5.B.3 (commit dabbdf1) — same shape, drop `mode`
// column dari format paste karena BankSoal cross-bab tanpa mode.
package banksoal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/pikip/lms/backend/internal/middleware"
)

type bulkRequest struct {
	Rows    string `json:"rows"`
	Mapel   string `json:"mapel"`
	Tingkat string `json:"tingkat"`
	Topik   string `json:"topik"`
}

// BulkCreate handles POST /api/v1/bank-soal/bulk.
func (h *Handler) BulkCreate(c *fiber.Ctx) error {
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
		Rows:    req.Rows,
		Mapel:   strings.TrimSpace(req.Mapel),
		Tingkat: strings.TrimSpace(req.Tingkat),
		Topik:   strings.TrimSpace(req.Topik),
	}

	res, err := h.svc.BulkCreate(c.UserContext(), callerID, role, in,
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
