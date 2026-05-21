// HTTP handlers untuk soalbab.
//
// Endpoints (Task 5.B.1 — CRUD):
//   - POST   /api/v1/bab/:id/soal      (guru/admin owner)
//   - GET    /api/v1/bab/:id/soal      (guru/admin owner; siswa BLOCKED)
//   - GET    /api/v1/soal-bab/:id      (guru/admin owner; siswa BLOCKED)
//   - PATCH  /api/v1/soal-bab/:id      (guru/admin owner)
//   - DELETE /api/v1/soal-bab/:id      (guru/admin owner)
//
// Image upload + bulk paste endpoints land in Task 5.B.2 + 5.B.3.
package soalbab

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/storage"
)

// soalbabService is the subset of *Service the handler depends on.
type soalbabService interface {
	Create(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*SoalBab, error)
	ListByBab(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in ListInput) ([]SoalBab, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*SoalBab, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*SoalBab, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*SoalBab, []string, error)
	UploadImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, in UploadImageInput, ip, userAgent string) (*ImageUploadResult, error)
	DeleteImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*SoalBab, error)
	PresignImageURL(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*SoalImageURLResult, error)
}

// Handler wires HTTP routes to soalbab Service.
type Handler struct {
	svc   soalbabService
	store storage.Storage // optional — used post-Delete to async-clean R2 image keys
}

// NewHandler returns a soalbab HTTP handler. Pass `store` non-nil to enable
// best-effort R2 cleanup on Delete; leave nil di test untuk skip R2.
func NewHandler(svc *Service, store storage.Storage) *Handler {
	return &Handler{svc: svc, store: store}
}

// ListResponse is the shape returned by GET /api/v1/bab/:id/soal.
type ListResponse struct {
	Items []SoalBab `json:"items"`
	Total int       `json:"total"`
}

// DefaultListLimit caps the result set when caller doesn't specify limit.
const DefaultListLimit = 100

// MaxListLimit clamps the upper bound to prevent abusive page sizes.
const MaxListLimit = 500

type createRequest struct {
	Pertanyaan string  `json:"pertanyaan"`
	OpsiA      string  `json:"opsi_a"`
	OpsiB      string  `json:"opsi_b"`
	OpsiC      string  `json:"opsi_c"`
	OpsiD      string  `json:"opsi_d"`
	OpsiE      string  `json:"opsi_e"`
	Jawaban    Jawaban `json:"jawaban"`
	Poin       int16   `json:"poin"`
	Mode       Mode    `json:"mode"`
	Urutan     int     `json:"urutan"`
}

type updateRequest struct {
	Version    int      `json:"version"`
	Pertanyaan *string  `json:"pertanyaan"`
	OpsiA      *string  `json:"opsi_a"`
	OpsiB      *string  `json:"opsi_b"`
	OpsiC      *string  `json:"opsi_c"`
	OpsiD      *string  `json:"opsi_d"`
	OpsiE      *string  `json:"opsi_e"`
	Jawaban    *Jawaban `json:"jawaban"`
	Poin       *int16   `json:"poin"`
	Mode       *Mode    `json:"mode"`
	Urutan     *int     `json:"urutan"`
}

// Create handles POST /api/v1/bab/:id/soal.
func (h *Handler) Create(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := CreateInput{
		Pertanyaan: req.Pertanyaan,
		OpsiA:      req.OpsiA, OpsiB: req.OpsiB,
		OpsiC: req.OpsiC, OpsiD: req.OpsiD, OpsiE: req.OpsiE,
		Jawaban: req.Jawaban,
		Poin:    req.Poin,
		Mode:    req.Mode,
		Urutan:  req.Urutan,
	}
	soal, err := h.svc.Create(c.UserContext(), babID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"soal": soal})
}

// ListByBab handles GET /api/v1/bab/:id/soal.
//
// Query params:
//   - mode=latihan|ulangan|keduanya  → narrow by mode (default = no narrow)
//   - limit=<int>                    → page cap (default 100, max 500)
func (h *Handler) ListByBab(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{Limit: DefaultListLimit}
	if rawMode := strings.TrimSpace(c.Query("mode")); rawMode != "" {
		m := Mode(rawMode)
		if !m.Valid() {
			return errResp(c, fiber.StatusBadRequest, "mode must be latihan|ulangan|keduanya", "invalid_mode")
		}
		in.Mode = m
	}
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		n, perr := strconv.Atoi(rawLimit)
		if perr != nil || n <= 0 {
			return errResp(c, fiber.StatusBadRequest, "limit must be a positive integer", "invalid_limit")
		}
		if n > MaxListLimit {
			n = MaxListLimit
		}
		in.Limit = n
	}

	rows, err := h.svc.ListByBab(c.UserContext(), babID, callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{Items: rows, Total: len(rows)})
}

// Get handles GET /api/v1/soal-bab/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	soal, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"soal": soal})
}

// Update handles PATCH /api/v1/soal-bab/:id.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req.Version <= 0 {
		return errResp(c, fiber.StatusBadRequest, "version must be positive", "invalid_version")
	}

	in := UpdateInput{
		ExpectedVersion: req.Version,
		Pertanyaan:      req.Pertanyaan,
		OpsiA:           req.OpsiA, OpsiB: req.OpsiB,
		OpsiC: req.OpsiC, OpsiD: req.OpsiD, OpsiE: req.OpsiE,
		Jawaban: req.Jawaban,
		Poin:    req.Poin,
		Mode:    req.Mode,
		Urutan:  req.Urutan,
	}

	soal, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"soal": soal})
}

// Delete handles DELETE /api/v1/soal-bab/:id.
//
// Returns the orphan ObjectKeys so caller / async cleanup can run R2
// DeleteObject (locked #69 pattern). When `h.store` is configured, the
// handler dispatches a best-effort cleanup synchronously after the DB
// commit — failures are logged but do not fail the response.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	soal, keys, err := h.svc.Delete(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}

	// Best-effort R2 compensating cleanup. Failure logged but non-fatal so
	// the DB delete remains the source of truth (locked #69).
	if h.store != nil && len(keys) > 0 {
		ctx := c.UserContext()
		for _, k := range keys {
			if delErr := h.store.DeleteObject(ctx, k); delErr != nil {
				slog.Warn("soalbab r2 cleanup",
					slog.String("soal_id", id.String()),
					slog.String("object_key", k),
					slog.String("err", delErr.Error()),
				)
			}
		}
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"soal_id":          soal.ID,
		"image_key_count":  len(keys),
	})
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrJawabanInvalid):
		return errResp(c, fiber.StatusBadRequest, "jawaban points to an empty option", "jawaban_invalid")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "soal not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrBabArchived):
		return errResp(c, fiber.StatusConflict, "bab is archived; soal cannot be modified", "bab_archived")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "soal has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("soalbab handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func friendlyMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	msg := err.Error()
	const sep = ": "
	if idx := strings.Index(msg, sep); idx >= 0 {
		rest := msg[idx+len(sep):]
		if idx2 := strings.Index(rest, sep); idx2 >= 0 {
			return rest[idx2+len(sep):]
		}
		return rest
	}
	return fallback
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
