// HTTP handler for Ulangan Bab flow (Task 5.D.1 + 5.D.2).
//
// Routes:
//   - POST /api/v1/siswa/bab/:id/ulangan/start          (5.D.1)
//   - POST /api/v1/siswa/hasil-soal-bab/:id/answer      (5.D.2 dispatched
//     by AttemptAnswerHandler — branches by hasil.mode)
//
// Submit/cron land in Task 5.D.3-5.D.4.
package soalbab

import (
	"context"
	"encoding/json"
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
	Answer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) error
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

// Answer handles POST /api/v1/siswa/hasil-soal-bab/:id/answer when the
// underlying hasil is mode='ulangan'. Wired via AttemptAnswerHandler.
func (h *UlanganHandler) Answer(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	raw := c.Body()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return errResp(c, fiber.StatusBadRequest, "request body required", "invalid_body")
	}
	var req answerRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	soalID, err := uuid.Parse(strings.TrimSpace(req.SoalID))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "soal_id must be a UUID", "invalid_soal_id")
	}
	jaw := Jawaban(strings.ToLower(strings.TrimSpace(req.Jawaban)))
	if !jaw.Valid() {
		return errResp(c, fiber.StatusBadRequest, "jawaban must be a|b|c|d|e", "invalid_jawaban")
	}

	if err := h.svc.Answer(c.UserContext(), hasilID, siswaID,
		AnswerInput{SoalID: soalID, Jawaban: jaw}); err != nil {
		return mapUlanganErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
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
	case errors.Is(err, ErrUlanganTimerExpired):
		return errResp(c, fiber.StatusGone, "waktu ulangan sudah habis", "timer_expired")
	case errors.Is(err, ErrSoalNotInPool):
		return errResp(c, fiber.StatusBadRequest, "soal tidak termasuk dalam attempt ini", "soal_not_in_pool")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrHasilNotOwned):
		return errResp(c, fiber.StatusForbidden, "attempt bukan milik kamu", "forbidden")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
	case errors.Is(err, ErrHasilModeInvalid):
		return errResp(c, fiber.StatusConflict, "attempt ini bukan ulangan", "hasil_mode_invalid")
	case errors.Is(err, ErrHasilAlreadyFinished):
		return errResp(c, fiber.StatusConflict, "attempt sudah selesai", "hasil_already_finished")
	case errors.Is(err, ErrHasilCancelled):
		return errResp(c, fiber.StatusConflict, "attempt dibatalkan oleh guru", "hasil_cancelled")
	default:
		slog.Error("soalbab ulangan handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
