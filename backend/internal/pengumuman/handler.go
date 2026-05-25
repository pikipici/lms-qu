// HTTP handlers untuk pengumuman.
//
// Endpoints:
//   - POST   /kelas/:id/pengumuman          (guru/admin)
//   - GET    /kelas/:id/pengumuman          (guru/admin/siswa enrolled)
//   - GET    /pengumuman/:id                (guru/admin/siswa enrolled)
//   - PATCH  /pengumuman/:id                (guru pemilik / admin)
//   - DELETE /pengumuman/:id                (guru pemilik / admin)
package pengumuman

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
)

// pengumumanService is the subset of *Service the handler depends on.
type pengumumanService interface {
	Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Pengumuman, error)
	ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Pengumuman, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Pengumuman, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Pengumuman, error)
	UploadAttachment(ctx context.Context, id, callerID uuid.UUID, callerRole string, in AttachmentUploadInput, ip, userAgent string) (*Pengumuman, error)
	DeleteAttachment(ctx context.Context, id, attachmentID, callerID uuid.UUID, callerRole string, ip, userAgent string) (*Pengumuman, error)
	PresignAttachmentURL(ctx context.Context, id, attachmentID, callerID uuid.UUID, callerRole string) (*AttachmentURLResult, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Pengumuman, error)
}

// Handler wires HTTP routes to pengumuman Service.
type Handler struct {
	svc pengumumanService
}

// NewHandler returns a pengumuman HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse is the shape returned by GET /kelas/:id/pengumuman.
type ListResponse struct {
	Items []Pengumuman `json:"items"`
	Total int          `json:"total"`
}

// DefaultListLimit caps the result set when caller doesn't specify limit.
const DefaultListLimit = 50

// MaxListLimit clamps the upper bound to prevent abusive page sizes.
const MaxListLimit = 200

type createRequest struct {
	BabID *uuid.UUID `json:"bab_id"`
	Judul string     `json:"judul"`
	Isi   string     `json:"isi"`
}

type updateRequest struct {
	Version int     `json:"version"`
	Judul   *string `json:"judul"`
	Isi     *string `json:"isi"`
	Status  *Status `json:"status"`
}

// Create handles POST /api/v1/kelas/:id/pengumuman.
func (h *Handler) Create(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
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
		BabID: req.BabID,
		Judul: req.Judul,
		Isi:   req.Isi,
	}
	p, err := h.svc.Create(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"pengumuman": p})
}

// ListByKelas handles GET /api/v1/kelas/:id/pengumuman.
//
// Query params:
//   - bab_id=<uuid>   → pin bab_id = uuid
//   - bab_id=null     → pin bab_id IS NULL (kelas-wide)
//   - bab_id absent   → no filter
//   - status=...      → guru/admin only; siswa always pinned to 'published'
//   - limit=<int>     → page cap (default 50, max 200)
func (h *Handler) ListByKelas(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{Limit: DefaultListLimit}

	rawBab := strings.TrimSpace(c.Query("bab_id"))
	if rawBab != "" {
		switch strings.ToLower(rawBab) {
		case "null", "none":
			zero := uuid.Nil
			in.BabID = &zero
		default:
			parsed, perr := uuid.Parse(rawBab)
			if perr != nil {
				return errResp(c, fiber.StatusBadRequest, "invalid bab_id query", "invalid_id")
			}
			in.BabID = &parsed
		}
	}

	rawStatus := strings.TrimSpace(c.Query("status"))
	if rawStatus != "" {
		st := Status(rawStatus)
		if !st.Valid() {
			return errResp(c, fiber.StatusBadRequest, "status must be published|archived", "invalid_status")
		}
		in.Status = &st
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

	rows, err := h.svc.ListByKelas(c.UserContext(), kelasID, callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{Items: rows, Total: len(rows)})
}

// Get handles GET /api/v1/pengumuman/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	p, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"pengumuman": p})
}

// Update handles PATCH /api/v1/pengumuman/:id.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
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
		Judul:           req.Judul,
		Isi:             req.Isi,
		Status:          req.Status,
	}
	p, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"pengumuman": p})
}

// UploadAttachment handles PUT /api/v1/pengumuman/:id/attachment.
func (h *Handler) UploadAttachment(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	fh, err := c.FormFile("file")
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "file is required", "file_required")
	}
	if fh.Size > MaxAttachmentBytes {
		return errResp(c, fiber.StatusRequestEntityTooLarge, "attachment too large", "payload_too_large")
	}
	f, err := fh.Open()
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "cannot read file", "invalid_file")
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, MaxAttachmentBytes+1))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "cannot read file", "invalid_file")
	}
	p, err := h.svc.UploadAttachment(c.UserContext(), id, callerID, role, AttachmentUploadInput{Filename: fh.Filename, Body: body}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"pengumuman": p})
}

// DeleteAttachment handles DELETE /api/v1/pengumuman/:id/attachments/:attachmentID.
func (h *Handler) DeleteAttachment(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
	}
	attachmentID, err := uuid.Parse(strings.TrimSpace(c.Params("attachmentID")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid attachment id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	p, err := h.svc.DeleteAttachment(c.UserContext(), id, attachmentID, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"pengumuman": p})
}

// AttachmentURL handles GET /api/v1/pengumuman/:id/attachments/:attachmentID/url.
func (h *Handler) AttachmentURL(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
	}
	attachmentID, err := uuid.Parse(strings.TrimSpace(c.Params("attachmentID")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid attachment id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	res, err := h.svc.PresignAttachmentURL(c.UserContext(), id, attachmentID, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"url": res.URL, "expires_at": res.ExpiresAt})
}

// Delete handles DELETE /api/v1/pengumuman/:id.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid pengumuman id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	p, err := h.svc.Delete(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"pengumuman_id": p.ID,
	})
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrIsiTooLong), errors.Is(err, ErrAttachmentTooLarge):
		return errResp(c, fiber.StatusRequestEntityTooLarge, friendlyMessage(err, "payload too large"), "payload_too_large")
	case errors.Is(err, ErrAttachmentUnsupported):
		return errResp(c, fiber.StatusBadRequest, "attachment must be an image or PDF", "unsupported_attachment")
	case errors.Is(err, ErrAttachmentMissing):
		return errResp(c, fiber.StatusNotFound, "attachment not found", "attachment_not_found")
	case errors.Is(err, ErrAttachmentStorageNeeded):
		return errResp(c, fiber.StatusServiceUnavailable, "attachment storage unavailable", "storage_unavailable")
	case errors.Is(err, ErrBabNotInKelas):
		return errResp(c, fiber.StatusBadRequest, "bab does not belong to this kelas", "bab_not_in_kelas")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "pengumuman not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrKelasArchived):
		return errResp(c, fiber.StatusConflict, "kelas is archived; pengumuman cannot be created", "kelas_archived")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "pengumuman has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("pengumuman handler", slog.String("err", err.Error()))
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
