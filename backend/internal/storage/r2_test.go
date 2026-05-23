package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
	"time"
)

// r2ConfigFromEnv loads R2 settings for integration tests. Returns the
// config and a flag indicating whether all required fields are present.
func r2ConfigFromEnv(t *testing.T) (R2Config, bool) {
	t.Helper()
	cfg := R2Config{
		AccountID:       os.Getenv("R2_ACCOUNT_ID"),
		AccessKeyID:     os.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		Bucket:          os.Getenv("R2_BUCKET"),
		PresignTTL:      900,
	}
	return cfg, cfg.IsConfigured()
}

func TestR2Helpers(t *testing.T) {
	if sanitizeASCII("") != "download" {
		t.Fatal("empty filename should fallback to download")
	}
	if got := sanitizeASCII("laporan \"final\" é.pdf"); got != "laporan _final_ _.pdf" {
		t.Fatalf("sanitizeASCII = %q", got)
	}
	if got := sanitizeASCII("line\nbreak\\file.txt"); got != "line_break_file.txt" {
		t.Fatalf("sanitizeASCII controls = %q", got)
	}
	if got := urlPathEscape("laporan final é.csv"); got != "laporan%20final%20%C3%A9.csv" {
		t.Fatalf("urlPathEscape = %q", got)
	}
}

func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Fatal("nil error should not be not-found")
	}
	if !isNotFound(&smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}) {
		t.Fatal("NoSuchKey should be not-found")
	}
	if !isNotFound(&smithy.GenericAPIError{Code: "404", Message: "missing"}) {
		t.Fatal("404 should be not-found")
	}
	if isNotFound(&smithy.GenericAPIError{Code: "AccessDenied", Message: "denied"}) {
		t.Fatal("AccessDenied should not be not-found")
	}
	if isNotFound(errors.New("plain error")) {
		t.Fatal("plain error should not be not-found")
	}
}

// TestR2Client_Integration runs a full PutObject -> ObjectExists -> GetObject
// -> PresignGet -> DeleteObject roundtrip against a real Cloudflare R2 bucket.
//
// Gated by R2_INTEGRATION=1 to avoid hitting the network on every CI run.
// Required env: R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET.
func TestR2Client_Integration(t *testing.T) {
	if os.Getenv("R2_INTEGRATION") != "1" {
		t.Skip("set R2_INTEGRATION=1 to run R2 integration tests")
	}
	cfg, ok := r2ConfigFromEnv(t)
	if !ok {
		t.Skip("R2 env vars not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cli, err := NewR2Client(ctx, cfg)
	if err != nil {
		t.Fatalf("NewR2Client: %v", err)
	}

	// HeadBucket should succeed.
	if err := cli.HeadBucket(ctx); err != nil {
		t.Fatalf("HeadBucket: %v", err)
	}

	// Use a probe key namespaced under _probe/ so it doesn't collide with
	// real LMS data and is easy to identify if cleanup ever fails.
	key := "_probe/r2-int-" + time.Now().UTC().Format("20060102T150405.000000000")
	body := []byte("hermes-r2-integration-test")

	// PutObject.
	if err := cli.PutObject(ctx, PutObjectInput{
		Key:         key,
		Body:        bytes.NewReader(body),
		Size:        int64(len(body)),
		ContentType: "text/plain",
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	t.Cleanup(func() {
		_ = cli.DeleteObject(context.Background(), key)
	})

	// ObjectExists -> true.
	exists, err := cli.ObjectExists(ctx, key)
	if err != nil {
		t.Fatalf("ObjectExists: %v", err)
	}
	if !exists {
		t.Fatal("ObjectExists = false right after PutObject")
	}

	// GetObject -> body matches.
	got, err := cli.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer got.Body.Close()
	read, err := io.ReadAll(got.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(read, body) {
		t.Fatalf("body mismatch: got %q want %q", read, body)
	}
	if got.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", got.ContentType)
	}

	// PresignGet -> URL returns the body via plain HTTP.
	url, err := cli.PresignGet(ctx, key, 60*time.Second)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.HasPrefix(url, "https://") {
		t.Fatalf("PresignGet URL not https: %s", url)
	}
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("HTTP GET presigned: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("presigned status = %d, want 200", resp.StatusCode)
	}
	read2, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(read2, body) {
		t.Fatalf("presigned body mismatch: got %q want %q", read2, body)
	}

	// PresignGet on missing key -> ErrObjectNotFound.
	_, err = cli.PresignGet(ctx, "_probe/definitely-missing-"+time.Now().UTC().Format("20060102T150405"), 60*time.Second)
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("PresignGet missing = %v, want wraps ErrObjectNotFound", err)
	}

	// DeleteObject.
	if err := cli.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}

	// DeleteObject missing -> idempotent (no error).
	if err := cli.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject second (idempotent) = %v, want nil", err)
	}

	// ObjectExists after delete -> false.
	exists, err = cli.ObjectExists(ctx, key)
	if err != nil {
		t.Fatalf("ObjectExists after delete: %v", err)
	}
	if exists {
		t.Fatal("ObjectExists = true after delete")
	}

	// GetObject after delete -> ErrObjectNotFound.
	_, err = cli.GetObject(ctx, key)
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("GetObject after delete = %v, want wraps ErrObjectNotFound", err)
	}
}

// TestR2Client_BadCredsHeadBucket verifies error path: with bogus creds we
// should get a non-nil error from HeadBucket. Skipped unless R2_INTEGRATION=1
// because we don't want CI making outbound calls without explicit opt-in.
func TestR2Client_BadCredsHeadBucket(t *testing.T) {
	if os.Getenv("R2_INTEGRATION") != "1" {
		t.Skip("set R2_INTEGRATION=1 to run R2 integration tests")
	}
	realCfg, ok := r2ConfigFromEnv(t)
	if !ok {
		t.Skip("R2 env vars not configured")
	}
	bad := realCfg
	bad.SecretAccessKey = "definitely-wrong-secret-aaaaaaaaaaaaaaaa"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cli, err := NewR2Client(ctx, bad)
	if err != nil {
		t.Fatalf("NewR2Client: %v", err)
	}
	if err := cli.HeadBucket(ctx); err == nil {
		t.Fatal("HeadBucket with bad creds returned nil error, want auth failure")
	}
}
