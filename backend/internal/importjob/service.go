// Service layer for ImportJob domain (bulk-import CSV via Cloudflare R2).
//
// Task 2.D.2: PreviewUpload — admin uploads CSV; we validate mime/size,
// parse + dedup, persist raw CSV to R2 (`import/<job_uuid>.csv`), and create
// an ImportJob row in status=preview with PreviewRowsJSON populated.
package importjob

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/storage"
	"gorm.io/gorm"
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
	// ErrConfirmRowsMismatch — re-parsing the R2 raw CSV produced a different
	// (smaller) total than what was recorded at upload time. Surface as 409
	// so the admin re-uploads instead of silently committing a smaller batch.
	ErrConfirmRowsMismatch = errors.New("import: re-parsed CSV diverges from preview")
	// ErrInternalConfirm — generic Confirm-time failure (R2 fetch, marshal,
	// etc) that isn't covered by a more-specific sentinel.
	ErrInternalConfirm = errors.New("import: confirm failed")
	// ErrJobNotCompleted — download credentials requested but job is not
	// in status=completed (e.g. still preview/processing/cancelled). Surface
	// as 409 so FE knows to wait for confirm to finish first.
	ErrJobNotCompleted = errors.New("import: job not completed")
	// ErrCredentialsExpired — credentials.csv lives at most 1h after
	// CompletedAt before the cleanup cron deletes it (Task 2.D.6). Surfaced
	// as 410 so FE drops the download button.
	ErrCredentialsExpired = errors.New("import: credentials window expired")
	// ErrCredentialsMissing — job is completed but no credentials_csv key
	// is set (e.g. Confirm flipped to failed before uploading). Surface as
	// 404 so admin re-runs.
	ErrCredentialsMissing = errors.New("import: credentials object not available")
	// ErrInternalDownload — generic Download-time failure (R2 presign, etc).
	ErrInternalDownload = errors.New("import: download failed")
)

// jobRepo is the subset of *Repo used by Service. Defined as an interface
// so handler tests can inject a stub without standing up GORM.
type jobRepo interface {
	Create(ctx context.Context, j *ImportJob) error
	FindByIDForAdmin(ctx context.Context, id, adminID uuid.UUID) (*ImportJob, error)
	SetStatus(ctx context.Context, id uuid.UUID, status Status, confirmedAt, completedAt *time.Time) error
	SetCounts(ctx context.Context, id uuid.UUID, success, fail int) error
	SetErrorsJSON(ctx context.Context, id uuid.UUID, errorsJSON []byte) error
	SetCredentialsPath(ctx context.Context, id uuid.UUID, path string) error
}

// userCreator is the slim auth.Repo subset Service.Confirm needs to provision
// imported siswa accounts. Kept narrow so tests can stub without standing up
// the full *auth.Repo.
type userCreator interface {
	FindUserByEmail(ctx context.Context, email string) (*auth.User, error)
	CreateUser(ctx context.Context, u *auth.User) error
}

// kelasFinderEnroller is the slim kelas.Repo subset Service.Confirm needs.
// `Enroll` returns (inserted bool, err); inserted=false means a prior active
// enrollment exists which we treat as success (already_enrolled).
type kelasFinderEnroller interface {
	FindByKodeInvite(ctx context.Context, kode string) (*kelas.Kelas, error)
	Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via kelas.JoinedVia) (bool, error)
}

// Service orchestrates ImportJob upload + lifecycle.
type Service struct {
	repo         jobRepo
	store        storage.Storage
	users        userCreator
	kelasRepo    kelasFinderEnroller
	now          func() time.Time
	previewLimit int           // soft cap on rows persisted into PreviewRowsJSON
	bcryptCost   int           // 0 = use bcrypt.DefaultCost via auth.HashPassword
	presignTTL   time.Duration // 0 = use 15min default; bumped via SetPresignTTL
}

// NewService constructs a Service. previewLimit caps how many rows are
// embedded in PreviewRowsJSON for the UI; the full row list is still
// available via re-parse during confirm (Task 2.D.4). 0 = use default.
//
// users + kelasRepo are nil-safe: PreviewUpload / GetPreview / Cancel work
// without them; only Confirm requires both. Tests covering only the upload
// flow can pass nil, nil to keep wiring minimal.
func NewService(repo jobRepo, store storage.Storage, previewLimit int) *Service {
	if previewLimit <= 0 {
		previewLimit = 200
	}
	return &Service{repo: repo, store: store, now: time.Now, previewLimit: previewLimit}
}

// SetUserCreator injects the auth-side surface needed by Confirm. Wire from
// main.go after constructing both services to avoid circular imports.
func (s *Service) SetUserCreator(u userCreator) { s.users = u }

// SetKelasRepo injects the kelas-side surface needed by Confirm.
func (s *Service) SetKelasRepo(k kelasFinderEnroller) { s.kelasRepo = k }

// SetBcryptCost overrides the bcrypt cost used when hashing generated
// passwords. 0 means use auth.HashPassword's default (bcrypt.DefaultCost).
// Tests pass bcrypt.MinCost to keep them fast.
func (s *Service) SetBcryptCost(c int) { s.bcryptCost = c }

// SetPresignTTL overrides the lifetime of presigned download URLs. Wired
// from main.go via cfg.Storage.R2.PresignTTLSec. <=0 falls back to 15min.
func (s *Service) SetPresignTTL(d time.Duration) {
	if d <= 0 {
		d = 0 // service.DownloadCredentials applies the 15m default
	}
	s.presignTTL = d
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

// credentialRow is a package-private staging type for credentials.csv build.
type credentialRow struct {
	Email     string
	Plain     string
	KodeKelas string
	KelasNama string
}

// ConfirmResult is the public response from Confirm.
type ConfirmResult struct {
	Job                  *ImportJob
	SuccessCount         int
	FailCount            int
	CredentialsObjectKey string // R2 key for credentials/<job_uuid>.csv (empty if 0 success rows)
	Failures             []ConfirmFailure
}

// ConfirmFailure is one row that did not produce a user (or did but failed
// to enroll). Persisted into ImportJob.ErrorsJSON for forensic queryability.
type ConfirmFailure struct {
	LineNo int    `json:"line_no"`
	Email  string `json:"email"`
	Reason string `json:"reason"` // stable enum, see Reason* constants
	Detail string `json:"detail,omitempty"`
}

// Reason codes used in ImportJob.ErrorsJSON. Stable so FE can map to UI
// copy without parsing free-form strings.
const (
	ConfirmReasonInvalidRow      = "invalid_row"       // parser flagged row as invalid
	ConfirmReasonDuplicateInDB   = "duplicate_in_db"   // email already exists in users table
	ConfirmReasonUserCreate      = "user_create_error" // unexpected DB failure on User insert
	ConfirmReasonHashError       = "hash_error"        // bcrypt or RNG failure
	ConfirmReasonKelasNotFound   = "kelas_not_found"   // kode_kelas given but no matching kelas
	ConfirmReasonEnrollError     = "enroll_error"      // user inserted but enroll DB call failed
)

// Confirm finalizes a preview ImportJob by creating users and (optionally)
// enrolling them into matching kelas. Locked decision #54 lifecycle:
//   preview → processing (lock at flip) → completed (always; partial failures
//   captured in ErrorsJSON, not by failing the whole call).
//
// Idempotency: a second confirm call on the same job returns 409 because
// status moved off preview at the lock-flip; admins should retry uploads
// instead. Hard preconditions (job not found / wrong owner / not preview /
// expired) match GetPreview's semantics.
//
// Order of operations:
//  1. FindByIDForAdmin + status/expiry guards
//  2. SetStatus preview → processing (CONFIRMED_AT=now). Acts as a lock so
//     a parallel confirm on the same job hits StatusNotInPreview and bails.
//  3. Re-fetch raw CSV from R2 ObjectKey + re-parse via Parse. Source-of-
//     truth: PreviewRowsJSON is capped (200 rows) so we cannot use it to
//     create the full batch.
//  4. Loop valid rows: bcrypt random pw → User.Create → if kode_kelas given,
//     resolve via FindByKodeInvite → Enroll. Each failure recorded in a
//     ConfirmFailure but never aborts the overall call.
//  5. Build credentials.csv from successful rows, PutObject to R2 at
//     credentials/<job_uuid>.csv, persist key via SetCredentialsPath.
//  6. SetCounts + SetErrorsJSON + SetStatus processing → completed
//     (COMPLETED_AT=now).
func (s *Service) Confirm(ctx context.Context, id, adminID uuid.UUID) (*ConfirmResult, error) {
	if s.users == nil || s.kelasRepo == nil {
		return nil, fmt.Errorf("%w: confirm dependencies not wired", ErrInternalConfirm)
	}

	// 1. Guard.
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
	if job.ObjectKey == nil || *job.ObjectKey == "" {
		return nil, fmt.Errorf("%w: job has no R2 object key", ErrInternalConfirm)
	}

	// 2. Lock: flip preview → processing. If this fails, leave job in preview
	//    (next admin attempt can retry). If it succeeds and a step below
	//    fails, we still flip to completed at the end with errors_json
	//    populated — never leave a job stuck in processing.
	confirmedAt := s.now()
	if err := s.repo.SetStatus(ctx, job.ID, StatusProcessing, &confirmedAt, nil); err != nil {
		return nil, fmt.Errorf("%w: lock to processing: %v", ErrPersistFailed, err)
	}
	job.Status = StatusProcessing
	job.ConfirmedAt = &confirmedAt

	// 3. Re-fetch + re-parse from R2.
	obj, err := s.store.GetObject(ctx, *job.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("%w: r2 get %s: %v", ErrInternalConfirm, *job.ObjectKey, err)
	}
	body, readErr := readAllAndClose(obj)
	if readErr != nil {
		return nil, fmt.Errorf("%w: r2 read body: %v", ErrInternalConfirm, readErr)
	}
	pr, parseErr := Parse(bytes.NewReader(body))
	if parseErr != nil {
		// CSV that previously parsed clean now fails — corruption or admin
		// edited the bucket directly. Surface ErrInternalConfirm with the
		// underlying sentinel for log forensics; keep job in processing
		// only briefly — flip to failed via SetStatus on exit.
		_ = s.repo.SetStatus(ctx, job.ID, StatusFailed, nil, ptrTime(s.now()))
		return nil, fmt.Errorf("%w: reparse: %v", ErrInternalConfirm, parseErr)
	}
	if pr.Stats.Total < job.TotalRows {
		_ = s.repo.SetStatus(ctx, job.ID, StatusFailed, nil, ptrTime(s.now()))
		return nil, ErrConfirmRowsMismatch
	}

	// 4. Loop. Each per-row failure is captured, never aborts.
	creds := make([]credentialRow, 0, pr.Stats.Valid)
	failures := make([]ConfirmFailure, 0)

	for _, row := range pr.Rows {
		if row.Status != RowValid {
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo,
				Email:  row.Email,
				Reason: ConfirmReasonInvalidRow,
				Detail: strings.Join(row.Errors, "; "),
			})
			continue
		}

		// Pre-check duplicate by email so we don't burn bcrypt cycles on a
		// user that's going to fail at insert anyway. Race window remains
		// (handled at the unique-violation path below) but most cases are
		// caught here cheaply.
		existing, fErr := s.users.FindUserByEmail(ctx, row.Email)
		if fErr != nil && !errors.Is(fErr, gorm.ErrRecordNotFound) {
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo, Email: row.Email,
				Reason: ConfirmReasonUserCreate, Detail: fErr.Error(),
			})
			continue
		}
		if existing != nil {
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo, Email: row.Email,
				Reason: ConfirmReasonDuplicateInDB,
			})
			continue
		}

		plain, gErr := GeneratePassword()
		if gErr != nil {
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo, Email: row.Email,
				Reason: ConfirmReasonHashError, Detail: gErr.Error(),
			})
			continue
		}
		hash, hErr := auth.HashPassword(plain, s.bcryptCost)
		if hErr != nil {
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo, Email: row.Email,
				Reason: ConfirmReasonHashError, Detail: hErr.Error(),
			})
			continue
		}

		actor := adminID
		newUser := &auth.User{
			Name:               row.Nama,
			Email:              row.Email,
			PasswordHash:       hash,
			Role:               auth.Siswa,
			Status:             auth.Active,
			MustChangePassword: true,
			CreatedByID:        &actor,
		}
		if cErr := s.users.CreateUser(ctx, newUser); cErr != nil {
			reason := ConfirmReasonUserCreate
			if isDuplicateEmail(cErr) {
				reason = ConfirmReasonDuplicateInDB
			}
			failures = append(failures, ConfirmFailure{
				LineNo: row.LineNo, Email: row.Email,
				Reason: reason, Detail: cErr.Error(),
			})
			continue
		}

		// Attempt enroll if kode_kelas provided. User is already created;
		// failures here go into errors_json but credentials.csv still
		// includes the email/password (admin can enroll manually).
		kelasNama := ""
		if kode := strings.TrimSpace(row.KodeKelas); kode != "" {
			k, kErr := s.kelasRepo.FindByKodeInvite(ctx, kode)
			if errors.Is(kErr, gorm.ErrRecordNotFound) {
				failures = append(failures, ConfirmFailure{
					LineNo: row.LineNo, Email: row.Email,
					Reason: ConfirmReasonKelasNotFound, Detail: kode,
				})
			} else if kErr != nil {
				failures = append(failures, ConfirmFailure{
					LineNo: row.LineNo, Email: row.Email,
					Reason: ConfirmReasonEnrollError, Detail: kErr.Error(),
				})
			} else {
				if _, eErr := s.kelasRepo.Enroll(ctx, k.ID, newUser.ID, kelas.JoinedViaAdmin); eErr != nil {
					failures = append(failures, ConfirmFailure{
						LineNo: row.LineNo, Email: row.Email,
						Reason: ConfirmReasonEnrollError, Detail: eErr.Error(),
					})
				} else {
					kelasNama = k.Nama
				}
			}
		}

		creds = append(creds, credentialRow{
			Email:     row.Email,
			Plain:     plain,
			KodeKelas: row.KodeKelas,
			KelasNama: kelasNama,
		})
	}

	// 5. Build + upload credentials.csv. Always emit — even an empty
	//    success-set still gets a header-only CSV so the FE can show
	//    "0 berhasil, N gagal" and provide a download for the failure log
	//    (errors_json is the canonical source for failures, but having the
	//    object exist keeps the UI uniform).
	credKey, putErr := s.uploadCredentials(ctx, job.ID, creds)
	if putErr != nil {
		// User rows are already in DB; we cannot roll back. Leave job in
		// failed state with no credentials.csv key but DO populate errors
		// so admin sees what got created. Operator can re-run a credential
		// regeneration job later.
		_ = s.persistFailures(ctx, job.ID, len(creds), len(failures), failures, "")
		_ = s.repo.SetStatus(ctx, job.ID, StatusFailed, nil, ptrTime(s.now()))
		return nil, fmt.Errorf("%w: r2 put credentials: %v", ErrInternalConfirm, putErr)
	}

	// 6. Persist counters + errors + flip to completed.
	if err := s.persistFailures(ctx, job.ID, len(creds), len(failures), failures, credKey); err != nil {
		// Don't unwind: job is functionally complete. Log + best-effort
		// flip status.
		slog.Warn("import: persist failures partial",
			slog.String("job_id", job.ID.String()),
			slog.String("err", err.Error()))
	}
	completedAt := s.now()
	if err := s.repo.SetStatus(ctx, job.ID, StatusCompleted, nil, &completedAt); err != nil {
		slog.Warn("import: flip to completed failed",
			slog.String("job_id", job.ID.String()),
			slog.String("err", err.Error()))
	}
	job.Status = StatusCompleted
	job.CompletedAt = &completedAt
	job.SuccessCount = len(creds)
	job.FailCount = len(failures)
	job.CredentialsCSV = &credKey

	return &ConfirmResult{
		Job:                  job,
		SuccessCount:         len(creds),
		FailCount:            len(failures),
		CredentialsObjectKey: credKey,
		Failures:             failures,
	}, nil
}

// CredentialsTTL is the lifetime of a generated credentials.csv after the
// import job moves to completed. After this window the cleanup cron (Task
// 2.D.6) deletes the R2 object and admins must re-import. Locked decision:
// 1 hour, conservative since the file contains plaintext passwords.
const CredentialsTTL = 1 * time.Hour

// DownloadCredentialsResult is the public response from DownloadCredentials.
type DownloadCredentialsResult struct {
	Job              *ImportJob
	URL              string        // presigned GET URL with attachment Content-Disposition
	ObjectKey        string        // R2 object key (for audit logging)
	Filename         string        // suggested attachment filename ("credentials-<job_id>.csv")
	TTL              time.Duration // how long the URL stays valid
	ExpiresAt        time.Time     // CompletedAt + CredentialsTTL — used by FE to disable button
}

// DownloadCredentials issues a presigned GET URL for an admin's
// credentials.csv. Lifecycle requirements:
//   - Job exists + owned by admin → else ErrJobNotFound (404)
//   - Status == completed         → else ErrJobNotCompleted (409)
//   - CompletedAt + CredentialsTTL not elapsed → else ErrCredentialsExpired (410)
//   - CredentialsCSV key set + R2 object still exists → else ErrCredentialsMissing (404)
//
// The presigned URL embeds Content-Disposition: attachment;
// filename="credentials-<job_id>.csv" so browsers download instead of
// previewing. TTL defaults to s.PresignTTL (config-driven, 15m default).
func (s *Service) DownloadCredentials(ctx context.Context, id, adminID uuid.UUID) (*DownloadCredentialsResult, error) {
	job, err := s.repo.FindByIDForAdmin(ctx, id, adminID)
	if err != nil {
		return nil, ErrJobNotFound
	}
	if job.Status != StatusCompleted {
		return nil, ErrJobNotCompleted
	}
	if job.CompletedAt == nil {
		// Defensive: completed jobs should always have CompletedAt set
		// (Confirm sets it). If not, treat as expired so admin re-runs.
		return nil, ErrCredentialsExpired
	}
	expiresAt := job.CompletedAt.Add(CredentialsTTL)
	if s.now().After(expiresAt) {
		return nil, ErrCredentialsExpired
	}
	if job.CredentialsCSV == nil || *job.CredentialsCSV == "" {
		return nil, ErrCredentialsMissing
	}
	objectKey := *job.CredentialsCSV

	filename := fmt.Sprintf("credentials-%s.csv", job.ID.String())
	ttl := s.presignTTL
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	url, err := s.store.PresignGetDownload(ctx, objectKey, ttl, filename)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			// R2 lost the object (cleanup cron raced us, or operator
			// purged the bucket). Match what the FE expects when the TTL
			// gate would have caught it.
			return nil, ErrCredentialsMissing
		}
		return nil, fmt.Errorf("%w: presign %s: %v", ErrInternalDownload, objectKey, err)
	}

	return &DownloadCredentialsResult{
		Job:       job,
		URL:       url,
		ObjectKey: objectKey,
		Filename:  filename,
		TTL:       ttl,
		ExpiresAt: expiresAt,
	}, nil
}

// uploadCredentials renders the credentials CSV and PutObjects it into R2.
// Returns the object key on success.
func (s *Service) uploadCredentials(ctx context.Context, jobID uuid.UUID, creds []credentialRow) (string, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"email", "password", "kode_kelas", "nama_kelas"}); err != nil {
		return "", err
	}
	for _, c := range creds {
		if err := w.Write([]string{c.Email, c.Plain, c.KodeKelas, c.KelasNama}); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}

	key, err := storage.BuildKey(storage.CategoryCredentials, jobID.String()+".csv")
	if err != nil {
		return "", fmt.Errorf("build credentials key: %w", err)
	}
	body := buf.Bytes()
	if err := s.store.PutObject(ctx, storage.PutObjectInput{
		Key:         key,
		Body:        bytes.NewReader(body),
		Size:        int64(len(body)),
		ContentType: "text/csv; charset=utf-8",
	}); err != nil {
		return "", err
	}
	return key, nil
}

// persistFailures writes counts + errors_json + credentials_csv path. Each
// step is best-effort: if one fails the others still try.
func (s *Service) persistFailures(ctx context.Context, jobID uuid.UUID, success, fail int, failures []ConfirmFailure, credKey string) error {
	var firstErr error
	if err := s.repo.SetCounts(ctx, jobID, success, fail); err != nil && firstErr == nil {
		firstErr = err
	}
	if len(failures) > 0 {
		blob, mErr := json.Marshal(failures)
		if mErr != nil && firstErr == nil {
			firstErr = mErr
		} else if mErr == nil {
			if err := s.repo.SetErrorsJSON(ctx, jobID, blob); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if credKey != "" {
		if err := s.repo.SetCredentialsPath(ctx, jobID, credKey); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ptrTime returns a pointer to t. Tiny helper to avoid `tt := s.now(); &tt`
// boilerplate at the SetStatus call sites.
func ptrTime(t time.Time) *time.Time { return &t }

// readAllAndClose drains the body of a storage.Object, then closes it.
func readAllAndClose(obj *storage.Object) ([]byte, error) {
	if obj == nil || obj.Body == nil {
		return nil, errors.New("nil body")
	}
	defer obj.Body.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(obj.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// isDuplicateEmail tries to detect Postgres unique-violation on users.email
// without coupling the importjob package to the pg driver. The error string
// from pgx / lib/pq contains "duplicate key value" + "users_email_key".
func isDuplicateEmail(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") && strings.Contains(msg, "email")
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
