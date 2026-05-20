// HTTP handlers untuk bab (guru/admin scope CRUD + archive).
package bab

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

// babService is the subset of *Service the handler depends on. Allows the
// handler to be unit-tested with a stub.
type babService interface {
	Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Bab, error)
	ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Bab, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Bab, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Bab, error)
	Archive(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Bab, error)
	Reorder(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ReorderInput, ip, userAgent string) ([]Bab, error)
}

// Handler wires HTTP routes to bab Service.
type Handler struct {
	svc babService
}

// NewHandler returns a bab HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse is the shape returned by GET /kelas/:id/bab.
type ListResponse struct {
	Items []Bab `json:"items"`
	Total int   `json:"total"`
}

type createRequest struct {
	Nomor     int    `json:"nomor"`
	Judul     string `json:"judul"`
	Deskripsi string `json:"deskripsi"`
}

type updateRequest struct {
	Version   int     `json:"version"`
	Nomor     *int    `json:"nomor"`
	Judul     *string `json:"judul"`
	Deskripsi *string `json:"deskripsi"`
	Urutan    *int    `json:"urutan"`
	Status    *Status `json:"status"`
}

// ListByKelas handles GET /api/v1/kelas/:id/bab.
//
// Query params:
//   - status=draft|published|archived (pin to a single status)
//   - include_archived=true (override default; archived hidden by default)
func (h *Handler) ListByKelas(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{
		IncludeArchived: strings.EqualFold(strings.TrimSpace(c.Query("include_archived")), "true"),
	}
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		s := Status(raw)
		if !s.Valid() {
			return babError(c, fiber.StatusBadRequest, "invalid status filter", "invalid_status")
		}
		in.Status = &s
	}

	rows, err := h.svc.ListByKelas(c.UserContext(), kelasID, callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{Items: rows, Total: len(rows)})
}

// Create handles POST /api/v1/kelas/:id/bab.
func (h *Handler) Create(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := CreateInput{
		Nomor:     req.Nomor,
		Judul:     req.Judul,
		Deskripsi: req.Deskripsi,
	}
	b, err := h.svc.Create(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"bab": b})
}

// Get handles GET /api/v1/bab/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	b, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"bab": b})
}

// Update handles PATCH /api/v1/bab/:id.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req.Version <= 0 {
		return babError(c, fiber.StatusBadRequest, "version must be positive", "invalid_version")
	}

	in := UpdateInput{
		ExpectedVersion: req.Version,
		Nomor:           req.Nomor,
		Judul:           req.Judul,
		Deskripsi:       req.Deskripsi,
		Urutan:          req.Urutan,
		Status:          req.Status,
	}
	b, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"bab": b})
}

// Archive handles POST /api/v1/bab/:id/archive.
func (h *Handler) Archive(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	b, err := h.svc.Archive(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"bab": b})
}

type reorderRequest struct {
	Order    []string       `json:"order"`
	Versions map[string]int `json:"versions"`
}

// Reorder handles POST /api/v1/kelas/:id/bab/reorder.
//
// Body: { order: [bab_id, ...], versions: { bab_id: version, ... } }
// Response on conflict: 409 + { error, code: "version_conflict",
// conflicts: [{bab_id, current_version}], request_id }
func (h *Handler) Reorder(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req reorderRequest
	if err := c.BodyParser(&req); err != nil {
		return babError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if len(req.Order) == 0 {
		return babError(c, fiber.StatusBadRequest, "order list is required", "invalid_body")
	}

	order := make([]uuid.UUID, len(req.Order))
	for i, raw := range req.Order {
		babID, perr := uuid.Parse(raw)
		if perr != nil {
			return babError(c, fiber.StatusBadRequest, "invalid bab id in order", "invalid_id")
		}
		order[i] = babID
	}

	versions := make(map[uuid.UUID]int, len(req.Versions))
	for k, v := range req.Versions {
		babID, perr := uuid.Parse(k)
		if perr != nil {
			return babError(c, fiber.StatusBadRequest, "invalid bab id in versions", "invalid_id")
		}
		versions[babID] = v
	}

	rows, err := h.svc.Reorder(c.UserContext(), kelasID, callerID, role, ReorderInput{Order: order, Versions: versions}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapReorderErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"items": rows, "total": len(rows)})
}

// mapReorderErr handles the bulk-reorder-specific 409 body shape. Other
// errors fall through to the regular mapServiceErr.
func mapReorderErr(c *fiber.Ctx, err error) error {
	var conflictErr *ReorderConflictErr
	if errors.As(err, &conflictErr) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error":      "bab has been modified by another request; please refresh",
			"code":       "version_conflict",
			"conflicts":  conflictErr.Conflicts,
			"request_id": middleware.RequestIDFromFiber(c),
		})
	}
	switch {
	case errors.Is(err, ErrReorderEmpty):
		return babError(c, fiber.StatusBadRequest, "order list is empty", "invalid_body")
	case errors.Is(err, ErrReorderDuplicate):
		return babError(c, fiber.StatusBadRequest, "duplicate bab id in order", "duplicate_in_order")
	case errors.Is(err, ErrReorderForeignBab):
		return babError(c, fiber.StatusBadRequest, "bab does not belong to this kelas", "bab_not_in_kelas")
	case errors.Is(err, ErrReorderMissing):
		return babError(c, fiber.StatusBadRequest, "order must include every bab in the kelas", "reorder_missing_bab")
	}
	return mapServiceErr(c, err)
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return babError(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrInvalidStatus):
		return babError(c, fiber.StatusBadRequest, "invalid status value", "invalid_status")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return babError(c, fiber.StatusNotFound, "bab not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return babError(c, fiber.StatusForbidden, "you do not own this kelas", "forbidden")
	case errors.Is(err, ErrAlreadyArchived):
		return babError(c, fiber.StatusConflict, "bab is already archived", "already_archived")
	case errors.Is(err, ErrKelasArchived):
		return babError(c, fiber.StatusConflict, "kelas is archived; bab cannot be created", "kelas_archived")
	case errors.Is(err, ErrVersionConflict):
		return babError(c, fiber.StatusConflict, "bab has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("bab handler", slog.String("err", err.Error()))
		return babError(c, fiber.StatusInternalServerError, "internal server error", "internal")
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

func babError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
