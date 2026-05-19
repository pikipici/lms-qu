// Package health implements liveness (/healthz) and readiness (/readyz)
// endpoints (locked decision #44).
//
//   - /healthz: process is alive. No DB. Always 200 if the binary is running.
//   - /readyz : DB ping + storage dir writable. Returns 503 with details if
//     any check fails. Loadbalancers / uptime monitors should hit this.
package health

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"gorm.io/gorm"
)

// Handler bundles the dependencies needed for readyz checks.
type Handler struct {
	Cfg *config.Config
	DB  *gorm.DB
}

// Liveness — minimal, no dependencies. Used by systemd / container probes.
func (h *Handler) Liveness(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Readiness — verifies the service is actually able to serve traffic.
// Currently checks DB ping + storage dir writable.
func (h *Handler) Readiness(c *fiber.Ctx) error {
	checks := map[string]string{}
	overall := true

	if err := db.Ping(c.UserContext(), h.DB); err != nil {
		checks["database"] = "fail: " + err.Error()
		overall = false
	} else {
		checks["database"] = "ok"
	}

	if err := storageWritable(c.UserContext(), h.Cfg.Storage.Dir); err != nil {
		checks["storage"] = "fail: " + err.Error()
		overall = false
	} else {
		checks["storage"] = "ok"
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

// storageWritable ensures the upload root exists and is writable. We use a
// short-lived sentinel file so we don't need O_TMPFILE semantics.
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

	// Honour ctx cancellation for safety even though writes are sync above.
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
