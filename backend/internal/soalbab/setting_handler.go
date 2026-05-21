// HTTP handler for UlanganBabSetting endpoints (Task 5.C.1).
//
// Routes:
//   - GET  /api/v1/bab/:id/ulangan-setting (guru/admin owner OR siswa enrolled)
//   - PUT  /api/v1/bab/:id/ulangan-setting (guru/admin owner only)
package soalbab

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

// settingService is the subset of *SettingService the handler depends on.
type settingService interface {
	GetForGuru(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*SettingView, error)
	GetForSiswa(ctx context.Context, babID, siswaID uuid.UUID) (*SiswaLobbyView, error)
	Upsert(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in UpsertSettingInput, ip, userAgent string) (*SettingView, error)
}

// SettingHandler wires HTTP routes to *SettingService.
type SettingHandler struct {
	svc settingService
}

// NewSettingHandler returns the HTTP handler for setting endpoints.
func NewSettingHandler(svc *SettingService) *SettingHandler {
	return &SettingHandler{svc: svc}
}

// upsertSettingRequest is the PUT JSON payload. Pointer fields used
// where presence is needed; required scalars stay non-pointer because
// zero is invalid for all of them (caught by validateSettingBounds).
type upsertSettingRequest struct {
	JumlahSoal                 int16   `json:"jumlah_soal"`
	DurasiMenit                int16   `json:"durasi_menit"`
	BatasAttempt               int16   `json:"batas_attempt"`
	IzinkanReviewSetelahSubmit bool    `json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *string `json:"waktu_buka_review,omitempty"`
	Version                    int     `json:"version"`
}

// Get handles GET /api/v1/bab/:id/ulangan-setting. Branches on caller
// role: guru/admin → full payload; siswa → trimmed lobby payload.
func (h *SettingHandler) Get(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	if role == string(auth.Siswa) {
		view, err := h.svc.GetForSiswa(c.UserContext(), babID, callerID)
		if err != nil {
			return mapSettingErr(c, err)
		}
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"setting": view})
	}

	view, err := h.svc.GetForGuru(c.UserContext(), babID, callerID, role)
	if err != nil {
		return mapSettingErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"setting": view})
}

// Upsert handles PUT /api/v1/bab/:id/ulangan-setting.
func (h *SettingHandler) Upsert(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	// Reject siswa explicitly so the audit log shows a hard 403 instead
	// of leaking ErrInvalidInput from a guru-only validator.
	if role == string(auth.Siswa) {
		return errResp(c, fiber.StatusForbidden, "siswa tidak diizinkan mengubah setting", "forbidden")
	}

	// Use json.Decoder w/ DisallowUnknownFields-style strictness via a
	// raw pre-check: empty body → 400 invalid_body to avoid silently
	// accepting all-zero payloads.
	raw := c.Body()
	if len(strings.TrimSpace(string(raw))) == 0 {
		return errResp(c, fiber.StatusBadRequest, "request body required", "invalid_body")
	}
	var req upsertSettingRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := UpsertSettingInput{
		ExpectedVersion:            req.Version,
		JumlahSoal:                 req.JumlahSoal,
		DurasiMenit:                req.DurasiMenit,
		BatasAttempt:               req.BatasAttempt,
		IzinkanReviewSetelahSubmit: req.IzinkanReviewSetelahSubmit,
	}
	if req.WaktuBukaReview != nil {
		raw := strings.TrimSpace(*req.WaktuBukaReview)
		if raw == "" {
			// Treat empty string as "clear the field".
			in.WaktuBukaReview = nil
		} else {
			t, perr := time.Parse(time.RFC3339, raw)
			if perr != nil {
				return errResp(c, fiber.StatusBadRequest, "waktu_buka_review must be RFC3339", "invalid_waktu_buka_review")
			}
			in.WaktuBukaReview = &t
		}
	}

	view, err := h.svc.Upsert(
		c.UserContext(), babID, callerID, role, in,
		c.IP(), string(c.Request().Header.UserAgent()),
	)
	if err != nil {
		return mapSettingErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"setting": view})
}

// mapSettingErr maps soalbab service sentinels (incl. setting-specific
// ones) to HTTP status + stable error code. Mirrors mapServiceErr but
// adds setting-flavored codes.
func mapSettingErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrSettingPoolExceeded):
		return errResp(c, fiber.StatusBadRequest, "jumlah_soal melebihi jumlah soal ulangan tersedia", "jumlah_soal_exceeds_pool")
	case errors.Is(err, ErrSettingPoolEmpty):
		return errResp(c, fiber.StatusBadRequest, "belum ada soal ulangan di bab ini; tambahkan soal terlebih dahulu", "ulangan_pool_empty")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "bab not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrBabArchived):
		return errResp(c, fiber.StatusConflict, "bab is archived; setting cannot be modified", "bab_archived")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "setting has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("soalbab setting handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
