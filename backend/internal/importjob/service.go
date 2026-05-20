// Service layer for ImportJob domain (bulk-import CSV via Cloudflare R2).
//
// Task 2.D.2: PreviewUpload — admin uploads CSV; we validate mime/size,
// parse + dedup, persist raw CSV to R2 (`import/<job_uuid>.csv`), and create
// an ImportJob row in status=preview with PreviewRowsJSON populated.
package importjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/storage"
)

// PreviewTTL is the lifetime of a preview ImportJob before it auto-expires
// (locked decision #54). Cleanup cron in Task 2.D.6 flips status=expired
// + DeleteObject(ObjectKey) once ExpiresAt < now.
const PreviewTTL = 1 * time.Hour

// Service-level errors. Wrapped with sentinels so the handler can surface
// stable codes to the client.
var (
	ErrUnsupportedMime = errors.New("import: only CSV uploads are accepted")
	ErrPersistFailed   = errors.New("import: persistence failed")
	// ErrJobNotFound — admin tried to read/cancel a job that doesn't exist
	// or that they don't own (FindByIDForAdmin scopes by admin_id).
	ErrJobNotFound = errors.New("import: job not found")
	// ErrJobNotInPreview — operation requires status=preview (e.g. cancel),
	// but the job has already moved on (processing/completed/expired/etc).
	ErrJobNotInPreview = errors.New("import: job not in preview status")
	// ErrJobExpired — job is preview but ExpiresAt < now. Resume should
	// surface as 410 so FE knows to drop the cached tab.
	ErrJobExpired = errors.New("import: preview window expired")
)

// jobRepo is the subset of *Repo used by Service. Defined as an interface
// so handler tests can inject a stub without standing up GORM.
type jobRepo interface {
	Create(ctx context.Context, j *ImportJob) error
	FindByIDForAdmin(ctx context.Context, id, adminID uuid.UUID) (*ImportJob, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status, confirmedAt, completedAt *time.Time) error
}

// Service orchestrates ImportJob upload + lifecycle.
type Service struct {
	repo    jobRepo
	store   storage.Storage
	now     func() time.Time
	previewLimit int // soft cap on rows persisted into PreviewRowsJSON
}

// NewService constructs a Service. previewLimit caps how many rows are
// embedded in PreviewRowsJSON for the UI; the full row list is still
// available via re-parse during confirm (Task 2.D.4). 0 = use default.
func NewService(repo jobRepo, store storage.Storage, previewLimit int) *Service {
	if previewLimit <= 0 {
		previewLimit = 200
	}
	return &Service{repo: repo, store: store, now: time.Now, previewLimit: previewLimit}
}

// SetClock overrides the time source (test hook).
func (s *Service) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	s.now = now
}

// PreviewUploadInput carries the raw upload payload + admin metadata.
type PreviewUploadInput struct {
	AdminID  uuid.UUID
	Filename string // user-supplied; we trust the parsed-out basename only
	Body     []byte // CSV bytes already read into memory (Task 2.D.2 size cap = MaxCSVBytes)
}

// PreviewUploadResult is the public response from PreviewUpload.
type PreviewUploadResult struct {
	Job        *ImportJob
	ParseStats ParseStat
	Rows       []Row // limited to previewLimit; full set lives in R2
}

// PreviewUpload validates + parses + persists a CSV bulk-import upload.
//
// Order of operations (failure rollback in reverse):
//  1. Sniff mime via http.DetectContentType — only text/csv or text/plain
//     accepted (Excel sometimes labels CSV as text/plain).
//  2. Parse CSV from in-memory buffer (locked decision #54: parse from
//     buffer, do NOT re-fetch from R2 to avoid coupling preview to network).
//  3. PutObject to R2 at "import/<job_uuid>.csv".
//  4. Insert ImportJob row in status=preview. If insert fails, DeleteObject
//     to avoid orphan R2 objects.
func (s *Service) PreviewUpload(ctx context.Context, in PreviewUploadInput) (*PreviewUploadResult, error) {
	// 1. Mime sniff. http.DetectContentType only inspects the first 512 bytes.
	// CSV detection is heuristic — DetectContentType returns "text/plain;
	// charset=utf-8" for most CSVs. We accept text/* to be lenient and rely
	// on the parser for real validation.
	if !looksLikeText(in.Body) {
		return nil, fmt.Errorf("%w: file does not appear to be text/CSV", ErrUnsupportedMime)
	}

	// 2. Parse.
	pr, err := Parse(bytes.NewReader(in.Body))
	if err != nil {
		return nil, err // parser returns sentinel errors; caller maps to 400
	}

	// 3. Persist to R2.
	jobID := uuid.New()
	objectKey, err := storage.BuildKey(storage.CategoryImport, jobID.String()+".csv")
	if err != nil {
		return nil, fmt.Errorf("import: build object key: %w", err)
	}
	if err := s.store.PutObject(ctx, storage.PutObjectInput{
		Key:         objectKey,
		Body:        bytes.NewReader(in.Body),
		Size:        int64(len(in.Body)),
		ContentType: "text/csv; charset=utf-8",
	}); err != nil {
		return nil, fmt.Errorf("import: r2 put: %w", err)
	}

	// 4. Insert ImportJob (with R2 cleanup compensation on DB failure).
	previewRows := pr.Rows
	if len(previewRows) > s.previewLimit {
		previewRows = previewRows[:s.previewLimit]
	}
	previewBlob, err := json.Marshal(previewRows)
	if err != nil {
		_ = s.store.DeleteObject(context.Background(), objectKey)
		return nil, fmt.Errorf("import: marshal preview: %w", err)
	}

	now := s.now()
	cleanFilename := sanitizeFilename(in.Filename)
	adminID := in.AdminID
	objKey := objectKey

	job := &ImportJob{
		ID:              jobID,
		AdminID:         &adminID,
		Filename:        cleanFilename,
		ObjectKey:       &objKey,
		Status:          StatusPreview,
		TotalRows:       pr.Stats.Total,
		ValidCount:      pr.Stats.Valid,
		InvalidCount:    pr.Stats.Invalid + pr.Stats.Duplicates,
		PreviewRowsJSON: previewBlob,
		ExpiresAt:       now.Add(PreviewTTL),
		CreatedAt:       now,
	}
	if err := s.repo.Create(ctx, job); err != nil {
		// Compensating delete: orphan R2 object would otherwise count toward
		// our cleanup cron's workload. Best-effort — log if this also fails.
		if delErr := s.store.DeleteObject(context.Background(), objectKey); delErr != nil {
			slog.Warn("import: r2 orphan cleanup failed",
				slog.String("object_key", objectKey),
				slog.String("err", delErr.Error()))
		}
		return nil, fmt.Errorf("%w: %v", ErrPersistFailed, err)
	}

	return &PreviewUploadResult{
		Job:        job,
		ParseStats: pr.Stats,
		Rows:       previewRows,
	}, nil
}

// GetPreviewResult is the public response from GetPreview (Task 2.D.3 resume).
type GetPreviewResult struct {
	Job  *ImportJob
	Rows []Row // decoded from PreviewRowsJSON
}

// GetPreview returns a preview ImportJob plus its decoded preview rows so an
// admin can resume an upload tab. Returns ErrJobNotFound if the job doesn't
// exist or isn't owned by adminID, ErrJobNotInPreview if status moved on,
// and ErrJobExpired if status=preview but the TTL elapsed.
//
// Resume scope (locked decision #54): admins only see their own jobs. We do
// NOT auto-flip expired-by-time jobs here — that's the cleanup cron's job
// (Task 2.D.6). Surfacing ErrJobExpired without mutating state means the FE
// can prompt "preview kadaluarsa, upload ulang" without racing the cron.
func (s *Service) GetPreview(ctx context.Context, id, adminID uuid.UUID) (*GetPreviewResult, error) {
	job, err := s.repo.FindByIDForAdmin(ctx, id, adminID)
	if err != nil {
		return nil, ErrJobNotFound
	}
	if job.Status != StatusPreview {
		return nil, ErrJobNotInPreview
	}
	if !job.ExpiresAt.IsZero() && s.now().After(job.ExpiresAt) {
		return nil, ErrJobExpired
	}

	var rows []Row
	if len(job.PreviewRowsJSON) > 0 {
		if err := json.Unmarshal(job.PreviewRowsJSON, &rows); err != nil {
			return nil, fmt.Errorf("import: decode preview rows: %w", err)
		}
	}
	return &GetPreviewResult{Job: job, Rows: rows}, nil
}

// CancelResult carries the cancelled job + the R2 object key that was
// removed (handler audit-logs both).
type CancelResult struct {
	Job       *ImportJob
	ObjectKey string // empty if job had no ObjectKey (defensive)
}

// Cancel flips a preview ImportJob to status=cancelled and best-effort
// deletes its R2 raw CSV. Idempotent semantics:
//   - Already cancelled: returns ErrJobNotInPreview (handler maps to 409).
//     We choose 409 over 200 so admins notice if a stale tab fires the
//     cancel button after the cleanup cron beat them to it.
//   - Already expired/processing/completed/failed: same — ErrJobNotInPreview.
//
// R2 delete is post-DB-commit and best-effort: if it fails we log + return
// success anyway, since the cleanup cron sweeps cancelled jobs eventually.
func (s *Service) Cancel(ctx context.Context, id, adminID uuid.UUID) (*CancelResult, error) {
	job, err := s.repo.FindByIDForAdmin(ctx, id, adminID)
	if err != nil {
		return nil, ErrJobNotFound
	}
	if job.Status != StatusPreview {
		return nil, ErrJobNotInPreview
	}

	// Flip status first. If R2 delete fails afterwards, the cron will
	// catch the orphan; if we deleted R2 first and the DB update failed,
	// we'd have a preview row pointing to a missing object.
	if err := s.repo.SetStatus(ctx, job.ID, StatusCancelled, nil, nil); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPersistFailed, err)
	}
	job.Status = StatusCancelled

	objKey := ""
	if job.ObjectKey != nil {
		objKey = *job.ObjectKey
	}
	if objKey != "" {
		// Use background context so a client cancellation between status
		// flip and R2 delete doesn't orphan the object more than necessary.
		if err := s.store.DeleteObject(context.Background(), objKey); err != nil {
			slog.Warn("import: r2 delete on cancel failed",
				slog.String("job_id", job.ID.String()),
				slog.String("object_key", objKey),
				slog.String("err", err.Error()))
		}
	}
	return &CancelResult{Job: job, ObjectKey: objKey}, nil
}

// looksLikeText sniffs the body for plausible CSV. We accept anything that
// http.DetectContentType identifies as text/* — full validation happens in
// the parser.
func looksLikeText(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	probe := body
	if len(probe) > 512 {
		probe = probe[:512]
	}
	mime := http.DetectContentType(probe)
	// Common return values for CSV: "text/plain; charset=utf-8", "text/csv".
	// Excel "CSV UTF-8" emits a BOM which sometimes flips detection to
	// "text/plain; charset=utf-8" anyway.
	mime = strings.ToLower(mime)
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	if strings.HasPrefix(mime, "application/csv") {
		return true
	}
	// Reject everything else (octet-stream, image/*, etc).
	return false
}

// sanitizeFilename strips path components and limits length. We only keep
// the basename for display; the actual R2 key is uuid-based, so no caller
// can trick us into a directory traversal via filename.
func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "upload.csv"
	}
	// Strip both forward and backslash path components (Windows-safe).
	if idx := strings.LastIndexAny(s, "/\\"); idx >= 0 {
		s = s[idx+1:]
	}
	// Cap length so DB filename column doesn't bloat.
	if len(s) > 255 {
		s = s[:255]
	}
	if s == "" {
		s = "upload.csv"
	}
	// Default to .csv extension if missing — purely cosmetic for the UI.
	if filepath.Ext(s) == "" {
		s = s + ".csv"
	}
	return s
}
