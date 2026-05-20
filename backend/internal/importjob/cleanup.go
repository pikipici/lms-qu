// Cleanup loop for ImportJob domain (Task 2.D.6).
//
// Two sweeps run on a 1h cadence:
//
//  1. Preview expiry: jobs in status=preview whose ExpiresAt < now are
//     flipped to status=expired and their R2 raw CSV (`import/<uuid>.csv`)
//     is best-effort deleted. Repo handles the status flip transactionally;
//     R2 deletes happen outside the transaction so a slow S3 round-trip
//     never holds row-level locks.
//
//  2. Credentials eviction: jobs in status=completed whose CompletedAt + 1h
//     has elapsed have their R2 credentials.csv blob deleted and the
//     credentials_csv column nulled out. Status stays at completed (we only
//     evict the download handle). DownloadCredentials starts returning
//     ErrCredentialsMissing for these rows once eviction lands.
//
// All R2 errors are logged via slog.Warn but never abort the loop — the
// next tick retries. The cleaner runs in its own goroutine started from
// main.go and exits when the parent context is cancelled (graceful
// shutdown on SIGINT/SIGTERM).
package importjob

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/pikip/lms/backend/internal/storage"
)

// CleanupInterval is the cadence at which RunCleanup ticks. Exported so a
// future test/admin endpoint could trigger an out-of-band sweep.
const CleanupInterval = 1 * time.Hour

// cleanupRepo is the subset of *Repo used by Cleaner. Defined as an
// interface so tests can stub without standing up GORM.
type cleanupRepo interface {
	ExpirePreviewBefore(ctx context.Context, cutoff time.Time) ([]ImportJob, error)
	ExpireCredentialsBefore(ctx context.Context, cutoff time.Time) ([]ImportJob, error)
}

// Cleaner runs the periodic ImportJob cleanup sweeps. Build with NewCleaner,
// inject from main.go, then call Run from a goroutine.
type Cleaner struct {
	repo  cleanupRepo
	store storage.Storage
	now   func() time.Time
	// credentialsTTL is the lifetime of credentials.csv after CompletedAt.
	// Defaults to CredentialsTTL (1h); override via SetCredentialsTTL for
	// tests or future tuning.
	credentialsTTL time.Duration
}

// NewCleaner constructs a Cleaner. repo + store must be non-nil; tests may
// pass an in-memory MockStorage and a stub repo.
func NewCleaner(repo cleanupRepo, store storage.Storage) *Cleaner {
	return &Cleaner{
		repo:           repo,
		store:          store,
		now:            time.Now,
		credentialsTTL: CredentialsTTL,
	}
}

// SetClock overrides the time source. Tests use this to simulate elapsed
// TTLs without sleeping.
func (c *Cleaner) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	c.now = now
}

// SetCredentialsTTL overrides the credentials.csv lifetime. <=0 keeps the
// existing value.
func (c *Cleaner) SetCredentialsTTL(d time.Duration) {
	if d <= 0 {
		return
	}
	c.credentialsTTL = d
}

// SweepResult is the per-tick outcome of one cleanup pass. Returned by
// RunOnce so callers (tests, future admin endpoint) can assert on the
// counts. RunCleanup discards the result.
type SweepResult struct {
	PreviewExpired       int // rows flipped preview -> expired
	PreviewObjectsDel    int // R2 import/* objects successfully deleted
	PreviewObjectsErr    int // R2 import/* deletes that errored (logged)
	CredentialsEvicted   int // rows whose credentials_csv was nulled
	CredentialsObjectsDel int // R2 credentials/* objects successfully deleted
	CredentialsObjectsErr int // R2 credentials/* deletes that errored (logged)
}

// Run starts the ticker loop. Blocks until ctx is cancelled. Safe to call
// from a goroutine; first sweep fires immediately so a freshly-restarted
// process catches stale jobs without waiting an hour.
func (c *Cleaner) Run(ctx context.Context) {
	if c == nil || c.repo == nil {
		slog.Warn("importjob cleanup: cleaner not configured, exit")
		return
	}
	// Kick off an initial sweep so a freshly-deployed binary doesn't leave
	// stale rows + R2 objects sitting for a full hour.
	if _, err := c.RunOnce(ctx); err != nil {
		slog.Warn("importjob cleanup: initial sweep error",
			slog.String("err", err.Error()))
	}

	t := time.NewTicker(CleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("importjob cleanup: shutdown")
			return
		case <-t.C:
			if _, err := c.RunOnce(ctx); err != nil {
				slog.Warn("importjob cleanup: sweep error",
					slog.String("err", err.Error()))
			}
		}
	}
}

// RunOnce executes both sweeps once and returns aggregate counts. Logs
// per-sweep summaries via slog. Errors from one sweep do NOT prevent the
// other from running — we always try both before returning.
func (c *Cleaner) RunOnce(ctx context.Context) (SweepResult, error) {
	now := c.now()
	var res SweepResult

	// 1. Preview expiry.
	previewRows, previewErr := c.repo.ExpirePreviewBefore(ctx, now)
	if previewErr != nil {
		slog.Warn("importjob cleanup: preview expiry repo error",
			slog.String("err", previewErr.Error()))
	} else {
		res.PreviewExpired = len(previewRows)
		for _, row := range previewRows {
			if row.ObjectKey == nil || *row.ObjectKey == "" {
				continue
			}
			// Use background context so a cancellation between status
			// flip and DeleteObject doesn't leave the loop half-done.
			if err := c.store.DeleteObject(context.Background(), *row.ObjectKey); err != nil {
				res.PreviewObjectsErr++
				slog.Warn("importjob cleanup: preview r2 delete failed",
					slog.String("job_id", row.ID.String()),
					slog.String("object_key", *row.ObjectKey),
					slog.String("err", err.Error()))
				continue
			}
			res.PreviewObjectsDel++
		}
		if res.PreviewExpired > 0 {
			slog.Info("importjob cleanup: preview swept",
				slog.Int("expired", res.PreviewExpired),
				slog.Int("r2_deleted", res.PreviewObjectsDel),
				slog.Int("r2_errors", res.PreviewObjectsErr))
		}
	}

	// 2. Credentials eviction.
	credCutoff := now.Add(-c.credentialsTTL)
	credRows, credErr := c.repo.ExpireCredentialsBefore(ctx, credCutoff)
	if credErr != nil {
		slog.Warn("importjob cleanup: credentials evict repo error",
			slog.String("err", credErr.Error()))
	} else {
		res.CredentialsEvicted = len(credRows)
		for _, row := range credRows {
			if row.CredentialsCSV == nil || *row.CredentialsCSV == "" {
				continue
			}
			if err := c.store.DeleteObject(context.Background(), *row.CredentialsCSV); err != nil {
				res.CredentialsObjectsErr++
				slog.Warn("importjob cleanup: credentials r2 delete failed",
					slog.String("job_id", row.ID.String()),
					slog.String("object_key", *row.CredentialsCSV),
					slog.String("err", err.Error()))
				continue
			}
			res.CredentialsObjectsDel++
		}
		if res.CredentialsEvicted > 0 {
			slog.Info("importjob cleanup: credentials swept",
				slog.Int("evicted", res.CredentialsEvicted),
				slog.Int("r2_deleted", res.CredentialsObjectsDel),
				slog.Int("r2_errors", res.CredentialsObjectsErr))
		}
	}

	return res, errors.Join(previewErr, credErr)
}
