// HTTP handler: nilai siswa endpoints (Task 7.A.1).
//
//   - GET /api/v1/siswa/kelas/:id/nilai
//   - GET /api/v1/siswa/nilai
//   - GET /api/v1/kelas/:id/rekap?format=json|csv (Task 7.B, guru/admin)
package nilai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

// Handler exposes the nilai HTTP endpoints.
type Handler struct {
	svc          serviceAPI
	enrollLookup rekapEnrollmentLookup
	userLookup   rekapUserLookup
}

type serviceAPI interface {
	SiswaKelasNilai(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) (*SiswaKelasNilaiResponse, error)
	SiswaList(ctx context.Context, siswaID uuid.UUID, callerRole string) (*SiswaListResponse, error)
	GuruKelasRekap(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, enrollLookup rekapEnrollmentLookup, userLookup rekapUserLookup) (*GuruRekapResponse, error)
}

// NewHandler wires the service into a Fiber handler. enrollLookup +
// userLookup are forwarded by GuruKelasRekap (so the handler can stay a
// thin pass-through and tests can inject mocks).
func NewHandler(svc *Service, enrollLookup rekapEnrollmentLookup, userLookup rekapUserLookup) *Handler {
	return &Handler{svc: svc, enrollLookup: enrollLookup, userLookup: userLookup}
}

// SiswaKelasNilai handles GET /api/v1/siswa/kelas/:id/nilai.
func (h *Handler) SiswaKelasNilai(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.SiswaKelasNilai(c.UserContext(), kelasID, siswaID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// SiswaList handles GET /api/v1/siswa/nilai (cross-kelas).
func (h *Handler) SiswaList(c *fiber.Ctx) error {
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.SiswaList(c.UserContext(), siswaID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// GuruKelasRekap handles GET /api/v1/kelas/:id/rekap?format=json|csv.
//
// Auth boundary: kelasGroup mounts this with admin/guru role guard. Service
// re-checks (defensive) and verifies kelas ownership for guru caller.
func (h *Handler) GuruKelasRekap(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	if role != string(auth.Admin) && role != string(auth.Guru) {
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	}

	res, err := h.svc.GuruKelasRekap(c.UserContext(), kelasID, callerID, role, h.enrollLookup, h.userLookup)
	if err != nil {
		return mapErr(c, err)
	}

	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format == "csv" {
		var buf bytes.Buffer
		if encErr := EncodeRekapCSV(&buf, res); encErr != nil {
			return errResp(c, fiber.StatusInternalServerError, "encode csv", "internal")
		}
		c.Set("Content-Type", "text/csv; charset=utf-8")
		c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="rekap-%s.csv"`, sanitizeCSVHeaderID(res.Kelas.ID.String())))
		return c.Status(fiber.StatusOK).Send(buf.Bytes())
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// mapErr translates Service sentinel errors to HTTP responses.
func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrNotFound):
		return errResp(c, fiber.StatusNotFound, "not found", "not_found")
	default:
		return errResp(c, fiber.StatusInternalServerError, "internal", "internal")
	}
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
