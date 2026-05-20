// HTTP handler for ImportJob domain (Task 2.D.2 — POST /admin/import-csv/upload).
package importjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
	"gorm.io/datatypes"
)

// uploadMaxBytes is a defensive cap matching MaxCSVBytes; we read into memory
// regardless because the parser needs the full buffer.
const uploadMaxBytes = MaxCSVBytes

// uploadService is the subset of *Service used by Handler. Defined here so
// tests can stub it out without standing up real Storage / Repo.
type uploadService interface {
	PreviewUpload(ctx context.Context, in PreviewUploadInput) (*PreviewUploadResult, error)
	GetPreview(ctx context.Context, id, adminID uuid.UUID) (*GetPreviewResult, error)
	Cancel(ctx context.Context, id, adminID uuid.UUID) (*CancelResult, error)
	Confirm(ctx context.Context, id, adminID uuid.UUID) (*ConfirmResult, error)
	DownloadCredentials(ctx context.Context, id, adminID uuid.UUID) (*DownloadCredentialsResult, error)
}

// auditLogger captures the LogAudit slice of auth.Repo. Inlined so we don't
// pull the full userRepo interface from the admin package.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Handler exposes ImportJob HTTP endpoints. Wired under /admin in main.go;
// the admin role guard is applied at the group level.
type Handler struct {
	svc   uploadService
	audit auditLogger
}

// NewHandler constructs an ImportJob HTTP handler. audit may be nil to skip
// audit logging (test convenience).
func NewHandler(svc uploadService, audit auditLogger) *Handler {
	return &Handler{svc: svc, audit: audit}
}

// PreviewUpload handles POST /api/v1/admin/import-csv/upload.
//
// Multipart form field name: "file". Max raw payload = MaxCSVBytes. On
// success returns 201 with:
//
//	{
//	  "job_id":        "<uuid>",
//	  "valid_count":   int,
//	  "invalid_count": int,  // includes duplicates
//	  "total_rows":    int,
//	  "preview_rows":  [Row],
//	  "expires_at":    RFC3339
//	}
func (h *Handler) PreviewUpload(c *fiber.Ctx) error {
	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return importError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "missing file form field", "missing_file")
	}
	if fileHeader.Size > uploadMaxBytes {
		return importError(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", uploadMaxBytes/(1024*1024)),
			"file_too_large")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "cannot open uploaded file", "open_failed")
	}
	defer src.Close()

	// Read with hard cap (+1 byte to detect "exactly at limit" vs overflow).
	limited := io.LimitReader(src, int64(uploadMaxBytes)+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "read upload: "+err.Error(), "read_failed")
	}
	if int64(len(body)) > uploadMaxBytes {
		return importError(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("file melebihi batas %d MB", uploadMaxBytes/(1024*1024)),
			"file_too_large")
	}

	res, err := h.svc.PreviewUpload(c.UserContext(), PreviewUploadInput{
		AdminID:  adminID,
		Filename: fileHeader.Filename,
		Body:     body,
	})
	if err != nil {
		status, code, msg := mapServiceError(err)
		return importError(c, status, msg, code)
	}

	objKey := ""
	if res.Job.ObjectKey != nil {
		objKey = *res.Job.ObjectKey
	}
	h.logAudit(c, adminID, res.Job.ID, "import_csv_uploaded", auditMeta(map[string]any{
		"filename":      res.Job.Filename,
		"object_key":    objKey,
		"total_rows":    res.ParseStats.Total,
		"valid_count":   res.ParseStats.Valid,
		"invalid_count": res.ParseStats.Invalid + res.ParseStats.Duplicates,
	}))

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"job_id":        res.Job.ID,
		"valid_count":   res.ParseStats.Valid,
		"invalid_count": res.ParseStats.Invalid + res.ParseStats.Duplicates,
		"total_rows":    res.ParseStats.Total,
		"preview_rows":  res.Rows,
		"expires_at":    res.Job.ExpiresAt.Format(time.RFC3339),
	})
}

// GetPreview handles GET /api/v1/admin/import-csv/:job_id.
//
// Resume endpoint for an admin's preview tab. Scoped to the admin who
// uploaded the job. Returns 200 with the same shape as PreviewUpload's
// success response. Status codes:
//   - 404 not_found       — job missing or owned by someone else
//   - 409 not_in_preview  — job already cancelled / processing / completed
//   - 410 preview_expired — preview TTL elapsed (FE drops cached tab)
func (h *Handler) GetPreview(c *fiber.Ctx) error {
	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return importError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	jobID, err := uuid.Parse(c.Params("job_id"))
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "job_id bukan UUID valid", "invalid_job_id")
	}

	res, err := h.svc.GetPreview(c.UserContext(), jobID, adminID)
	if err != nil {
		status, code, msg := mapServiceError(err)
		return importError(c, status, msg, code)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"job_id":        res.Job.ID,
		"valid_count":   res.Job.ValidCount,
		"invalid_count": res.Job.InvalidCount,
		"total_rows":    res.Job.TotalRows,
		"preview_rows":  res.Rows,
		"expires_at":    res.Job.ExpiresAt.Format(time.RFC3339),
		"filename":      res.Job.Filename,
		"status":        res.Job.Status,
	})
}

// Cancel handles POST /api/v1/admin/import-csv/:job_id/cancel.
//
// Flips a preview ImportJob to status=cancelled and best-effort deletes the
// R2 raw CSV. Audit log: import_csv_cancelled. Status codes mirror
// GetPreview (404/409/410). Success response is the cancelled job summary.
func (h *Handler) Cancel(c *fiber.Ctx) error {
	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return importError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	jobID, err := uuid.Parse(c.Params("job_id"))
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "job_id bukan UUID valid", "invalid_job_id")
	}

	res, err := h.svc.Cancel(c.UserContext(), jobID, adminID)
	if err != nil {
		status, code, msg := mapServiceError(err)
		return importError(c, status, msg, code)
	}

	h.logAudit(c, adminID, res.Job.ID, "import_csv_cancelled", auditMeta(map[string]any{
		"filename":   res.Job.Filename,
		"object_key": res.ObjectKey,
	}))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"job_id":   res.Job.ID,
		"status":   res.Job.Status,
		"filename": res.Job.Filename,
	})
}

// Confirm handles POST /api/v1/admin/import-csv/:job_id/confirm.
//
// Finalizes a preview ImportJob by creating users + auto-enrolling them and
// generating a credentials.csv stored at credentials/<job_uuid>.csv in R2.
// Always returns 200 on partial success — failures are surfaced via
// `errors_json` and the per-row failure list in the response body. Status:
//   - 200 ok                  → completed (may include partial failures)
//   - 404 not_found           → job missing or wrong owner
//   - 409 not_in_preview      → already confirmed/cancelled/expired/etc
//   - 410 preview_expired     → preview TTL elapsed
//   - 409 rows_mismatch       → R2 CSV diverges from preview state
//   - 500 confirm_failed      → R2 fetch / put / persist failure
func (h *Handler) Confirm(c *fiber.Ctx) error {
	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return importError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	jobID, err := uuid.Parse(c.Params("job_id"))
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "job_id bukan UUID valid", "invalid_job_id")
	}

	res, err := h.svc.Confirm(c.UserContext(), jobID, adminID)
	if err != nil {
		status, code, msg := mapServiceError(err)
		return importError(c, status, msg, code)
	}

	h.logAudit(c, adminID, res.Job.ID, "import_csv_confirmed", auditMeta(map[string]any{
		"filename":               res.Job.Filename,
		"object_key":             ptrToString(res.Job.ObjectKey),
		"credentials_object_key": res.CredentialsObjectKey,
		"success_count":          res.SuccessCount,
		"fail_count":             res.FailCount,
	}))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"job_id":                 res.Job.ID,
		"status":                 res.Job.Status,
		"filename":               res.Job.Filename,
		"success_count":          res.SuccessCount,
		"fail_count":             res.FailCount,
		"credentials_object_key": res.CredentialsObjectKey,
		"failures":               res.Failures,
	})
}

// DownloadCredentials handles GET /api/v1/admin/import-csv/:job_id/credentials.csv.
//
// Issues a presigned R2 GET URL with a forced attachment Content-Disposition
// and 302-redirects the client. The browser fetches R2 directly using that
// short-lived URL — the API never streams credential bytes itself. Status:
//   - 302 Found             → Location header set to presigned URL
//   - 400 invalid_job_id    → :job_id is not a UUID
//   - 404 not_found         → job missing or wrong owner
//   - 409 not_completed     → job is preview/processing/cancelled/etc
//   - 410 credentials_expired → CompletedAt + 1h elapsed
//   - 404 credentials_missing → job completed but no credentials_csv key
//                                (or R2 object went away)
//   - 500 download_failed   → R2 presign or other internal failure
//
// Audit action: file_url_issued — meta { object_key, filename, ttl_sec }.
func (h *Handler) DownloadCredentials(c *fiber.Ctx) error {
	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return importError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	jobID, err := uuid.Parse(c.Params("job_id"))
	if err != nil {
		return importError(c, fiber.StatusBadRequest, "job_id bukan UUID valid", "invalid_job_id")
	}

	res, err := h.svc.DownloadCredentials(c.UserContext(), jobID, adminID)
	if err != nil {
		status, code, msg := mapServiceError(err)
		return importError(c, status, msg, code)
	}

	h.logAudit(c, adminID, res.Job.ID, "file_url_issued", auditMeta(map[string]any{
		"object_key": res.ObjectKey,
		"filename":   res.Filename,
		"ttl_sec":    int(res.TTL.Seconds()),
	}))

	return c.Redirect(res.URL, fiber.StatusFound)
}

// mapServiceError translates parser/service sentinel errors into stable HTTP
// status + code pairs the FE can branch on.
func mapServiceError(err error) (status int, code string, msg string) {
	switch {
	case errors.Is(err, ErrCSVTooLarge):
		return fiber.StatusRequestEntityTooLarge, "csv_too_large", "csv melebihi batas ukuran"
	case errors.Is(err, ErrTooManyRows):
		return fiber.StatusBadRequest, "too_many_rows", fmt.Sprintf("csv melebihi %d baris", MaxCSVRows)
	case errors.Is(err, ErrEmptyCSV):
		return fiber.StatusBadRequest, "empty_csv", "csv kosong"
	case errors.Is(err, ErrMalformedHeader):
		return fiber.StatusBadRequest, "malformed_header", "header csv malformed"
	case errors.Is(err, ErrMissingNamaColumn):
		return fiber.StatusBadRequest, "missing_nama_column", "kolom nama tidak ditemukan"
	case errors.Is(err, ErrMissingEmailColumn):
		return fiber.StatusBadRequest, "missing_email_column", "kolom email tidak ditemukan"
	case errors.Is(err, ErrInvalidUTF8):
		return fiber.StatusBadRequest, "invalid_utf8", "csv bukan utf-8 valid (re-save sebagai 'CSV UTF-8' di excel)"
	case errors.Is(err, ErrUnsupportedMime):
		return fiber.StatusUnsupportedMediaType, "unsupported_mime", "hanya csv yang diterima"
	case errors.Is(err, ErrJobNotFound):
		return fiber.StatusNotFound, "not_found", "import job tidak ditemukan"
	case errors.Is(err, ErrJobExpired):
		return fiber.StatusGone, "preview_expired", "preview kadaluarsa, upload ulang"
	case errors.Is(err, ErrJobNotInPreview):
		return fiber.StatusConflict, "not_in_preview", "import job sudah tidak dalam status preview"
	case errors.Is(err, ErrConfirmRowsMismatch):
		return fiber.StatusConflict, "rows_mismatch", "csv di r2 berbeda dari preview, upload ulang"
	case errors.Is(err, ErrInternalConfirm):
		return fiber.StatusInternalServerError, "confirm_failed", "gagal memproses konfirmasi import"
	case errors.Is(err, ErrJobNotCompleted):
		return fiber.StatusConflict, "not_completed", "import job belum selesai"
	case errors.Is(err, ErrCredentialsExpired):
		return fiber.StatusGone, "credentials_expired", "credentials.csv kadaluarsa, jalankan import ulang"
	case errors.Is(err, ErrCredentialsMissing):
		return fiber.StatusNotFound, "credentials_missing", "credentials.csv tidak tersedia"
	case errors.Is(err, ErrInternalDownload):
		return fiber.StatusInternalServerError, "download_failed", "gagal generate link unduh"
	case errors.Is(err, ErrPersistFailed):
		return fiber.StatusInternalServerError, "persist_failed", "gagal menyimpan import job"
	default:
		return fiber.StatusInternalServerError, "internal", "internal server error"
	}
}

func (h *Handler) logAudit(c *fiber.Ctx, actorID, targetID uuid.UUID, action string, meta datatypes.JSON) {
	if h.audit == nil {
		return
	}
	actorRole := string(auth.Admin)
	targetType := "import_job"
	ip := c.IP()
	ua := strings.TrimSpace(string(c.Request().Header.UserAgent()))
	entry := &auth.AuditLog{
		ActorID:    &actorID,
		ActorRole:  &actorRole,
		Action:     action,
		TargetType: &targetType,
		TargetID:   &targetID,
		Meta:       meta,
		IP:         strPtrOrNil(ip),
		UserAgent:  strPtrOrNil(ua),
		At:         time.Now(),
	}
	if err := h.audit.LogAudit(c.UserContext(), entry); err != nil {
		slog.Warn("import audit log failed",
			slog.String("action", action),
			slog.String("target_id", targetID.String()),
			slog.String("err", err.Error()))
	}
}

func auditMeta(m map[string]any) datatypes.JSON {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ptrToString returns *p if non-nil, "" otherwise.
func ptrToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func importError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
