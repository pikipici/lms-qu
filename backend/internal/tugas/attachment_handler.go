// HTTP handlers for tugas attachment upload/list/delete/presigned-url.
//
// Endpoints (Task 4.A.3):
//   - POST   /tugas/:id/attachments              (guru/admin owner — multipart)
//   - GET    /tugas/:id/attachments              (guru/admin/siswa enrolled)
//   - DELETE /tugas/:id/attachments/:attID       (guru/admin owner)
//   - GET    /tugas/:id/attachments/:attID/url   (guru/admin/siswa enrolled)
package tugas

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

// AttachmentListResponse is returned by GET /tugas/:id/attachments.
type AttachmentListResponse struct {
	Items []Attachment `json:"items"`
	Total int          `json:"total"`
}

// UploadAttachment handles POST /api/v1/tugas/:id/attachments (multipart).
//
// Form fields:
//   - file (required, the attachment binary)
//
// Status codes:
//   - 201 created            → {attachment, object_key, original_filename, size_bytes}
//   - 400 invalid_id / missing_file / attachment_limit_reached
//   - 403 forbidden          → not kelas owner / admin
//   - 404 not_found          → tugas missing
//   - 409 kelas_archived     → kelas archived
//   - 413 payload_too_large  → > 20MB
//   - 415 unsupported_mime   → mime not in allowlist (locked #46)
//   - 500 r2_put_failed / internal
//   - 503 r2_unavailable     → store not configured
func (h *Handler) UploadAttachment(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "missing file form field", "missing_file")
	}
	if fileHeader.Size > MaxTugasAttachmentBytes {
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxTugasAttachmentBytes/(1024*1024)),
			"payload_too_large")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "cannot open uploaded file", "open_failed")
	}
	defer src.Close()
	limited := io.LimitReader(src, MaxTugasAttachmentBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "read upload: "+err.Error(), "read_failed")
	}
	if int64(len(body)) > MaxTugasAttachmentBytes {
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxTugasAttachmentBytes/(1024*1024)),
			"payload_too_large")
	}

	in := UploadAttachmentInput{
		OriginalFilename: fileHeader.Filename,
		Body:             body,
	}
	att, err := h.svc.UploadAttachment(c.UserContext(), tugasID, callerID, role, in,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"attachment":        att,
		"object_key":        att.ObjectKey,
		"original_filename": att.OriginalFilename,
		"size_bytes":        att.SizeBytes,
	})
}

// ListAttachments handles GET /api/v1/tugas/:id/attachments.
func (h *Handler) ListAttachments(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	rows, err := h.svc.ListAttachments(c.UserContext(), tugasID, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(AttachmentListResponse{Items: rows, Total: len(rows)})
}

// DeleteAttachment handles DELETE /api/v1/tugas/:id/attachments/:attID.
func (h *Handler) DeleteAttachment(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
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

	if err := h.svc.DeleteAttachment(c.UserContext(), tugasID, attID, callerID, role,
		c.IP(), string(c.Request().Header.UserAgent())); err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"tugas_id":      tugasID,
		"attachment_id": attID,
	})
}

// AttachmentURL handles GET /api/v1/tugas/:id/attachments/:attID/url.
//
// Returns a 15-minute presigned download URL. Audit logged (locked #62).
func (h *Handler) AttachmentURL(c *fiber.Ctx) error {
	tugasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid tugas id", "invalid_id")
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

	res, err := h.svc.PresignAttachmentURL(c.UserContext(), tugasID, attID, callerID, role,
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

// silence unused import errors when this file is built without companions.
var _ = errors.New
