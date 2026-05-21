// Attempt answer dispatcher — Task 5.D.2.
//
// POST /api/v1/siswa/hasil-soal-bab/:id/answer is shared between latihan
// (immediate is_benar feedback, locked #81) and ulangan (delayed grade,
// no feedback, locked #76). The route reads hasil.mode and forwards to
// the appropriate handler so the FE can hit a single endpoint and the
// service-layer divergence stays clean.
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

// attemptModePeeker is the minimal API needed to look up hasil.mode
// before dispatching. Implemented by *Repo.
type attemptModePeeker interface {
	FindHasilByID(ctx context.Context, id uuid.UUID) (*HasilSoalBab, error)
}

// AttemptAnswerHandler dispatches POST /siswa/hasil-soal-bab/:id/answer
// to the right service based on hasil.mode. Hasil ownership is verified
// downstream (latihan/ulangan service guards on siswa_id), so the peek
// here is mode-only and doesn't leak existence — non-existent hasil
// returns the same not_found shape the inner handlers produce.
type AttemptAnswerHandler struct {
	repo    attemptModePeeker
	latihan *LatihanHandler
	ulangan *UlanganHandler
}

// NewAttemptAnswerHandler wires the dispatcher.
func NewAttemptAnswerHandler(repo attemptModePeeker, lh *LatihanHandler, uh *UlanganHandler) *AttemptAnswerHandler {
	return &AttemptAnswerHandler{repo: repo, latihan: lh, ulangan: uh}
}

// Answer dispatches to the right inner handler. If we can't peek the
// hasil row (not found / db error), we fall through to the latihan
// handler which already knows how to surface ErrNotFound — keeps
// behaviour identical to pre-5.D.2 for the not-found case.
func (h *AttemptAnswerHandler) Answer(c *fiber.Ctx) error {
	hasilID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid hasil id", "invalid_id")
	}
	// Body emptiness guard up here so both branches share the same shape.
	raw := c.Body()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return errResp(c, fiber.StatusBadRequest, "request body required", "invalid_body")
	}

	hasil, err := h.repo.FindHasilByID(c.UserContext(), hasilID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errResp(c, fiber.StatusNotFound, "tidak ditemukan", "not_found")
		}
		slog.Error("soalbab attempt answer peek", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	// Defensive: we don't return an error for ownership mismatch here —
	// the inner handlers do the proper 403. We only branch on mode.
	_ = middleware.UserIDFromCtx // ensure import retained (used by inner handlers).

	switch hasil.Mode {
	case HasilModeUlangan:
		return h.ulangan.Answer(c)
	case HasilModeLatihan:
		return h.latihan.Answer(c)
	default:
		// Unknown mode — surface as conflict so frontend can drop the cache.
		return errResp(c, fiber.StatusConflict, "mode attempt tidak dikenali", "hasil_mode_invalid")
	}
}
