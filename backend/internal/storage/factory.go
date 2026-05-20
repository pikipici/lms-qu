package storage

import (
	"context"
	"errors"
	"log"
	"strings"
)

// R2Config holds Cloudflare R2 connection settings (locked decision #61).
//
// Endpoint is derived as "https://<AccountID>.r2.cloudflarestorage.com" by
// the implementation; callers only need to populate AccountID + credentials
// + Bucket. PresignTTL applies to download URLs (locked default 15m, #62).
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PresignTTL      int // seconds; clamped to [60, 86400] at call site
}

// IsConfigured reports whether all required R2 fields are non-empty.
// Returns false on partial config — caller should treat that as
// "not configured" and fall back to MockStorage in non-prod, or fail-fast
// in prod.
func (c R2Config) IsConfigured() bool {
	return strings.TrimSpace(c.AccountID) != "" &&
		strings.TrimSpace(c.AccessKeyID) != "" &&
		strings.TrimSpace(c.SecretAccessKey) != "" &&
		strings.TrimSpace(c.Bucket) != ""
}

// FactoryOptions tunes NewStorage behavior.
type FactoryOptions struct {
	// AllowMockFallback: when true and R2Config.IsConfigured() == false,
	// NewStorage returns an in-memory MockStorage with a warning log
	// (suitable for dev/CI). When false, NewStorage returns an error.
	AllowMockFallback bool

	// Logger is the optional logger used to emit fallback warnings.
	// nil = use standard log package.
	Logger *log.Logger
}

// NewStorage constructs a Storage implementation according to cfg.
//
// Behavior matrix:
//
//	cfg.IsConfigured() == true   -> R2Client (real Cloudflare R2 via aws-sdk-go-v2).
//	cfg.IsConfigured() == false:
//	    AllowMockFallback = true  -> MockStorage + warning.
//	    AllowMockFallback = false -> error.
func NewStorage(cfg R2Config, opts FactoryOptions) (Storage, error) {
	if cfg.IsConfigured() {
		return NewR2Client(context.Background(), cfg)
	}

	if !opts.AllowMockFallback {
		return nil, errors.New("storage: R2 not configured (missing one of R2_ACCOUNT_ID / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY / R2_BUCKET)")
	}

	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("storage: R2 not configured — using in-memory MockStorage (dev fallback). DO NOT USE IN PRODUCTION.")
	return NewMockStorage(), nil
}

// ErrR2NotImplemented is retained for API compat with callers that detected
// the transitional skeleton state in v0.8.0. With Task 2.D.0.b shipped this
// error should never be returned by NewStorage.
//
// Deprecated: 2.D.0.b shipped the real R2 client; this sentinel will be
// removed in a future release.
var ErrR2NotImplemented = errors.New("storage: R2 client not yet implemented (deprecated, see Task 2.D.0.b)")
