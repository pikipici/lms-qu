// cmd/server boots the LMS API server (Go Fiber).
//
// Fase 0 scope:
//   - load config (TZ lock, DB conn, JWT secret, rate limit, storage)
//   - init storage dirs
//   - mount middleware: recover, request-id, logger, global rate limit
//   - mount /api/v1/healthz + /api/v1/readyz
//   - serve frontend/out static + SPA fallback
//   - graceful shutdown on SIGINT/SIGTERM
//
// Domain endpoints (auth, admin, kelas, bab, ...) join in Fase 1+.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/pikip/lms/backend/internal/admin"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"github.com/pikip/lms/backend/internal/health"
	"github.com/pikip/lms/backend/internal/importjob"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/storage"
	"gorm.io/gorm"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server exit", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	logger.Info("lms boot",
		slog.String("env", cfg.Env),
		slog.Int("port", cfg.Port),
		slog.String("tz", cfg.Timezone.String()),
		slog.String("frontend_dir", cfg.FrontendDir),
		slog.Bool("automigrate", cfg.AutoMigrate),
	)

	if err := storage.Init(cfg.Storage.Dir); err != nil {
		return err
	}

	// Object storage (Cloudflare R2, locked decision #61). When R2 creds are
	// missing in dev/CI we fall back to in-memory MockStorage so the server
	// still boots. Production deploys MUST set the R2_* env vars; we surface
	// the choice in startup logs and in /readyz.
	objectStore, err := storage.NewStorage(
		storage.R2Config{
			AccountID:       cfg.Storage.R2.AccountID,
			AccessKeyID:     cfg.Storage.R2.AccessKeyID,
			SecretAccessKey: cfg.Storage.R2.SecretAccessKey,
			Bucket:          cfg.Storage.R2.Bucket,
			PresignTTL:      cfg.Storage.R2.PresignTTLSec,
		},
		storage.FactoryOptions{AllowMockFallback: !cfg.IsProduction()},
	)
	if err != nil {
		return fmt.Errorf("storage: init: %w", err)
	}
	logger.Info("storage ready",
		slog.Bool("r2_configured", cfg.Storage.R2.Bucket != ""),
		slog.String("backend", fmt.Sprintf("%T", objectStore)),
	)

	// Pre-warm R2 connection: first HeadBucket can take 5-15s on hosts where
	// IPv6 is broken (happy-eyeballs fallback) or DNS resolvers are slow.
	// Doing it here, BEFORE app.Listen, ensures readyz returns cached-OK on
	// the systemd ExecStartPost probe instead of timing out.
	if probe, ok := objectStore.(interface {
		HeadBucket(context.Context) error
		Bucket() string
	}); ok {
		warmCtx, warmCancel := context.WithTimeout(context.Background(), 30*time.Second)
		t0 := time.Now()
		if err := probe.HeadBucket(warmCtx); err != nil {
			logger.Warn("storage: r2 prewarm failed (non-fatal)",
				slog.String("bucket", probe.Bucket()),
				slog.Duration("elapsed", time.Since(t0)),
				slog.String("err", err.Error()))
		} else {
			logger.Info("storage: r2 prewarm ok",
				slog.String("bucket", probe.Bucket()),
				slog.Duration("elapsed", time.Since(t0)))
		}
		warmCancel()
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gdb, closeDB, err := db.Open(rootCtx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()

	app := newFiberApp(cfg, logger)
	mountRoutes(app, cfg, gdb, objectStore)
	mountStatic(app, cfg, logger)

	addr := fmt.Sprintf(":%d", cfg.Port)
	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", slog.String("addr", addr))
		errCh <- app.Listen(addr)
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, fiber.ErrServiceUnavailable) {
			return fmt.Errorf("listen: %w", err)
		}
	}

	shutdownCtx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		logger.Warn("graceful shutdown failed", slog.String("err", err.Error()))
	}
	logger.Info("bye")
	return nil
}

func newLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func newFiberApp(cfg *config.Config, log *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:               "lms-api",
		ServerHeader:          "lms-api",
		DisableStartupMessage: true,
		BodyLimit:             cfg.Storage.MaxTugasFileMB * 1024 * 1024, // hard ceiling; per-route limits stricter
		ReadTimeout:           30 * time.Second,
		WriteTimeout:          60 * time.Second,
		IdleTimeout:           120 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			rid := middleware.RequestIDFromFiber(c)
			log.LogAttrs(c.UserContext(), slog.LevelWarn, "http error",
				slog.String("request_id", rid),
				slog.String("path", c.Path()),
				slog.Int("status", code),
				slog.String("err", err.Error()),
			)
			return c.Status(code).JSON(fiber.Map{
				"error":      err.Error(),
				"code":       fiberCode(code),
				"request_id": rid,
			})
		},
	})

	// Order matters (#57): recover -> request-id -> logger -> rate-limit -> cors.
	app.Use(middleware.Recover(log))
	app.Use(middleware.RequestID())
	app.Use(middleware.Logger(log))
	app.Use(middleware.GlobalRateLimit(cfg.RateLimit.GlobalPerMin))

	if len(cfg.CORS.AllowedOrigins) > 0 {
		app.Use(cors.New(cors.Config{
			AllowOrigins: strings.Join(cfg.CORS.AllowedOrigins, ","),
			AllowHeaders: "Origin, Content-Type, Accept, Authorization, " + middleware.HeaderRequestID,
		}))
	}
	return app
}

func mountRoutes(app *fiber.App, cfg *config.Config, gdb *gorm.DB, objectStore storage.Storage) {
	api := app.Group("/api/v1")

	hh := &health.Handler{Cfg: cfg, DB: gdb, Storage: objectStore}
	api.Get("/healthz", hh.Liveness)
	api.Get("/readyz", hh.Readiness)

	// Auth
	authRepo := auth.NewRepo(gdb)
	authSvc := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authSvc)

	authGroup := api.Group("/auth")
	authGroup.Post("/login",
		auth.LoginRateLimit(cfg.RateLimit.LoginPer15Min),
		authHandler.Login,
	)
	authGroup.Post("/refresh",
		auth.RefreshRateLimit(cfg.RateLimit.RefreshPerMin),
		authHandler.Refresh,
	)
	authGroup.Post("/logout", authHandler.Logout)

	// Protected routes (bearer + force-change-password gate).
	// ForceChangePassword whitelist lets through:
	//   GET  /api/v1/auth/me
	//   POST /api/v1/auth/change-password
	//   POST /api/v1/auth/logout
	//   POST /api/v1/auth/logout-all
	protected := authGroup.Group("",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
	)
	protected.Get("/me", authHandler.Me)
	protected.Post("/change-password", authHandler.ChangePassword)
	protected.Post("/logout-all", authHandler.LogoutAll)
	protected.Get("/sessions", authHandler.Sessions)

	adminHandler := admin.NewHandler(authRepo, cfg)
	adminGroup := api.Group("/admin",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin)),
	)
	adminUsers := adminGroup.Group("/users")
	adminUsers.Get("/", adminHandler.ListUsers)
	adminUsers.Post("/", adminHandler.CreateUser)
	adminUsers.Get("/:id", adminHandler.GetUser)
	adminUsers.Patch("/:id", adminHandler.UpdateUser)
	adminUsers.Delete("/:id", adminHandler.DeleteUser)
	adminUsers.Post("/:id/reset-password", adminHandler.ResetUserPassword)
	adminUsers.Post("/:id/suspend", adminHandler.SuspendUser)
	adminUsers.Post("/:id/unsuspend", adminHandler.UnsuspendUser)
	adminUsers.Post("/:id/unlock", adminHandler.UnlockUser)
	adminUsers.Post("/:id/role", adminHandler.ChangeUserRole)
	adminUsers.Get("/:id/sessions", adminHandler.ListTargetSessions)
	adminUsers.Post("/:id/revoke-sessions", adminHandler.RevokeTargetSessions)

	adminGroup.Get("/audit-log", adminHandler.ListAuditLog)
	adminGroup.Get("/login-attempts", adminHandler.ListLoginAttempts)

	// Kelas (Phase 2): guru manages own kelas, admin sees all.
	kelasRepo := kelas.NewRepo(gdb)
	kelasSvc := kelas.NewService(kelasRepo, authRepo, authRepo)
	kelasHandler := kelas.NewHandler(kelasSvc)

	// Admin bulk-enroll (Phase 2.C.2): assign multiple siswa directly into a
	// kelas without kode invite. Wired under /admin so it inherits the admin
	// role guard.
	adminEnrollHandler := admin.NewKelasEnrollHandler(authRepo, kelasRepo)
	adminGroup.Post("/kelas/:id/enroll", adminEnrollHandler.BulkEnroll)

	// Bulk-import CSV (Phase 2.D.2): admin uploads CSV → preview ImportJob.
	importRepo := importjob.NewRepo(gdb)
	importSvc := importjob.NewService(importRepo, objectStore, 0)
	importHandler := importjob.NewHandler(importSvc, authRepo)
	adminGroup.Post("/import-csv/upload", importHandler.PreviewUpload)
	adminGroup.Get("/import-csv/:job_id", importHandler.GetPreview)
	adminGroup.Post("/import-csv/:job_id/cancel", importHandler.Cancel)
	kelasGroup := api.Group("/kelas",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	kelasGroup.Get("/", kelasHandler.List)
	kelasGroup.Post("/", kelasHandler.Create)
	kelasGroup.Get("/:id", kelasHandler.Get)
	kelasGroup.Patch("/:id", kelasHandler.Update)
	kelasGroup.Post("/:id/archive", kelasHandler.Archive)
	kelasGroup.Post("/:id/duplicate", kelasHandler.Duplicate)
	kelasGroup.Get("/:id/enrollments", kelasHandler.ListEnrollments)

	// Siswa-side enrollment (Phase 2.C): siswa joins a kelas via kode invite.
	// Rate-limited per IP+siswa to deter scraping. Mounted under /siswa group
	// so role-guard locks it to siswa-only.
	siswaGroup := api.Group("/siswa",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Siswa)),
	)
	siswaGroup.Post("/kelas/join",
		kelas.JoinKodeRateLimit(10),
		kelasHandler.JoinByKode,
	)
	siswaGroup.Get("/kelas", kelasHandler.ListMyKelas)
}

func mountStatic(app *fiber.App, cfg *config.Config, log *slog.Logger) {
	if cfg.FrontendDir == "" {
		return
	}
	if _, err := os.Stat(cfg.FrontendDir); err != nil {
		log.Warn("frontend dir not found, skipping static",
			slog.String("dir", cfg.FrontendDir),
			slog.String("err", err.Error()),
		)
		return
	}
	app.Static("/", cfg.FrontendDir, fiber.Static{
		Compress:      true,
		CacheDuration: 60 * time.Second,
	})
	// SPA fallback for client-side routes (login, dashboard, etc.). API routes
	// are matched first because they're registered before this Use().
	app.Use(func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/") {
			return fiber.ErrNotFound
		}
		index := filepath.Join(cfg.FrontendDir, "index.html")
		if _, err := os.Stat(index); err != nil {
			return fiber.ErrNotFound
		}
		return c.SendFile(index)
	})
}

func fiberCode(status int) string {
	switch status {
	case fiber.StatusBadRequest:
		return "bad_request"
	case fiber.StatusUnauthorized:
		return "unauthorized"
	case fiber.StatusForbidden:
		return "forbidden"
	case fiber.StatusNotFound:
		return "not_found"
	case fiber.StatusConflict:
		return "conflict"
	case fiber.StatusTooManyRequests:
		return "too_many_requests"
	case fiber.StatusServiceUnavailable:
		return "unavailable"
	default:
		return "internal"
	}
}
