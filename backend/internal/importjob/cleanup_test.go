package importjob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/storage"
)

// stubCleanupRepo is a minimal cleanupRepo that records the cutoffs passed
// in and returns canned rows. Each function may also error to exercise
// failure paths.
type stubCleanupRepo struct {
	previewRows []ImportJob
	previewErr  error
	previewCuts []time.Time

	credRows []ImportJob
	credErr  error
	credCuts []time.Time
}

func (s *stubCleanupRepo) ExpirePreviewBefore(_ context.Context, cutoff time.Time) ([]ImportJob, error) {
	s.previewCuts = append(s.previewCuts, cutoff)
	if s.previewErr != nil {
		return nil, s.previewErr
	}
	return s.previewRows, nil
}

func (s *stubCleanupRepo) ExpireCredentialsBefore(_ context.Context, cutoff time.Time) ([]ImportJob, error) {
	s.credCuts = append(s.credCuts, cutoff)
	if s.credErr != nil {
		return nil, s.credErr
	}
	return s.credRows, nil
}

// flakyStorage wraps a MockStorage and lets tests inject a per-key delete
// error so we can assert that the cleaner counts errors without aborting.
type flakyStorage struct {
	*storage.MockStorage
	mu        sync.Mutex
	failKey   string
	failErr   error
	deletedOK []string
}

func newFlakyStorage(failKey string, failErr error) *flakyStorage {
	return &flakyStorage{
		MockStorage: storage.NewMockStorage(),
		failKey:     failKey,
		failErr:     failErr,
	}
}

func (f *flakyStorage) DeleteObject(ctx context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if key == f.failKey {
		return f.failErr
	}
	f.deletedOK = append(f.deletedOK, key)
	return f.MockStorage.DeleteObject(ctx, key)
}

func ptrStr(s string) *string { return &s }

func newPreviewRow(t *testing.T, key string) ImportJob {
	t.Helper()
	return ImportJob{
		ID:        uuid.New(),
		Status:    StatusPreview,
		ObjectKey: ptrStr(key),
	}
}

func newCompletedRow(t *testing.T, credKey string) ImportJob {
	t.Helper()
	completed := time.Now().Add(-2 * time.Hour)
	return ImportJob{
		ID:             uuid.New(),
		Status:         StatusCompleted,
		CredentialsCSV: ptrStr(credKey),
		CompletedAt:    &completed,
	}
}

// --- Tests ---

// TestCleaner_RunOnce_PreviewHappy verifies a clean preview sweep flips
// the row + deletes its R2 object + returns counts.
func TestCleaner_RunOnce_PreviewHappy(t *testing.T) {
	store := storage.NewMockStorage()
	rawKey := "import/abc.csv"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: rawKey, Body: emptyReader(), Size: 0, ContentType: "text/csv",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	row := newPreviewRow(t, rawKey)
	repo := &stubCleanupRepo{previewRows: []ImportJob{row}}
	c := NewCleaner(repo, store)
	c.SetClock(func() time.Time { return time.Now() })

	res, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.PreviewExpired != 1 {
		t.Errorf("PreviewExpired = %d, want 1", res.PreviewExpired)
	}
	if res.PreviewObjectsDel != 1 {
		t.Errorf("PreviewObjectsDel = %d, want 1", res.PreviewObjectsDel)
	}
	if res.PreviewObjectsErr != 0 {
		t.Errorf("PreviewObjectsErr = %d, want 0", res.PreviewObjectsErr)
	}
	if exists, _ := store.ObjectExists(context.Background(), rawKey); exists {
		t.Errorf("R2 object %q still present after sweep", rawKey)
	}
	if len(repo.previewCuts) != 1 {
		t.Errorf("previewCuts = %d, want 1", len(repo.previewCuts))
	}
}

// TestCleaner_RunOnce_PreviewNoRows is the typical idle tick: no preview
// has expired, repo returns zero rows. Should be a no-op without error.
func TestCleaner_RunOnce_PreviewNoRows(t *testing.T) {
	store := storage.NewMockStorage()
	repo := &stubCleanupRepo{previewRows: nil}
	c := NewCleaner(repo, store)

	res, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.PreviewExpired != 0 || res.PreviewObjectsDel != 0 {
		t.Errorf("got %+v, want all zero", res)
	}
}

// TestCleaner_RunOnce_CredentialsHappy verifies completed-job credentials
// eviction: cutoff is now-TTL and the canned row's credentials.csv key is
// deleted from R2.
func TestCleaner_RunOnce_CredentialsHappy(t *testing.T) {
	store := storage.NewMockStorage()
	credKey := "credentials/xyz.csv"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: credKey, Body: emptyReader(), Size: 0, ContentType: "text/csv",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	row := newCompletedRow(t, credKey)
	repo := &stubCleanupRepo{credRows: []ImportJob{row}}
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	c := NewCleaner(repo, store)
	c.SetClock(func() time.Time { return now })

	res, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if res.CredentialsEvicted != 1 {
		t.Errorf("CredentialsEvicted = %d, want 1", res.CredentialsEvicted)
	}
	if res.CredentialsObjectsDel != 1 {
		t.Errorf("CredentialsObjectsDel = %d, want 1", res.CredentialsObjectsDel)
	}
	if res.CredentialsObjectsErr != 0 {
		t.Errorf("CredentialsObjectsErr = %d, want 0", res.CredentialsObjectsErr)
	}
	if exists, _ := store.ObjectExists(context.Background(), credKey); exists {
		t.Errorf("R2 object %q still present after sweep", credKey)
	}
	if len(repo.credCuts) != 1 {
		t.Errorf("credCuts = %d, want 1", len(repo.credCuts))
	}
	wantCut := now.Add(-CredentialsTTL)
	if !repo.credCuts[0].Equal(wantCut) {
		t.Errorf("credCuts[0] = %v, want %v", repo.credCuts[0], wantCut)
	}
}

// TestCleaner_RunOnce_CredentialsDeleteError verifies a single R2 delete
// failure is counted but does NOT abort the loop — other rows still get
// processed, sweep returns no error itself (repo error is the only thing
// we surface).
func TestCleaner_RunOnce_CredentialsDeleteError(t *testing.T) {
	flakeKey := "credentials/flake.csv"
	okKey := "credentials/ok.csv"
	store := newFlakyStorage(flakeKey, errors.New("r2 throttled"))
	for _, k := range []string{flakeKey, okKey} {
		if err := store.MockStorage.PutObject(context.Background(), storage.PutObjectInput{
			Key: k, Body: emptyReader(), Size: 0, ContentType: "text/csv",
		}); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}

	rows := []ImportJob{newCompletedRow(t, flakeKey), newCompletedRow(t, okKey)}
	repo := &stubCleanupRepo{credRows: rows}
	c := NewCleaner(repo, store)

	res, err := c.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: unexpected err: %v", err)
	}
	if res.CredentialsEvicted != 2 {
		t.Errorf("CredentialsEvicted = %d, want 2", res.CredentialsEvicted)
	}
	if res.CredentialsObjectsDel != 1 {
		t.Errorf("CredentialsObjectsDel = %d, want 1", res.CredentialsObjectsDel)
	}
	if res.CredentialsObjectsErr != 1 {
		t.Errorf("CredentialsObjectsErr = %d, want 1", res.CredentialsObjectsErr)
	}
	if exists, _ := store.ObjectExists(context.Background(), okKey); exists {
		t.Errorf("R2 object %q (ok path) still present after sweep", okKey)
	}
}

// TestCleaner_RunOnce_RepoError verifies that a repo error from one sweep
// is surfaced via errors.Join and does NOT prevent the OTHER sweep from
// running.
func TestCleaner_RunOnce_RepoError(t *testing.T) {
	store := storage.NewMockStorage()
	credKey := "credentials/recover.csv"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: credKey, Body: emptyReader(), Size: 0, ContentType: "text/csv",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	previewBoom := errors.New("preview db down")
	repo := &stubCleanupRepo{
		previewErr: previewBoom,
		credRows:   []ImportJob{newCompletedRow(t, credKey)},
	}
	c := NewCleaner(repo, store)

	res, err := c.RunOnce(context.Background())
	if err == nil || !errors.Is(err, previewBoom) {
		t.Fatalf("RunOnce err = %v, want errors.Is preview boom", err)
	}
	// Credentials sweep must still have run despite the preview failure.
	if res.CredentialsEvicted != 1 {
		t.Errorf("CredentialsEvicted = %d, want 1 (cred sweep should run despite preview err)", res.CredentialsEvicted)
	}
	if res.CredentialsObjectsDel != 1 {
		t.Errorf("CredentialsObjectsDel = %d, want 1", res.CredentialsObjectsDel)
	}
}

// TestCleaner_Run_ContextCancel verifies the loop exits promptly when the
// parent context is cancelled. The initial sweep runs once.
func TestCleaner_Run_ContextCancel(t *testing.T) {
	store := storage.NewMockStorage()
	repo := &stubCleanupRepo{}
	c := NewCleaner(repo, store)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()
	// Give Run a moment to fire its initial sweep.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after context cancel")
	}
	// Initial sweep should have queried the preview table at least once.
	if len(repo.previewCuts) < 1 {
		t.Errorf("previewCuts = %d, want >=1 from initial sweep", len(repo.previewCuts))
	}
}

// emptyReader returns a zero-length Reader for PutObject calls.
func emptyReader() io.Reader { return bytes.NewReader(nil) }
