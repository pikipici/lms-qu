// Package health implements liveness (/healthz) and readiness (/readyz)
// endpoints (locked decision #44).
//
//   - /healthz: process is alive. No DB. Always 200 if the binary is running.
//   - /readyz : DB ping + object storage probe (R2 HeadBucket cached 30s, or
//     local-disk writability fallback for legacy compat). Returns 503 with
//     details if any check fails. Loadbalancers / uptime monitors should hit
//     this.
package health

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"github.com/pikip/lms/backend/internal/storage"
	"gorm.io/gorm"
)

// r2Prober is implemented by storage.R2Client. Defined locally so we don't
// depend on a concrete type and so MockStorage / unconfigured environments
// can simply not implement it.
type r2Prober interface {
	HeadBucket(ctx context.Context) error
	Bucket() string
}

// Handler bundles the dependencies needed for readyz checks.
type Handler struct {
	Cfg     *config.Config
	DB      *gorm.DB
	Storage storage.Storage // optional; nil = skip object-store probe

	// HeadBucket result cache (locked decision #44 + 2.D.0.b: cache 30s).
	mu             sync.Mutex
	r2LastOK       time.Time
	r2LastErr      error
	r2LastFailures int
}

const (
	r2CacheTTL          = 30 * time.Second
	r2FailureThreshold  = 2 // consecutive HeadBucket failures before reporting fail
	r2HeadBucketTimeout = 5 * time.Second
)

// Liveness — minimal, no dependencies. Used by systemd / container probes.
func (h *Handler) Liveness(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Readiness — verifies the service is actually able to serve traffic.
// Currently checks DB ping + object storage (R2 HeadBucket cached, or local
// disk writability for legacy fallback).
func (h *Handler) Readiness(c *fiber.Ctx) error {
	checks := map[string]string{}
	overall := true

	if err := db.Ping(c.UserContext(), h.DB); err != nil {
		checks["database"] = "fail: " + err.Error()
		overall = false
	} else {
		checks["database"] = "ok"
	}

	storageStatus, storageOK := h.checkStorage(c.UserContext())
	checks["storage"] = storageStatus
	if !storageOK {
		overall = false
	}

	body := fiber.Map{
		"status": "ready",
		"checks": checks,
	}

	if !overall {
		body["status"] = "not_ready"
		return c.Status(fiber.StatusServiceUnavailable).JSON(body)
	}
	return c.JSON(body)
}

// checkStorage probes the object-store backend. R2 results are cached for
// r2CacheTTL to avoid hammering Cloudflare on every readyz hit. Two
// consecutive failures are required before reporting fail (transient
// Cloudflare blips shouldn't flap the readyz signal).
func (h *Handler) checkStorage(ctx context.Context) (string, bool) {
	if probe, ok := h.Storage.(r2Prober); ok {
		return h.checkR2(ctx, probe)
	}
	// Legacy local-disk fallback (or MockStorage in dev).
	if err := storageWritable(ctx, h.Cfg.Storage.Dir); err != nil {
		return "fail: " + err.Error(), false
	}
	return "ok", true
}

func (h *Handler) checkR2(ctx context.Context, probe r2Prober) (string, bool) {
	h.mu.Lock()
	cached := h.r2LastOK
	failures := h.r2LastFailures
	h.mu.Unlock()

	// Fresh cached OK — short-circuit.
	if !cached.IsZero() && time.Since(cached) < r2CacheTTL && failures == 0 {
		return fmt.Sprintf("ok (r2:%s, cached)", probe.Bucket()), true
	}

	probeCtx, cancel := context.WithTimeout(ctx, r2HeadBucketTimeout)
	defer cancel()
	err := probe.HeadBucket(probeCtx)

	h.mu.Lock()
	defer h.mu.Unlock()
	if err == nil {
		h.r2LastOK = time.Now()
		h.r2LastErr = nil
		h.r2LastFailures = 0
		return fmt.Sprintf("ok (r2:%s)", probe.Bucket()), true
	}

	h.r2LastFailures = failures + 1
	h.r2LastErr = err

	// First failure with a recent cached OK -> transient, still ready.
	if h.r2LastFailures < r2FailureThreshold && !cached.IsZero() && time.Since(cached) < r2CacheTTL*2 {
		return fmt.Sprintf("ok (r2:%s, transient err: %s, cached)", probe.Bucket(), err), true
	}
	return fmt.Sprintf("fail: r2 head_bucket (%d consecutive): %s", h.r2LastFailures, err.Error()), false
}

// storageWritable ensures the upload root exists and is writable. We use a
// short-lived sentinel file so we don't need O_TMPFILE semantics. Used as
// a legacy fallback when the object store does not implement r2Prober
// (e.g. dev MockStorage).
func storageWritable(ctx context.Context, dir string) error {
	if dir == "" {
		return errors.New("storage dir is empty")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	probe := filepath.Join(dir, ".readyz-probe")
	if err := os.WriteFile(probe, []byte("probe"), 0o600); err != nil {
		return fmt.Errorf("write probe: %w", err)
	}
	_ = os.Remove(probe)

	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
