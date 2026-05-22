// Package storage abstracts the object storage backend used by the LMS.
//
// As of v0.8.0 the production backend is **Cloudflare R2** (S3-compatible,
// locked decision #61). Tests use an in-memory MockStorage. Local-disk
// helpers (Init, Path) are retained as deprecated for backward compat with
// startup wiring; new code MUST use the Storage interface.
//
// Locked decisions referenced:
//   - #58 Path/key convention: <kategori>/<uuid>.<ext>
//   - #61 R2 single bucket per env, prefix per kategori
//   - #62 Upload via backend (multipart -> validate -> PutObject); download
//     via presigned GET URL (TTL 15m, bucket non-public)
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Category names — keep in sync with locked decisions #58/#61.
const (
	CategoryTugas      = "tugas"
	CategorySoal       = "soal"
	CategoryMateri     = "materi"
	CategorySubmission = "submission"
	CategoryImport     = "import"
	// CategoryCredentials holds generated post-import credentials.csv blobs
	// (Task 2.D.4). Same admin-only scope as CategoryImport but separated so
	// cleanup/eviction policies and presigned-URL TTLs can diverge later.
	CategoryCredentials = "credentials"
	// CategoryBankSoal holds inline image slots for BankSoal rows
	// (Fase 6 / Task 6.B.2). Distinct from CategorySoal (which is the
	// SoalBab bab-tied bank from Fase 5) supaya cleanup cron + audit
	// per-fitur tidak nyampur. Locked decision #58 prefix per kategori.
	CategoryBankSoal = "soal-bank"
)

var validCategories = map[string]struct{}{
	CategoryTugas:       {},
	CategorySoal:        {},
	CategoryMateri:      {},
	CategorySubmission:  {},
	CategoryImport:      {},
	CategoryCredentials: {},
	CategoryBankSoal:    {},
}

// IsValidCategory reports whether name is one of the known kategori prefixes.
func IsValidCategory(name string) bool {
	_, ok := validCategories[name]
	return ok
}

// ErrObjectNotFound is returned by Get/Delete/Exists/PresignGet when the
// requested key does not exist in the backing store.
var ErrObjectNotFound = errors.New("storage: object not found")

// PutObjectInput describes a single upload call.
//
// Body is read fully by the implementation. If Size is >= 0, implementations
// MAY use it to optimize transfer (e.g. set Content-Length); a value of -1
// means "unknown, stream until EOF".
type PutObjectInput struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

// Object is a fetched object plus metadata. The caller MUST Close Body.
type Object struct {
	Key         string
	Size        int64
	ContentType string
	Body        io.ReadCloser
}

// Storage abstracts the underlying object store. Implementations:
//   - R2Client    (production, Cloudflare R2 via aws-sdk-go-v2; Task 2.D.0.b)
//   - MockStorage (tests + dev fallback when R2 is not configured)
//
// All methods take a context for cancellation/timeout. Errors should wrap
// ErrObjectNotFound when the requested key is missing.
type Storage interface {
	// PutObject uploads (or overwrites) an object at the given key.
	PutObject(ctx context.Context, in PutObjectInput) error

	// GetObject fetches the object body + metadata. Caller MUST Close Body.
	// Returns ErrObjectNotFound if the key does not exist.
	GetObject(ctx context.Context, key string) (*Object, error)

	// DeleteObject removes the object at the given key. Idempotent: a
	// missing key is NOT an error.
	DeleteObject(ctx context.Context, key string) error

	// CopyObject server-side copies the object at srcKey to dstKey within
	// the same bucket. Used by duplicate flows (tugas/bab) so we don't
	// have to GET+PUT the entire body through the LMS process. Returns
	// ErrObjectNotFound if srcKey does not exist.
	CopyObject(ctx context.Context, srcKey, dstKey string) error

	// ObjectExists reports whether the key currently exists.
	ObjectExists(ctx context.Context, key string) (bool, error)

	// PresignGet returns a time-bounded URL the browser can use to GET the
	// object directly. ttl is clamped to a sane minimum/maximum by the
	// implementation. Returns ErrObjectNotFound if the key does not exist.
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)

	// PresignGetDownload is like PresignGet but the resulting URL forces the
	// browser to treat the response as an attachment with the given filename
	// (via ResponseContentDisposition on S3/R2). filename may be empty to
	// fall back to plain attachment without a name. Used by Task 2.D.5 so
	// admins downloading credentials.csv get a stable filename instead of
	// the uuid-based object key.
	PresignGetDownload(ctx context.Context, key string, ttl time.Duration, filename string) (string, error)
}

// BuildKey constructs an object key with the canonical "<kategori>/<...>"
// layout (locked decision #61). Validates kategori and rejects empty or
// traversal-shaped parts.
//
// Example:
//
//	BuildKey(CategoryImport, jobID, "preview.csv")
//	  -> "import/<uuid>/preview.csv"
func BuildKey(category string, parts ...string) (string, error) {
	if !IsValidCategory(category) {
		return "", fmt.Errorf("storage: unknown category %q", category)
	}
	if len(parts) == 0 {
		return "", errors.New("storage: BuildKey requires at least one part")
	}
	cleaned := make([]string, 0, len(parts)+1)
	cleaned = append(cleaned, category)
	for _, p := range parts {
		trimmed := strings.Trim(strings.TrimSpace(p), "/")
		if trimmed == "" {
			return "", errors.New("storage: empty key part")
		}
		if strings.Contains(trimmed, "..") || strings.ContainsAny(trimmed, "\\\x00") {
			return "", fmt.Errorf("storage: invalid key part %q", p)
		}
		cleaned = append(cleaned, trimmed)
	}
	return path.Join(cleaned...), nil
}

// --- legacy local-disk helpers (deprecated; kept for backward compat) ---

// Init ensures a per-category subdirectory layout exists under root.
//
// Deprecated: as of v0.8.0 LMS uses Cloudflare R2 for all uploads (locked
// #61). This helper is retained only so existing callers (cmd/server) keep
// compiling during the migration window. New code MUST use Storage.PutObject.
func Init(root string) error {
	if root == "" {
		return errors.New("storage: root is empty")
	}
	for cat := range validCategories {
		if err := os.MkdirAll(filepath.Join(root, cat), 0o750); err != nil {
			return fmt.Errorf("storage: mkdir %s: %w", cat, err)
		}
	}
	return nil
}

// Path returns the legacy on-disk path for category/name.
//
// Deprecated: see Init. Use BuildKey + Storage.PutObject for new code.
func Path(root, category, name string) (string, error) {
	if !IsValidCategory(category) {
		return "", fmt.Errorf("storage: unknown category %q", category)
	}
	if name == "" {
		return "", errors.New("storage: empty filename")
	}
	return filepath.Join(root, category, name), nil
}
