// HTTP handler untuk siswa-side enrollment endpoints. Lives in kelas package
// so it shares the same Service + audit plumbing; logically scoped to siswa
// flow (mounted under /api/v1/siswa di main.go).
package kelas

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
)

type joinByKodeRequest struct {
	KodeInvite string `json:"kode_invite"`
}

type joinByKodeResponse struct {
	Kelas    *Kelas `json:"kelas"`
	Inserted bool   `json:"inserted"`
}

// JoinByKode handles POST /api/v1/siswa/kelas/join — siswa joins a kelas via
// kode invite. Idempotent: returning inserted=false means siswa was already
// enrolled. Use ErrEnrollmentRemoved to signal "you were removed; ask guru".
func (h *Handler) JoinByKode(c *fiber.Ctx) error {
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	var req joinByKodeRequest
	if err := c.BodyParser(&req); err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if strings.TrimSpace(req.KodeInvite) == "" {
		return kelasError(c, fiber.StatusBadRequest, "kode invite is required", "kode_invite_required")
	}

	res, err := h.svc.JoinByKode(c.UserContext(), siswaID, JoinByKodeInput{
		KodeInvite: req.KodeInvite,
	}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapEnrollmentErr(c, err)
	}

	status := fiber.StatusCreated
	if !res.Inserted {
		// Idempotent — already enrolled. Surface 200 OK so the FE can branch
		// without treating it as an error.
		status = fiber.StatusOK
	}
	return c.Status(status).JSON(joinByKodeResponse{
		Kelas:    res.Kelas,
		Inserted: res.Inserted,
	})
}

func mapEnrollmentErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrKodeInviteEmpty):
		return kelasError(c, fiber.StatusBadRequest, "kode invite is required", "kode_invite_required")
	case errors.Is(err, ErrKodeInviteNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return kelasError(c, fiber.StatusNotFound, "kode invite tidak ditemukan", "kode_invite_not_found")
	case errors.Is(err, ErrKelasArchived):
		return kelasError(c, fiber.StatusConflict, "kelas sudah diarsipkan", "kelas_archived")
	case errors.Is(err, ErrEnrollmentRemoved):
		return kelasError(c, fiber.StatusConflict, "kamu pernah dikeluarkan dari kelas ini; minta guru/admin untuk mendaftarkan ulang", "enrollment_removed")
	default:
		slog.Error("kelas enrollment handler", slog.String("err", err.Error()))
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}
