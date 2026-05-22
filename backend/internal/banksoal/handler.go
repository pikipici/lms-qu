// HTTP handlers untuk banksoal.
//
// Endpoints (Task 6.B.1 — CRUD per-guru):
//   - POST   /api/v1/bank-soal      (guru/admin)
//   - GET    /api/v1/bank-soal      (guru/admin; siswa BLOCKED)
//   - GET    /api/v1/bank-soal/:id  (guru/admin owner-only)
//   - PATCH  /api/v1/bank-soal/:id  (guru/admin owner-only)
//   - DELETE /api/v1/bank-soal/:id  (guru/admin owner-only, soft delete)
//
// Image upload + bulk paste endpoints land in Task 6.B.2 + 6.B.3.
package banksoal

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

// banksoalService is the subset of *Service the handler depends on.
type banksoalService interface {
	Create(ctx context.Context, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*BankSoal, error)
	List(ctx context.Context, callerID uuid.UUID, callerRole string, in ListInput) (*ListResult, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*BankSoal, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*BankSoal, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole string, expectedVersion int, ip, userAgent string) (*BankSoal, error)
	UploadImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, in UploadImageInput, ip, userAgent string) (*ImageUploadResult, error)
	DeleteImage(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*BankSoal, error)
	PresignImageURL(ctx context.Context, soalID, callerID uuid.UUID, callerRole string, slot ImageSlot, ip, userAgent string) (*BankSoalImageURLResult, error)
	BulkCreate(ctx context.Context, callerID uuid.UUID, callerRole string, in BulkCreateInput, ip, userAgent string) (*BulkCreateResult, error)
}

// Handler wires HTTP routes to banksoal Service.
type Handler struct {
	svc   banksoalService
	store storage.Storage // optional — used by 6.B.2 image endpoints
}

// NewHandler returns a banksoal HTTP handler.
func NewHandler(svc *Service, store storage.Storage) *Handler {
	return &Handler{svc: svc, store: store}
}

// ListResponse is the shape returned by GET /api/v1/bank-soal.
type ListResponse struct {
	Items  []BankSoal `json:"items"`
	Total  int64      `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
}

// DefaultListLimit caps the result set when caller doesn't specify limit.
const DefaultListLimit = 50

// MaxListLimit clamps the upper bound to prevent abusive page sizes.
const MaxListLimit = 200

type createRequest struct {
	Mapel      string  `json:"mapel"`
	Tingkat    string  `json:"tingkat"`
	Topik      string  `json:"topik"`
	Pertanyaan string  `json:"pertanyaan"`
	OpsiA      string  `json:"opsi_a"`
	OpsiB      string  `json:"opsi_b"`
	OpsiC      string  `json:"opsi_c"`
	OpsiD      string  `json:"opsi_d"`
	OpsiE      string  `json:"opsi_e"`
	Jawaban    Jawaban `json:"jawaban"`
	Poin       int16   `json:"poin"`
}

type updateRequest struct {
	Version    int      `json:"version"`
	Mapel      *string  `json:"mapel"`
	Tingkat    *string  `json:"tingkat"`
	Topik      *string  `json:"topik"`
	Pertanyaan *string  `json:"pertanyaan"`
	OpsiA      *string  `json:"opsi_a"`
	OpsiB      *string  `json:"opsi_b"`
	OpsiC      *string  `json:"opsi_c"`
	OpsiD      *string  `json:"opsi_d"`
	OpsiE      *string  `json:"opsi_e"`
	Jawaban    *Jawaban `json:"jawaban"`
	Poin       *int16   `json:"poin"`
}

type deleteRequest struct {
	Version int `json:"version"`
}

// Create handles POST /api/v1/bank-soal.
func (h *Handler) Create(c *fiber.Ctx) error {
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
		Mapel:      req.Mapel,
		Tingkat:    req.Tingkat,
		Topik:      req.Topik,
		Pertanyaan: req.Pertanyaan,
		OpsiA:      req.OpsiA, OpsiB: req.OpsiB,
		OpsiC: req.OpsiC, OpsiD: req.OpsiD, OpsiE: req.OpsiE,
		Jawaban: req.Jawaban,
		Poin:    req.Poin,
	}
	soal, err := h.svc.Create(c.UserContext(), callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"soal": soal})
}

// List handles GET /api/v1/bank-soal.
//
// Query params:
//   - mapel=<exact>         → narrow by mapel
//   - tingkat=<exact>       → narrow by tingkat
//   - topik=<substr>        → ILIKE substring match
//   - limit=<int>           → page cap (default 50, max 200)
//   - offset=<int>          → page offset (default 0)
func (h *Handler) List(c *fiber.Ctx) error {
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{
		Mapel:   strings.TrimSpace(c.Query("mapel")),
		Tingkat: strings.TrimSpace(c.Query("tingkat")),
		Topik:   strings.TrimSpace(c.Query("topik")),
		Limit:   DefaultListLimit,
		Offset:  0,
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
	if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
		n, perr := strconv.Atoi(rawOffset)
		if perr != nil || n < 0 {
			return errResp(c, fiber.StatusBadRequest, "offset must be a non-negative integer", "invalid_offset")
		}
		in.Offset = n
	}

	res, err := h.svc.List(c.UserContext(), callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{
		Items:  res.Items,
		Total:  res.Total,
		Limit:  res.Limit,
		Offset: res.Offset,
	})
}

// Get handles GET /api/v1/bank-soal/:id.
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

// Update handles PATCH /api/v1/bank-soal/:id.
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
		Mapel:           req.Mapel,
		Tingkat:         req.Tingkat,
		Topik:           req.Topik,
		Pertanyaan:      req.Pertanyaan,
		OpsiA:           req.OpsiA, OpsiB: req.OpsiB,
		OpsiC: req.OpsiC, OpsiD: req.OpsiD, OpsiE: req.OpsiE,
		Jawaban: req.Jawaban,
		Poin:    req.Poin,
	}

	soal, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"soal": soal})
}

// Delete handles DELETE /api/v1/bank-soal/:id.
//
// Soft delete (locked #84): row di-mark deleted_at supaya HasilUjian
// referensi tetap valid. Body: {"version": <int>} untuk optimistic
// concurrency. Image keys NOT deleted from R2 di soft delete — purge
// terjadi di hard-delete cron Fase 8.
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

	// Version dari body OR query param ?version= (DELETE body optional di clients).
	expectedVersion := 0
	var req deleteRequest
	if err := c.BodyParser(&req); err == nil {
		expectedVersion = req.Version
	}
	if expectedVersion <= 0 {
		if rawV := strings.TrimSpace(c.Query("version")); rawV != "" {
			if n, perr := strconv.Atoi(rawV); perr == nil {
				expectedVersion = n
			}
		}
	}
	if expectedVersion <= 0 {
		return errResp(c, fiber.StatusBadRequest, "version must be positive (body or ?version=)", "invalid_version")
	}

	soal, err := h.svc.Delete(c.UserContext(), id, callerID, role, expectedVersion, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"soal_id": soal.ID,
		"deleted": true,
	})
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrJawabanInvalid):
		return errResp(c, fiber.StatusBadRequest, "jawaban points to an empty option", "jawaban_invalid")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "bank soal not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "soal has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("banksoal handler", slog.String("err", err.Error()))
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
