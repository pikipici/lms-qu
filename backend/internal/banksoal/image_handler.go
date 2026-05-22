// HTTP handlers untuk banksoal image slots (Task 6.B.2).
//
// Endpoints (mirror SoalBab Task 5.B.2, locked #78):
//   - POST   /api/v1/bank-soal/:id/image?slot=<pertanyaan|a|b|c|d|e>
//     (multipart, owner guru atau admin)
//   - DELETE /api/v1/bank-soal/:id/image?slot=<...>
//   - GET    /api/v1/bank-soal/:id/image-url?slot=<...>
//     (presigned GET URL TTL 15m, inline disposition)
//
// Locked decisions referenced:
//   - #46 mime allowlist (image/jpeg, image/png, image/webp).
//   - #62 presigned URL TTL 15m default.
//   - #69 hard delete + R2 cleanup compensating.
//   - #78 inline 6-slot, 5MB raw cap, resize 1920px.
//   - #84 per-guru pribadi — owner only (no cross-guru read).
package banksoal

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

// UploadImage handles POST /api/v1/bank-soal/:id/image?slot=<...>.
//
// Form fields:
//   - file (required, image binary; max 5MB raw)
//
// Status codes:
//   - 200 ok                 → {soal, slot, object_key, mime_type, size_bytes, width, height, replaced}
//   - 400 invalid_id / invalid_slot / missing_file
//   - 403 forbidden          → not soal owner / admin
//   - 404 not_found          → soal missing/soft-deleted
//   - 413 payload_too_large  → > 5MB
//   - 415 unsupported_mime   → mime not in allowlist
//   - 422 image_decode_failed / image_encode_failed
//   - 500 internal / image_upload_failed
//   - 503 r2_unavailable     → store not configured
func (h *Handler) UploadImage(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	slot := ImageSlot(strings.TrimSpace(c.Query("slot")))
	if !slot.Valid() {
		return errResp(c, fiber.StatusBadRequest, "slot must be pertanyaan|a|b|c|d|e", "invalid_slot")
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
	if fileHeader.Size > MaxBankSoalImageBytes {
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxBankSoalImageBytes/(1024*1024)),
			"payload_too_large")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "cannot open uploaded file", "open_failed")
	}
	defer src.Close()
	body, err := io.ReadAll(io.LimitReader(src, MaxBankSoalImageBytes+1))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "read upload: "+err.Error(), "read_failed")
	}
	if int64(len(body)) > MaxBankSoalImageBytes {
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxBankSoalImageBytes/(1024*1024)),
			"payload_too_large")
	}

	res, err := h.svc.UploadImage(c.UserContext(), id, callerID, role, UploadImageInput{
		Slot: slot,
		Body: body,
	}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapImageErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"soal":       res.Soal,
		"slot":       string(res.Slot),
		"object_key": res.ObjectKey,
		"mime_type":  res.MimeType,
		"size_bytes": res.SizeBytes,
		"width":      res.Width,
		"height":     res.Height,
		"replaced":   res.Replaced,
	})
}

// DeleteImage handles DELETE /api/v1/bank-soal/:id/image?slot=<...>.
func (h *Handler) DeleteImage(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	slot := ImageSlot(strings.TrimSpace(c.Query("slot")))
	if !slot.Valid() {
		return errResp(c, fiber.StatusBadRequest, "slot must be pertanyaan|a|b|c|d|e", "invalid_slot")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	soal, err := h.svc.DeleteImage(c.UserContext(), id, callerID, role, slot,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapImageErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"soal": soal,
		"slot": string(slot),
	})
}

// ImageURL handles GET /api/v1/bank-soal/:id/image-url?slot=<...>.
//
// Returns a 15-minute inline-disposition presigned download URL (locked #62).
func (h *Handler) ImageURL(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid soal id", "invalid_id")
	}
	slot := ImageSlot(strings.TrimSpace(c.Query("slot")))
	if !slot.Valid() {
		return errResp(c, fiber.StatusBadRequest, "slot must be pertanyaan|a|b|c|d|e", "invalid_slot")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.PresignImageURL(c.UserContext(), id, callerID, role, slot,
		c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapImageErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"url":        res.URL,
		"expires_at": res.ExpiresAt.Format(time.RFC3339),
		"slot":       string(res.Slot),
		"object_key": res.ObjectKey,
	})
}

// mapImageErr maps banksoal image sentinel errors to fiber HTTP responses.
func mapImageErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrImageSlotInvalid):
		return errResp(c, fiber.StatusBadRequest, "slot must be pertanyaan|a|b|c|d|e", "invalid_slot")
	case errors.Is(err, ErrImageSlotEmpty):
		return errResp(c, fiber.StatusNotFound, "image slot is empty", "image_slot_empty")
	case errors.Is(err, ErrImageUnsupportedMime):
		return errResp(c, fiber.StatusUnsupportedMediaType, "image mime not allowed (jpg/png/webp only)", "unsupported_mime")
	case errors.Is(err, ErrImageTooLarge):
		return errResp(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("image melebihi batas %d MB", MaxBankSoalImageBytes/(1024*1024)),
			"payload_too_large")
	case errors.Is(err, ErrImageDecodeFailed):
		return errResp(c, fiber.StatusUnprocessableEntity, "image decode failed (corrupt or unsupported)", "image_decode_failed")
	case errors.Is(err, ErrImageEncodeFailed):
		return errResp(c, fiber.StatusInternalServerError, "image encode failed", "image_encode_failed")
	case errors.Is(err, ErrImageUploadFailed):
		return errResp(c, fiber.StatusInternalServerError, "image upload failed", "image_upload_failed")
	case errors.Is(err, ErrR2Required):
		return errResp(c, fiber.StatusServiceUnavailable, "object store not configured", "r2_unavailable")
	default:
		return mapServiceErr(c, err)
	}
}
