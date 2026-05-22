// Flow handlers — Start + Items + Answer endpoints (Task 6.D.1, 6.D.2).
//
// Routes (siswa role-guarded):
//   - POST /api/v1/siswa/ujian/:id/start                 (6.D.1)
//   - GET  /api/v1/siswa/hasil-ujian/:id/items           (6.D.1)
//   - POST /api/v1/siswa/hasil-ujian/:id/answer          (6.D.2)
package ujian

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/middleware"
)

// flowSvc is the subset of *FlowService the handler depends on.
type flowSvc interface {
	Start(ctx context.Context, ujianID, siswaID uuid.UUID, ip, userAgent string) (*StartResult, error)
	SaveAnswer(ctx context.Context, hasilID, siswaID uuid.UUID, in AnswerInput) error
	Submit(ctx context.Context, hasilID, siswaID uuid.UUID, ip, userAgent string) (*SubmitResult, error)
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

// answerRequest is the JSON body for POST .../answer.
type answerRequest struct {
	SoalID  string `json:"soal_id"`
	Jawaban string `json:"jawaban"`
}

// Answer handles POST /api/v1/siswa/hasil-ujian/:id/answer (6.D.2).
//
// Body: { "soal_id": "<uuid>", "jawaban": "a|b|c|d|e" }
//
// Delayed grade: success returns 200 {"ok": true} tanpa is_benar/jawaban_benar
// (locked #76 mirror). Grading dilakukan saat submit (6.D.3) atau cron
// auto-grade (6.D.4 locked #87).
func (h *FlowHandler) Answer(c *fiber.Ctx) error {
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
	jaw := banksoal.Jawaban(strings.ToLower(strings.TrimSpace(req.Jawaban)))
	if !jaw.Valid() {
		return errResp(c, fiber.StatusBadRequest, "jawaban must be a|b|c|d|e", "invalid_jawaban")
	}

	if err := h.flow.SaveAnswer(c.UserContext(), hasilID, siswaID,
		AnswerInput{SoalID: soalID, Jawaban: jaw}); err != nil {
		return mapFlowErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ok": true})
}

// Submit handles POST /api/v1/siswa/hasil-ujian/:id/submit (6.D.3).
//
// Single-tx auto-grade dengan pg_advisory_xact_lock per hasil_id;
// race-safe vs cron auto-grade (6.D.4 reuse same key locked #87).
// Idempotent — kalau attempt sudah selesai, balikin existing rekap
// dengan already_submitted=true.
func (h *FlowHandler) Submit(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.flow.Submit(c.UserContext(), hasilID, siswaID,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapFlowErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"summary": res})
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
	case errors.Is(err, ErrSoalNotInPool):
		return errResp(c, fiber.StatusBadRequest, "soal tidak termasuk dalam attempt ini", "soal_not_in_pool")
	case errors.Is(err, ErrUjianSubmitAfterGrace):
		return errResp(c, fiber.StatusGone, "submit terlambat; nilai akan diproses oleh sistem", "submit_after_grace")
	case errors.Is(err, ErrHasilCancelled):
		return errResp(c, fiber.StatusConflict, "attempt dibatalkan oleh guru", "hasil_cancelled")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "kamu tidak terdaftar di kelas ujian ini", "forbidden")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "ujian tidak ditemukan", "not_found")
	default:
		slog.Error("ujian flow handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
