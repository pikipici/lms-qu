// HTTP handler for Latihan flow (Task 5.C.2).
//
// Routes (all siswa-only):
//   - POST /api/v1/siswa/bab/:id/latihan/start
//   - POST /api/v1/siswa/hasil-soal-bab/:id/answer
//   - POST /api/v1/siswa/hasil-soal-bab/:id/finish
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

// latihanService is the subset of *LatihanService the handler uses.
type latihanService interface {
	Start(ctx context.Context, babID, siswaID uuid.UUID) (*StartResult, error)
	Answer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) (*AnswerResult, error)
	Finish(ctx context.Context, hasilID, siswaID uuid.UUID) (*FinishResult, error)
}

// LatihanHandler wires HTTP routes to LatihanService.
type LatihanHandler struct {
	svc latihanService
}

// NewLatihanHandler returns the HTTP handler for latihan endpoints.
func NewLatihanHandler(svc *LatihanService) *LatihanHandler {
	return &LatihanHandler{svc: svc}
}

// Start handles POST /api/v1/siswa/bab/:id/latihan/start.
func (h *LatihanHandler) Start(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.Start(c.UserContext(), babID, siswaID)
	if err != nil {
		return mapLatihanErr(c, err)
	}
	status := fiber.StatusCreated
	if res.Resume {
		status = fiber.StatusOK
	}
	return c.Status(status).JSON(fiber.Map{"hasil": res})
}

type answerRequest struct {
	SoalID  string `json:"soal_id"`
	Jawaban string `json:"jawaban"`
}

// Answer handles POST /api/v1/siswa/hasil-soal-bab/:id/answer.
func (h *LatihanHandler) Answer(c *fiber.Ctx) error {
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

	res, err := h.svc.Answer(c.UserContext(), hasilID, siswaID, AnswerInput{SoalID: soalID, Jawaban: jaw})
	if err != nil {
		return mapLatihanErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"answer": res})
}

// Finish handles POST /api/v1/siswa/hasil-soal-bab/:id/finish.
func (h *LatihanHandler) Finish(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.Finish(c.UserContext(), hasilID, siswaID)
	if err != nil {
		return mapLatihanErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"summary": res})
}

// mapLatihanErr maps Latihan sentinels → HTTP status + stable code.
func mapLatihanErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrLatihanPoolEmpty):
		return errResp(c, fiber.StatusBadRequest, "bab ini belum punya soal latihan", "latihan_pool_empty")
	case errors.Is(err, ErrSoalNotInPool):
		return errResp(c, fiber.StatusBadRequest, "soal tidak termasuk dalam attempt ini", "soal_not_in_pool")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrHasilNotOwned):
		return errResp(c, fiber.StatusForbidden, "attempt bukan milik kamu", "forbidden")
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrBabNotPublished):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
	case errors.Is(err, ErrHasilModeInvalid):
		return errResp(c, fiber.StatusConflict, "attempt ini bukan latihan", "hasil_mode_invalid")
	case errors.Is(err, ErrHasilAlreadyFinished):
		return errResp(c, fiber.StatusConflict, "attempt sudah selesai; mulai latihan baru kalau mau ulang", "hasil_already_finished")
	case errors.Is(err, ErrHasilCancelled):
		return errResp(c, fiber.StatusConflict, "attempt dibatalkan oleh guru", "hasil_cancelled")
	default:
		slog.Error("soalbab latihan handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
