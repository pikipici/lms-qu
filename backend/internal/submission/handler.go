// HTTP handlers for the submission domain.
//
// Endpoints (Task 4.C.2 - 4.C.4):
//   POST   /api/v1/siswa/tugas/:id/submit          (siswa enrolled — multipart)
//   GET    /api/v1/siswa/tugas/:id/submission      (siswa enrolled — own + tugas info)
//   GET    /api/v1/tugas/:id/submissions           (guru/admin owner — rekap)
//   GET    /api/v1/submission/:id                  (guru/admin owner OR siswa pemilik)
//   GET    /api/v1/submission/:id/attachments/:attID/url  (presigned 15m)
//   POST   /api/v1/submission/:id/grade            (guru/admin owner)
package submission

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

// Handler bundles HTTP handlers untuk submission domain.
type Handler struct {
	svc *Service
}

// NewHandler returns a Handler bound to a Service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// errResp emits the standard error envelope (mirror tugas/pengumuman pattern).
func errResp(c *fiber.Ctx, status int, msg, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      msg,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}

// mapServiceErr translates submission service errors to HTTP status.
func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return errResp(c, fiber.StatusNotFound, err.Error(), "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, err.Error(), "forbidden")
	case errors.Is(err, ErrTugasNotPublished):
		return errResp(c, fiber.StatusNotFound, err.Error(), "not_found")
	case errors.Is(err, ErrDeadlinePassed):
		return errResp(c, fiber.StatusForbidden, err.Error(), "deadline_passed")
	case errors.Is(err, ErrAlreadyGraded):
		return errResp(c, fiber.StatusConflict, err.Error(), "already_graded")
	case errors.Is(err, ErrAttachmentRequired):
		return errResp(c, fiber.StatusBadRequest, err.Error(), "attachment_required")
	case errors.Is(err, ErrAttachmentLimit):
		return errResp(c, fiber.StatusBadRequest, err.Error(), "attachment_limit_reached")
	case errors.Is(err, ErrAttachmentTooLarge):
		return errResp(c, fiber.StatusRequestEntityTooLarge, err.Error(), "payload_too_large")
	case errors.Is(err, ErrUnsupportedMime):
		return errResp(c, fiber.StatusUnsupportedMediaType, err.Error(), "unsupported_mime")
	case errors.Is(err, ErrR2Required):
		return errResp(c, fiber.StatusServiceUnavailable, err.Error(), "r2_unavailable")
	case errors.Is(err, ErrAttachmentUploadFailed):
		return errResp(c, fiber.StatusInternalServerError, err.Error(), "r2_put_failed")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, err.Error(), "invalid_input")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "version conflict", "version_conflict")
	default:
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

// Submit handles POST /api/v1/siswa/tugas/:id/submit (multipart form).
//
// Form fields:
//   - catatan (optional): markdown body, ≤ 50KB
//   - files (multiple, optional): up to 5 × 20MB; mime allowlist locked #46
//
// Response:
//   - 201 created → {submission, is_resubmit}
//   - 400 invalid_input / attachment_required / attachment_limit_reached
//   - 403 forbidden / deadline_passed
//   - 404 not_found (tugas missing/draft/archived)
//   - 409 already_graded
//   - 413 payload_too_large
//   - 415 unsupported_mime
//   - 503 r2_unavailable
func (h *Handler) Submit(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	form, err := c.MultipartForm()
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "expected multipart/form-data", "invalid_form")
	}

	catatan := ""
	if vals := form.Value["catatan"]; len(vals) > 0 {
		catatan = strings.TrimSpace(vals[0])
	}

	fileHeaders := form.File["files"]
	if len(fileHeaders) > MaxAttachmentsPerSubmission {
		return errResp(c, fiber.StatusBadRequest,
			fmt.Sprintf("max %d attachments per submission", MaxAttachmentsPerSubmission),
			"attachment_limit_reached")
	}

	atts := make([]AttachmentInput, 0, len(fileHeaders))
	for i, fh := range fileHeaders {
		if fh.Size > MaxAttachmentBytes {
			return errResp(c, fiber.StatusRequestEntityTooLarge,
				fmt.Sprintf("attachment %d exceeds %d MB", i+1, MaxAttachmentBytes/(1024*1024)),
				"payload_too_large")
		}
		src, oerr := fh.Open()
		if oerr != nil {
			return errResp(c, fiber.StatusBadRequest, "cannot open uploaded file", "open_failed")
		}
		limited := io.LimitReader(src, MaxAttachmentBytes+1)
		body, rerr := io.ReadAll(limited)
		_ = src.Close()
		if rerr != nil {
			return errResp(c, fiber.StatusBadRequest, "read upload: "+rerr.Error(), "read_failed")
		}
		if int64(len(body)) > MaxAttachmentBytes {
			return errResp(c, fiber.StatusRequestEntityTooLarge,
				fmt.Sprintf("attachment %d exceeds %d MB", i+1, MaxAttachmentBytes/(1024*1024)),
				"payload_too_large")
		}
		atts = append(atts, AttachmentInput{
			OriginalFilename: fh.Filename,
			Body:             body,
		})
	}

	res, err := h.svc.Submit(c.UserContext(), tugasID, siswaID, SubmitInput{
		Catatan:     catatan,
		Attachments: atts,
	}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"submission":  res.Submission,
		"is_resubmit": res.IsResubmit,
	})
}

// GetMySubmission handles GET /api/v1/siswa/tugas/:id/submission.
func (h *Handler) GetMySubmission(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	res, err := h.svc.GetMySubmission(c.UserContext(), tugasID, siswaID)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// ListByTugasResponse mirrors tugas list shape.
type ListByTugasResponse struct {
	Items []Submission `json:"items"`
	Total int          `json:"total"`
}

// ListByTugas handles GET /api/v1/tugas/:id/submissions?status=.
func (h *Handler) ListByTugas(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var statusFilter *Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		st := Status(raw)
		if !st.Valid() {
			return errResp(c, fiber.StatusBadRequest, "invalid status filter", "invalid_input")
		}
		statusFilter = &st
	}

	rows, err := h.svc.ListByTugas(c.UserContext(), tugasID, callerID, role, statusFilter)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListByTugasResponse{Items: rows, Total: len(rows)})
}

// Get handles GET /api/v1/submission/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid submission id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	sub, err := h.svc.GetByID(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(sub)
}

// AttachmentURL handles GET /api/v1/submission/:id/attachments/:attID/url.
func (h *Handler) AttachmentURL(c *fiber.Ctx) error {
	subID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid submission id", "invalid_id")
	}
	attID, err := uuid.Parse(strings.TrimSpace(c.Params("attID")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid attachment id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.PresignAttachmentURL(c.UserContext(), subID, attID, callerID, role,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"url":               res.URL,
		"expires_at":        res.ExpiresAt.Format(time.RFC3339),
		"original_filename": res.OriginalFilename,
		"mime_type":         res.MimeType,
	})
}

// ListMineResponse mirrors the list shape of other endpoints.
type ListMineResponse struct {
	Items []MySubmissionItem `json:"items"`
	Total int                `json:"total"`
}

// ListMine handles GET /api/v1/siswa/submissions?limit=.
//
// Returns ALL submissions of the calling siswa lintas kelas, JOIN-ed dengan
// tugas snapshot. Used by the "Tugas Saya" dashboard page (Task 4.D.2).
func (h *Handler) ListMine(c *fiber.Ctx) error {
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		n, perr := strconv.Atoi(raw)
		if perr != nil || n <= 0 {
			return errResp(c, fiber.StatusBadRequest, "limit must be a positive integer", "invalid_input")
		}
		limit = n
	}
	rows, err := h.svc.ListMine(c.UserContext(), siswaID, limit)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListMineResponse{Items: rows, Total: len(rows)})
}

// PendingHandler bundles HTTP handler untuk guru pending counters.
type PendingHandler struct {
	counter *PendingCounter
}

// NewPendingHandler returns a Handler bound to a PendingCounter.
func NewPendingHandler(c *PendingCounter) *PendingHandler {
	return &PendingHandler{counter: c}
}

// Count handles GET /api/v1/guru/pending-counts.
func (h *PendingHandler) Count(c *fiber.Ctx) error {
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)
	res, err := h.counter.Count(c.UserContext(), callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(res)
}

// GradeRequest is the JSON body for POST /submission/:id/grade.
type GradeRequest struct {
	NilaiAsli float64 `json:"nilai_asli"`
	Feedback  string  `json:"feedback"`
	Version   int     `json:"version"`
}

// Grade handles POST /api/v1/submission/:id/grade.
func (h *Handler) Grade(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid submission id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req GradeRequest
	if perr := c.BodyParser(&req); perr != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid json body: "+perr.Error(), "invalid_input")
	}

	sub, err := h.svc.Grade(c.UserContext(), id, callerID, role, GradeInput{
		NilaiAsli:       req.NilaiAsli,
		Feedback:        req.Feedback,
		ExpectedVersion: req.Version,
	}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(sub)
}
