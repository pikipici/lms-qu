// HTTP handlers untuk tugas.
//
// Endpoints (Task 4.A.2):
//   - POST   /kelas/:id/tugas      (guru/admin)
//   - GET    /kelas/:id/tugas      (guru/admin/siswa enrolled)
//   - GET    /tugas/:id            (guru/admin/siswa enrolled)
//   - PATCH  /tugas/:id            (guru pemilik / admin)
//   - DELETE /tugas/:id            (guru pemilik / admin)
package tugas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
)

// tugasService is the subset of *Service the handler depends on.
type tugasService interface {
	Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Tugas, error)
	ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Tugas, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Tugas, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Tugas, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Tugas, []string, error)
	Duplicate(ctx context.Context, srcID, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Tugas, error)
	UploadAttachment(ctx context.Context, tugasID, callerID uuid.UUID, callerRole string, in UploadAttachmentInput, ip, userAgent string) (*Attachment, error)
	DeleteAttachment(ctx context.Context, tugasID, attachmentID, callerID uuid.UUID, callerRole, ip, userAgent string) error
	PresignAttachmentURL(ctx context.Context, tugasID, attachmentID, callerID uuid.UUID, callerRole, ip, userAgent string) (*AttachmentURLResult, error)
	ListAttachments(ctx context.Context, tugasID, callerID uuid.UUID, callerRole string) ([]Attachment, error)
}

// Handler wires HTTP routes to tugas Service.
type Handler struct {
	svc tugasService
}

// NewHandler returns a tugas HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse is the shape returned by GET /kelas/:id/tugas.
type ListResponse struct {
	Items []Tugas `json:"items"`
	Total int     `json:"total"`
}

// DefaultListLimit caps the result set when caller doesn't specify limit.
const DefaultListLimit = 50

// MaxListLimit clamps the upper bound to prevent abusive page sizes.
const MaxListLimit = 200

type createRequest struct {
	BabID           *uuid.UUID `json:"bab_id"`
	Judul           string     `json:"judul"`
	Deskripsi       string     `json:"deskripsi"`
	Deadline        *time.Time `json:"deadline"`
	IzinkanLate     bool       `json:"izinkan_late"`
	PenaltyPersen   int16      `json:"penalty_persen"`
	WajibAttachment bool       `json:"wajib_attachment"`
	Bobot           *int       `json:"bobot"`
	Status          *Status    `json:"status"`
}

// updateRequest uses json.RawMessage for fields where we need to distinguish
// "absent" vs "explicit null" (bab_id, deadline). String/bool/int fields use
// pointer for "absent → nil; present → non-nil".
type updateRequest struct {
	Version         int             `json:"version"`
	Judul           *string         `json:"judul"`
	Deskripsi       *string         `json:"deskripsi"`
	BabID           json.RawMessage `json:"bab_id"`
	Deadline        json.RawMessage `json:"deadline"`
	IzinkanLate     *bool           `json:"izinkan_late"`
	PenaltyPersen   *int16          `json:"penalty_persen"`
	WajibAttachment *bool           `json:"wajib_attachment"`
	Bobot           *int            `json:"bobot"`
	Status          *Status         `json:"status"`
}

// Create handles POST /api/v1/kelas/:id/tugas.
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
		BabID:           req.BabID,
		Judul:           req.Judul,
		Deskripsi:       req.Deskripsi,
		Deadline:        req.Deadline,
		IzinkanLate:     req.IzinkanLate,
		PenaltyPersen:   req.PenaltyPersen,
		WajibAttachment: req.WajibAttachment,
		Bobot:           req.Bobot,
		Status:          req.Status,
	}
	t, err := h.svc.Create(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"tugas": t})
}

// ListByKelas handles GET /api/v1/kelas/:id/tugas.
//
// Query params:
//   - bab_id=<uuid> | bab_id=null  → narrow by bab; absent = no filter
//   - status=draft|published|archived → guru/admin only; siswa always pinned
//   - limit=<int>                  → page cap (default 50, max 200)
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
			return errResp(c, fiber.StatusBadRequest, "status must be draft|published|archived", "invalid_status")
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

// Get handles GET /api/v1/tugas/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	t, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"tugas": t})
}

// Update handles PATCH /api/v1/tugas/:id.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
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
		Deskripsi:       req.Deskripsi,
		IzinkanLate:     req.IzinkanLate,
		PenaltyPersen:   req.PenaltyPersen,
		WajibAttachment: req.WajibAttachment,
		Bobot:           req.Bobot,
		Status:          req.Status,
	}
	if len(req.BabID) > 0 {
		in.BabIDExplicit = true
		if string(req.BabID) != "null" {
			var bid uuid.UUID
			if err := json.Unmarshal(req.BabID, &bid); err != nil {
				return errResp(c, fiber.StatusBadRequest, "invalid bab_id", "invalid_id")
			}
			in.BabID = &bid
		}
	}
	if len(req.Deadline) > 0 {
		in.DeadlineExplicit = true
		if string(req.Deadline) != "null" {
			var d time.Time
			if err := json.Unmarshal(req.Deadline, &d); err != nil {
				return errResp(c, fiber.StatusBadRequest, "invalid deadline", "invalid_body")
			}
			in.Deadline = &d
		}
	}

	t, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"tugas": t})
}

// Delete handles DELETE /api/v1/tugas/:id.
//
// Returns the orphan attachment ObjectKeys so caller / future task 4.A.3
// can run R2 DeleteObject compensating cleanup. Untuk MVP Task 4.A.2, R2
// cleanup belum di-wire (no attachments yet), tapi shape sudah ready.
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	t, _, err := h.svc.Delete(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"tugas_id": t.ID,
	})
}

type duplicateRequest struct {
	Judul string `json:"judul"`
}

// Duplicate handles POST /api/v1/tugas/:id/duplicate.
//
// Body: { judul?: string } — optional override; default appends " (Salinan)"
// to the source tugas judul. Response 201 + { tugas: <new_tugas> }.
//
// Mirror pola bab.Duplicate (Task 3.A.4): copy fields ke status=draft baru
// + R2 CopyObject untuk attachment + audit `tugas_duplicated`.
func (h *Handler) Duplicate(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req duplicateRequest
	// Body optional — empty body is fine (auto-suffix).
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
		}
	}

	t, err := h.svc.Duplicate(c.UserContext(), id, callerID, role, DuplicateInput{Judul: req.Judul}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"tugas": t})
}

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrDeskripsiTooLong):
		return errResp(c, fiber.StatusRequestEntityTooLarge, friendlyMessage(err, "deskripsi too long"), "payload_too_large")
	case errors.Is(err, ErrBabNotInKelas):
		return errResp(c, fiber.StatusBadRequest, "bab does not belong to this kelas", "bab_not_in_kelas")
	case errors.Is(err, ErrAttachmentUnsupportedMime):
		return errResp(c, fiber.StatusUnsupportedMediaType, friendlyMessage(err, "attachment mime not allowed"), "unsupported_mime")
	case errors.Is(err, ErrAttachmentTooLarge):
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("attachment melebihi batas %d MB", MaxTugasAttachmentBytes/(1024*1024)),
			"payload_too_large")
	case errors.Is(err, ErrAttachmentLimitReached):
		return errResp(c, fiber.StatusBadRequest,
			fmt.Sprintf("max %d attachment per tugas", MaxAttachmentsPerTugas),
			"attachment_limit_reached")
	case errors.Is(err, ErrAttachmentUploadFailed):
		slog.Error("tugas attachment r2", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "upload to object store failed", "r2_put_failed")
	case errors.Is(err, ErrR2Required):
		return errResp(c, fiber.StatusServiceUnavailable, "object store not configured", "r2_unavailable")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "tugas not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrKelasArchived):
		return errResp(c, fiber.StatusConflict, "kelas is archived; tugas cannot be created", "kelas_archived")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "tugas has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("tugas handler", slog.String("err", err.Error()))
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
