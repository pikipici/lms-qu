// HTTP handlers untuk materi (guru/admin scope CRUD — youtube + markdown).
//
// PDF upload + presigned download di Task 3.C.3, MarkRead siswa di Task 3.C.4.
package materi

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

// materiService is the subset of *Service the handler depends on. Allows the
// handler to be unit-tested with a stub.
type materiService interface {
	Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Materi, error)
	ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Materi, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Materi, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Materi, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Materi, *string, error)
	Upload(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in UploadInput, ip, userAgent string) (*Materi, error)
	PresignFileURL(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*FileURLResult, error)
}

// Handler wires HTTP routes to materi Service.
type Handler struct {
	svc materiService
}

// NewHandler returns a materi HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse is the shape returned by GET /kelas/:id/materi.
type ListResponse struct {
	Items []Materi `json:"items"`
	Total int      `json:"total"`
}

type createRequest struct {
	BabID  *uuid.UUID `json:"bab_id"`
	Judul  string     `json:"judul"`
	Tipe   Tipe       `json:"tipe"`
	Konten string     `json:"konten"`
}

type updateRequest struct {
	Version int     `json:"version"`
	Judul   *string `json:"judul"`
	Konten  *string `json:"konten"`
	Urutan  *int    `json:"urutan"`
}

// Create handles POST /api/v1/kelas/:id/materi.
//
// Body: { bab_id?: uuid|null, judul: string, tipe: "youtube"|"markdown",
//         konten: string }
// Tipe "pdf" rejected here — multipart endpoint diperlukan (Task 3.C.3).
func (h *Handler) Create(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	in := CreateInput{
		BabID:  req.BabID,
		Judul:  req.Judul,
		Tipe:   req.Tipe,
		Konten: req.Konten,
	}
	m, err := h.svc.Create(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"materi": m})
}

// ListByKelas handles GET /api/v1/kelas/:id/materi.
//
// Query params:
//   - bab_id=<uuid> → pin bab_id = uuid
//   - bab_id=null   → pin bab_id IS NULL (materi berdiri bebas)
//   - bab_id absent → no filter (return all in kelas)
func (h *Handler) ListByKelas(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{}
	raw := strings.TrimSpace(c.Query("bab_id"))
	if raw != "" {
		switch strings.ToLower(raw) {
		case "null", "none", "":
			zero := uuid.Nil
			in.BabID = &zero
		default:
			parsed, perr := uuid.Parse(raw)
			if perr != nil {
				return materiError(c, fiber.StatusBadRequest, "invalid bab_id query", "invalid_id")
			}
			in.BabID = &parsed
		}
	}

	rows, err := h.svc.ListByKelas(c.UserContext(), kelasID, callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{Items: rows, Total: len(rows)})
}

// Get handles GET /api/v1/materi/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid materi id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	m, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"materi": m})
}

// Update handles PATCH /api/v1/materi/:id.
//
// Body: { version: int, judul?, konten?, urutan? }. Tipe is immutable —
// server rejects with 409 tipe_immutable if attempted at the storage layer
// (handler doesn't expose tipe in updateRequest).
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid materi id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req.Version <= 0 {
		return materiError(c, fiber.StatusBadRequest, "version must be positive", "invalid_version")
	}

	in := UpdateInput{
		ExpectedVersion: req.Version,
		Judul:           req.Judul,
		Konten:          req.Konten,
		Urutan:          req.Urutan,
	}
	m, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"materi": m})
}

// Delete handles DELETE /api/v1/materi/:id.
//
// PDF tipe in Task 3.C.2 is technically delete-able from DB but R2 cleanup
// happens in Task 3.C.3 — handler returns the object_key in response so a
// caller (or a future cleanup goroutine) can verify orphan state.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid materi id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	m, objectKey, err := h.svc.Delete(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	resp := fiber.Map{"materi_id": m.ID, "tipe": m.Tipe}
	if objectKey != nil {
		resp["object_key"] = *objectKey
		resp["pending_r2_cleanup"] = true
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return materiError(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrTipeUnsupported):
		return materiError(c, fiber.StatusBadRequest, "pdf must be uploaded via multipart endpoint", "tipe_unsupported")
	case errors.Is(err, ErrInvalidTipe):
		return materiError(c, fiber.StatusBadRequest, "invalid tipe value", "invalid_tipe")
	case errors.Is(err, ErrKontenTooLong):
		return materiError(c, fiber.StatusRequestEntityTooLarge, friendlyMessage(err, "konten too long"), "payload_too_large")
	case errors.Is(err, ErrBabNotInKelas):
		return materiError(c, fiber.StatusBadRequest, "bab does not belong to this kelas", "bab_not_in_kelas")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return materiError(c, fiber.StatusNotFound, "materi not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return materiError(c, fiber.StatusForbidden, "you do not own this kelas", "forbidden")
	case errors.Is(err, ErrKelasArchived):
		return materiError(c, fiber.StatusConflict, "kelas is archived; materi cannot be created", "kelas_archived")
	case errors.Is(err, ErrTipeImmutable):
		return materiError(c, fiber.StatusConflict, "materi tipe cannot be changed; delete and recreate instead", "tipe_immutable")
	case errors.Is(err, ErrVersionConflict):
		return materiError(c, fiber.StatusConflict, "materi has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("materi handler", slog.String("err", err.Error()))
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
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

func materiError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
