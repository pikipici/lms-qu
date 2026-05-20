// HTTP handlers untuk kelas (guru-scope CRUD + duplicate + archive).
package kelas

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// kelasService is the subset of *Service the handler depends on. Allows the
// handler to be unit-tested with a stub.
type kelasService interface {
	Create(ctx context.Context, guruID uuid.UUID, in CreateInput, ip, userAgent string) (*Kelas, error)
	ListForGuru(ctx context.Context, guruID uuid.UUID, in ListInput) (*ListResult, error)
	ListAllAdmin(ctx context.Context, in ListInput) (*ListResult, error)
	Get(ctx context.Context, id, viewerID uuid.UUID, viewerRole string) (*Kelas, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Kelas, error)
	Archive(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Kelas, error)
	Duplicate(ctx context.Context, id, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Kelas, error)
}

// Handler wires HTTP routes to kelas Service.
type Handler struct {
	svc kelasService
}

// NewHandler returns a kelas HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse is the shape returned by GET /kelas list endpoints.
type ListResponse struct {
	Items      []Kelas `json:"items"`
	Page       int     `json:"page"`
	PageSize   int     `json:"page_size"`
	Total      int64   `json:"total"`
	TotalPages int     `json:"total_pages"`
}

type createRequest struct {
	Nama             string `json:"nama"`
	Deskripsi        string `json:"deskripsi"`
	BobotSoalUlangan int    `json:"bobot_soal_ulangan"`
	BobotTugas       int    `json:"bobot_tugas"`
}

type updateRequest struct {
	Version          int     `json:"version"`
	Nama             string  `json:"nama"`
	Deskripsi        *string `json:"deskripsi"`
	BobotSoalUlangan *int    `json:"bobot_soal_ulangan"`
	BobotTugas       *int    `json:"bobot_tugas"`
}

type duplicateRequest struct {
	NewNama string `json:"new_nama"`
}

// List handles GET /api/v1/kelas. Guru gets only their own; admin gets all.
func (h *Handler) List(c *fiber.Ctx) error {
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	page, pageSize := pagination(c)
	includeArchived := strings.EqualFold(strings.TrimSpace(c.Query("include_archived")), "true")

	in := ListInput{
		IncludeArchived: includeArchived,
		Limit:           pageSize,
		Offset:          (page - 1) * pageSize,
	}

	var (
		res *ListResult
		err error
	)
	switch role {
	case string(auth.Admin):
		res, err = h.svc.ListAllAdmin(c.UserContext(), in)
	case string(auth.Guru):
		guruID, gerr := middleware.UserIDFromCtx(c)
		if gerr != nil {
			return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		res, err = h.svc.ListForGuru(c.UserContext(), guruID, in)
	default:
		return kelasError(c, fiber.StatusForbidden, "insufficient role", "forbidden")
	}
	if err != nil {
		slog.Error("kelas list", slog.String("err", err.Error()))
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{
		Items:      res.Items,
		Page:       page,
		PageSize:   pageSize,
		Total:      res.Total,
		TotalPages: totalPages(res.Total, pageSize),
	})
}

// Create handles POST /api/v1/kelas. Guru-only.
func (h *Handler) Create(c *fiber.Ctx) error {
	guruID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := CreateInput{
		Nama:             req.Nama,
		Deskripsi:        req.Deskripsi,
		BobotSoalUlangan: req.BobotSoalUlangan,
		BobotTugas:       req.BobotTugas,
	}
	k, err := h.svc.Create(c.UserContext(), guruID, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"kelas": k})
}

// Get handles GET /api/v1/kelas/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	viewerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	k, err := h.svc.Get(c.UserContext(), id, viewerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"kelas": k})
}

// Update handles PATCH /api/v1/kelas/:id.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req.Version <= 0 {
		return kelasError(c, fiber.StatusBadRequest, "version must be positive", "invalid_version")
	}

	in := UpdateInput{
		ExpectedVersion:  req.Version,
		Nama:             req.Nama,
		Deskripsi:        req.Deskripsi,
		BobotSoalUlangan: req.BobotSoalUlangan,
		BobotTugas:       req.BobotTugas,
	}
	k, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"kelas": k})
}

// Archive handles POST /api/v1/kelas/:id/archive.
func (h *Handler) Archive(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	k, err := h.svc.Archive(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"kelas": k})
}

// Duplicate handles POST /api/v1/kelas/:id/duplicate.
func (h *Handler) Duplicate(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req duplicateRequest
	if body := strings.TrimSpace(string(c.Body())); body != "" {
		if err := c.BodyParser(&req); err != nil {
			return kelasError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
		}
	}

	dup, err := h.svc.Duplicate(c.UserContext(), id, callerID, role, DuplicateInput{NewNama: req.NewNama}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"kelas": dup})
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return kelasError(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrBobotInvalid):
		return kelasError(c, fiber.StatusBadRequest, "bobot soal ulangan + bobot tugas must equal 100", "invalid_bobot")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return kelasError(c, fiber.StatusNotFound, "kelas not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return kelasError(c, fiber.StatusForbidden, "you do not own this kelas", "forbidden")
	case errors.Is(err, ErrAlreadyArchived):
		return kelasError(c, fiber.StatusBadRequest, "kelas is already archived", "already_archived")
	case errors.Is(err, ErrVersionConflict):
		return kelasError(c, fiber.StatusConflict, "kelas has been modified by another request; please refresh", "version_conflict")
	case errors.Is(err, ErrKodeInviteFailed):
		return kelasError(c, fiber.StatusServiceUnavailable, "could not generate invite code; retry shortly", "kode_invite_failed")
	default:
		slog.Error("kelas handler", slog.String("err", err.Error()))
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func friendlyMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	msg := err.Error()
	// strip "kelas: invalid input: " prefix when present so the API user sees
	// just the field-level reason ("nama is required").
	const sep = ": "
	if idx := strings.Index(msg, sep); idx >= 0 {
		// "kelas: invalid input: nama is required" → after first sep "invalid input: nama is required"
		// after second → "nama is required". We pop two prefixes when present.
		rest := msg[idx+len(sep):]
		if idx2 := strings.Index(rest, sep); idx2 >= 0 {
			return rest[idx2+len(sep):]
		}
		return rest
	}
	return fallback
}

func pagination(c *fiber.Ctx) (int, int) {
	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func totalPages(total int64, pageSize int) int {
	if total <= 0 {
		return 0
	}
	return int((total + int64(pageSize) - 1) / int64(pageSize))
}

func kelasError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
