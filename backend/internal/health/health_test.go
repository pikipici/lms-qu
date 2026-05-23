package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/storage"
)

type fakeR2Storage struct {
	err   error
	calls int
}

func (f *fakeR2Storage) HeadBucket(context.Context) error {
	f.calls++
	return f.err
}

func (f *fakeR2Storage) Bucket() string { return "test-bucket" }

func (f *fakeR2Storage) PutObject(context.Context, storage.PutObjectInput) error    { return nil }
func (f *fakeR2Storage) GetObject(context.Context, string) (*storage.Object, error) { return nil, nil }
func (f *fakeR2Storage) DeleteObject(context.Context, string) error                 { return nil }
func (f *fakeR2Storage) CopyObject(context.Context, string, string) error           { return nil }
func (f *fakeR2Storage) ObjectExists(context.Context, string) (bool, error)         { return false, nil }
func (f *fakeR2Storage) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", nil
}
func (f *fakeR2Storage) PresignGetDownload(context.Context, string, time.Duration, string) (string, error) {
	return "", nil
}

func TestLivenessReturnsOK(t *testing.T) {
	app := fiber.New()
	h := &Handler{}
	app.Get("/healthz", h.Liveness)

	resp, err := app.Test(httptest.NewRequest("GET", "/healthz", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %v, want ok", body["status"])
	}
	if _, err := time.Parse(time.RFC3339, body["time"].(string)); err != nil {
		t.Fatalf("time is not RFC3339: %v", body["time"])
	}
}

func TestReadinessFailsWhenDatabaseIsMissing(t *testing.T) {
	app := fiber.New()
	h := &Handler{Cfg: &config.Config{Storage: config.StorageConfig{Dir: t.TempDir()}}}
	app.Get("/readyz", h.Readiness)

	resp, err := app.Test(httptest.NewRequest("GET", "/readyz", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusServiceUnavailable)
	}

	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "not_ready" {
		t.Fatalf("body status = %q, want not_ready", body.Status)
	}
	if !strings.Contains(body.Checks["database"], "db: not initialised") {
		t.Fatalf("database check = %q, want db not initialised", body.Checks["database"])
	}
	if body.Checks["storage"] != "ok" {
		t.Fatalf("storage check = %q, want ok", body.Checks["storage"])
	}
}

func TestCheckStorageLocalDiskFallback(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Cfg: &config.Config{Storage: config.StorageConfig{Dir: dir}}}

	status, ok := h.checkStorage(context.Background())
	if !ok || status != "ok" {
		t.Fatalf("checkStorage = (%q, %v), want ok true", status, ok)
	}
}

func TestCheckStorageLocalDiskFallbackFailsForEmptyDir(t *testing.T) {
	h := &Handler{Cfg: &config.Config{Storage: config.StorageConfig{Dir: ""}}}

	status, ok := h.checkStorage(context.Background())
	if ok {
		t.Fatalf("checkStorage ok = true, want false")
	}
	if !strings.Contains(status, "storage dir is empty") {
		t.Fatalf("status = %q, want storage dir error", status)
	}
}

func TestCheckR2CachesSuccessfulProbe(t *testing.T) {
	probe := &fakeR2Storage{}
	h := &Handler{Storage: probe}

	status, ok := h.checkStorage(context.Background())
	if !ok || status != "ok (r2:test-bucket)" {
		t.Fatalf("first checkStorage = (%q, %v), want fresh r2 ok", status, ok)
	}
	status, ok = h.checkStorage(context.Background())
	if !ok || status != "ok (r2:test-bucket, cached)" {
		t.Fatalf("second checkStorage = (%q, %v), want cached r2 ok", status, ok)
	}
	if probe.calls != 1 {
		t.Fatalf("probe calls = %d, want 1", probe.calls)
	}
}

func TestCheckR2FailureThreshold(t *testing.T) {
	probe := &fakeR2Storage{err: errors.New("boom")}
	h := &Handler{Storage: probe}

	status, ok := h.checkStorage(context.Background())
	if ok {
		t.Fatalf("first failing check ok = true, want false without cached OK")
	}
	if !strings.Contains(status, "1 consecutive") {
		t.Fatalf("status = %q, want first failure count", status)
	}

	status, ok = h.checkStorage(context.Background())
	if ok {
		t.Fatalf("second failing check ok = true, want false")
	}
	if !strings.Contains(status, "2 consecutive") {
		t.Fatalf("status = %q, want second failure count", status)
	}
}

func TestCheckR2TreatsFirstFailureAfterRecentOKAsTransient(t *testing.T) {
	probe := &fakeR2Storage{}
	h := &Handler{Storage: probe}

	if status, ok := h.checkStorage(context.Background()); !ok || status != "ok (r2:test-bucket)" {
		t.Fatalf("initial checkStorage = (%q, %v), want ok", status, ok)
	}

	h.r2LastOK = time.Now().Add(-r2CacheTTL - time.Second)
	probe.err = errors.New("temporary r2 outage")
	status, ok := h.checkStorage(context.Background())
	if !ok {
		t.Fatalf("transient failure ok = false, status %q", status)
	}
	if !strings.Contains(status, "transient err") || !strings.Contains(status, "cached") {
		t.Fatalf("status = %q, want transient cached status", status)
	}
}
