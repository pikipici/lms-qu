// PDF upload + presigned download for materi (Task 3.C.3).
//
// Upload pipeline (locked decisions #46 + #62 + #64):
//   1. Auth + ownership guard via service.findKelasOrForbidden (handler).
//   2. File header sniff via http.DetectContentType — must be application/pdf
//      (locked #63 + #46). Reject 415 unsupported_mime if mismatch.
//   3. Size cap MaxMateriBytes = 20MB (locked #64). Reject 413 oversize.
//   4. Generate uuid → object_key = "materi/<uuid>.pdf".
//   5. R2 PutObject. On fail: 500 (no DB row yet — clean state).
//   6. Insert DB row with tipe='pdf' + object_key + original_filename +
//      mime_type + size_bytes. On fail: compensating R2 DeleteObject +
//      500 materi_db_failed (locked #62 trade-off — bandwidth dobel ok).
//   7. Return {materi, object_key, original_filename, size_bytes}.
//
// Presigned download (locked #62):
//   GET /materi/:id/file-url → store.PresignGetDownload(key, ttl, original)
//   with inline disposition (FE renders PDF di iframe, locked roadmap §3.D.2).
//   Audit log file_url_issued.
package materi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/storage"
)

// MaxMateriBytes is the hard cap per PDF upload (locked #64). Constant
// scoped to materi domain — CSV import has its own cap di importjob.
const MaxMateriBytes int64 = 20 * 1024 * 1024

// PresignTTL is the default TTL for materi PDF presigned download URLs
// (locked roadmap — 15 minutes per #62).
const PresignTTL = 15 * time.Minute

// pdfMimeType is the only mime allowed for materi PDF uploads (locked #64).
const pdfMimeType = "application/pdf"

// Sentinel errors specific to the upload/presign paths.
var (
	ErrUnsupportedMime = errors.New("materi: unsupported mime type")
	ErrPayloadTooLarge = errors.New("materi: payload too large")
	ErrUploadFailed    = errors.New("materi: upload failed")
	ErrR2Required      = errors.New("materi: object store not configured")
)

// UploadInput holds fields for POST /kelas/:id/materi/upload.
//
// Body is the in-memory PDF bytes (handler reads multipart with hard cap).
// OriginalFilename is sanitized client-supplied filename, persisted di DB
// untuk download UX (locked #46 + #58).
type UploadInput struct {
	BabID            *uuid.UUID
	Judul            string
	OriginalFilename string
	Body             []byte
}

// Upload validates + stores a PDF materi via R2 + inserts a DB row.
//
// Trade-offs (locked #62): client → backend → R2 (no direct browser→R2 in
// MVP). Bandwidth doubles but auth + mime + size validation stays on
// trusted server side. Compensating delete on DB insert failure keeps
// R2 + DB consistent.
func (s *Service) Upload(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in UploadInput, ip, userAgent string) (*Materi, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if len(in.Body) == 0 {
		return nil, fmt.Errorf("%w: empty file body", ErrInvalidInput)
	}
	if int64(len(in.Body)) > MaxMateriBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrPayloadTooLarge, len(in.Body), MaxMateriBytes)
	}

	// Mime sniff via the standard library (#46). Only the first 512 bytes
	// are inspected; we reject anything that doesn't sniff to PDF.
	probe := in.Body
	if len(probe) > 512 {
		probe = probe[:512]
	}
	mime := http.DetectContentType(probe)
	if !strings.HasPrefix(mime, pdfMimeType) {
		return nil, fmt.Errorf("%w: detected %q, want %q", ErrUnsupportedMime, mime, pdfMimeType)
	}

	// Ownership + lifecycle guard.
	k, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}
	if in.BabID != nil {
		if err := s.assertBabInKelas(ctx, *in.BabID, kelasID); err != nil {
			return nil, err
		}
	}

	// Build object key under materi/<uuid>.pdf (locked #58 + #61).
	objectID := uuid.New()
	objectKey, err := storage.BuildKey(storage.CategoryMateri, objectID.String()+".pdf")
	if err != nil {
		return nil, fmt.Errorf("materi upload: build key: %w", err)
	}

	// Compute next urutan slot before R2 PutObject so an early DB error
	// triggers compensating R2 delete only when actually needed.
	babFilter := babFilterFrom(in.BabID)
	maxUrutan, err := s.repo.MaxUrutan(ctx, kelasID, babFilter)
	if err != nil {
		return nil, fmt.Errorf("materi upload max urutan: %w", err)
	}

	if perr := s.store.PutObject(ctx, storage.PutObjectInput{
		Key:         objectKey,
		Body:        bytes.NewReader(in.Body),
		Size:        int64(len(in.Body)),
		ContentType: pdfMimeType,
	}); perr != nil {
		return nil, fmt.Errorf("%w: %v", ErrUploadFailed, perr)
	}

	cleanFilename := sanitizeFilename(in.OriginalFilename)
	mimeStr := pdfMimeType
	size := int64(len(in.Body))
	row := &Materi{
		KelasID:          kelasID,
		BabID:            in.BabID,
		Judul:            judul,
		Tipe:             TipePDF,
		Konten:           "",
		ObjectKey:        &objectKey,
		OriginalFilename: &cleanFilename,
		MimeType:         &mimeStr,
		SizeBytes:        &size,
		Urutan:           maxUrutan + 1,
		Version:          1,
	}
	if cerr := s.repo.Create(ctx, row); cerr != nil {
		// Compensating R2 cleanup — DB row never landed, drop the orphan.
		if derr := s.store.DeleteObject(context.Background(), objectKey); derr != nil {
			s.logAudit(ctx, "materi_r2_orphan", &callerID, callerRole, nil, &kelasID, ip, userAgent, map[string]any{
				"object_key": objectKey,
				"reason":     "compensating_delete_failed",
				"err":        derr.Error(),
			})
		}
		return nil, fmt.Errorf("materi upload db: %w", cerr)
	}

	s.logAudit(ctx, "materi_uploaded", &callerID, callerRole, &row.ID, &kelasID, ip, userAgent, map[string]any{
		"materi_id":         row.ID.String(),
		"bab_id":            babIDStr(in.BabID),
		"judul":             row.Judul,
		"tipe":              string(row.Tipe),
		"object_key":        objectKey,
		"original_filename": cleanFilename,
		"mime_type":         pdfMimeType,
		"size_bytes":        size,
	})
	return row, nil
}

// FileURLResult is returned by PresignFileURL.
type FileURLResult struct {
	URL              string
	ExpiresAt        time.Time
	OriginalFilename string
	MimeType         string
}

// PresignFileURL issues a short-lived GET URL for a materi PDF. Caller must
// have ownership/enrollment on the materi's kelas (handler enforces; this
// method delegates to findKelasOrForbidden).
//
// Returns ErrTipeUnsupported when called on non-pdf tipe (FE shouldn't ask
// — markdown/youtube don't have R2 payloads).
func (s *Service) PresignFileURL(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*FileURLResult, error) {
	if s.store == nil {
		return nil, ErrR2Required
	}
	m, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("materi presign find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, m.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	if m.Tipe != TipePDF {
		return nil, fmt.Errorf("%w: presigned download only available for tipe=pdf", ErrTipeUnsupported)
	}
	if m.ObjectKey == nil || *m.ObjectKey == "" {
		return nil, fmt.Errorf("materi presign: object_key missing — schema invariant violated")
	}

	original := ""
	if m.OriginalFilename != nil {
		original = *m.OriginalFilename
	}
	url, perr := s.store.PresignGetDownload(ctx, *m.ObjectKey, PresignTTL, original)
	if perr != nil {
		if errors.Is(perr, storage.ErrObjectNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("materi presign: %w", perr)
	}

	expiresAt := s.now().Add(PresignTTL)
	mime := pdfMimeType
	if m.MimeType != nil {
		mime = *m.MimeType
	}
	s.logAudit(ctx, "materi_file_url_issued", &callerID, callerRole, &id, &m.KelasID, ip, userAgent, map[string]any{
		"materi_id":  id.String(),
		"object_key": *m.ObjectKey,
		"ttl":        int(PresignTTL.Seconds()),
	})
	return &FileURLResult{
		URL:              url,
		ExpiresAt:        expiresAt,
		OriginalFilename: original,
		MimeType:         mime,
	}, nil
}

// sanitizeFilename strips path separators and trims to a max length so a
// hostile client can't smuggle traversal characters into the original
// filename column. We keep this lenient — it's stored in DB column for UX
// only; download URL identity is the object_key uuid.
func sanitizeFilename(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "materi.pdf"
	}
	s = path.Base(s)
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, "/", "")
	s = strings.ReplaceAll(s, "\x00", "")
	if s == "" || s == "." || s == ".." {
		return "materi.pdf"
	}
	const maxLen = 200
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// ---------- HTTP handlers ----------

// Upload handles POST /api/v1/kelas/:id/materi/upload (multipart/form-data).
//
// Form fields:
//   - file       (required, the PDF)
//   - judul      (required)
//   - bab_id     (optional uuid; omit or "null" → no bab association)
//
// Status codes:
//   - 201 created            → {materi, object_key, original_filename, size_bytes}
//   - 400 invalid_body       → missing file or judul
//   - 400 invalid_id         → bab_id not a uuid
//   - 403 forbidden          → not kelas owner / admin
//   - 404 not_found          → kelas missing
//   - 409 kelas_archived     → kelas archived
//   - 413 payload_too_large  → > 20MB
//   - 415 unsupported_mime   → mime sniff != application/pdf
//   - 500 internal           → R2 PutObject or DB insert failure
func (h *Handler) Upload(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "missing file form field", "missing_file")
	}
	if fileHeader.Size > MaxMateriBytes {
		return materiError(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxMateriBytes/(1024*1024)),
			"payload_too_large")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "cannot open uploaded file", "open_failed")
	}
	defer src.Close()
	limited := io.LimitReader(src, MaxMateriBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "read upload: "+err.Error(), "read_failed")
	}
	if int64(len(body)) > MaxMateriBytes {
		return materiError(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxMateriBytes/(1024*1024)),
			"payload_too_large")
	}

	judul := strings.TrimSpace(c.FormValue("judul"))
	if judul == "" {
		return materiError(c, fiber.StatusBadRequest, "judul is required", "invalid_body")
	}

	var babID *uuid.UUID
	if rawBab := strings.TrimSpace(c.FormValue("bab_id")); rawBab != "" && !strings.EqualFold(rawBab, "null") {
		parsed, perr := uuid.Parse(rawBab)
		if perr != nil {
			return materiError(c, fiber.StatusBadRequest, "invalid bab_id", "invalid_id")
		}
		babID = &parsed
	}

	in := UploadInput{
		BabID:            babID,
		Judul:            judul,
		OriginalFilename: fileHeader.Filename,
		Body:             body,
	}
	row, err := h.svc.Upload(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapUploadErr(c, err)
	}
	resp := fiber.Map{
		"materi":            row,
		"object_key":        derefString(row.ObjectKey),
		"original_filename": derefString(row.OriginalFilename),
	}
	if row.SizeBytes != nil {
		resp["size_bytes"] = *row.SizeBytes
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// FileURL handles GET /api/v1/materi/:id/file-url.
//
// Status codes:
//   - 200 ok                 → {url, expires_at, original_filename, mime_type}
//   - 400 invalid_id
//   - 400 tipe_unsupported   → tipe != pdf (FE shouldn't ask)
//   - 403 forbidden
//   - 404 not_found
//   - 500 internal
func (h *Handler) FileURL(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return materiError(c, fiber.StatusBadRequest, "invalid materi id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return materiError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	res, err := h.svc.PresignFileURL(c.UserContext(), id, callerID, role, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapUploadErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"url":               res.URL,
		"expires_at":        res.ExpiresAt.Format(time.RFC3339),
		"original_filename": res.OriginalFilename,
		"mime_type":         res.MimeType,
	})
}

// mapUploadErr translates upload/presign service errors to HTTP responses.
// Falls through to mapServiceErr for shared errors (forbidden, not_found,
// kelas_archived, etc.) so we don't duplicate mapping logic.
func mapUploadErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrUnsupportedMime):
		return materiError(c, fiber.StatusUnsupportedMediaType, "file must be application/pdf", "unsupported_mime")
	case errors.Is(err, ErrPayloadTooLarge):
		return materiError(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", MaxMateriBytes/(1024*1024)),
			"payload_too_large")
	case errors.Is(err, ErrUploadFailed):
		slog.Error("materi upload r2", slog.String("err", err.Error()))
		return materiError(c, fiber.StatusInternalServerError, "upload to object store failed", "r2_put_failed")
	case errors.Is(err, ErrR2Required):
		return materiError(c, fiber.StatusServiceUnavailable, "object store not configured", "r2_unavailable")
	}
	return mapServiceErr(c, err)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
