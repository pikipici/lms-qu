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
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"github.com/pikip/lms/backend/internal/health"
	"github.com/pikip/lms/backend/internal/importjob"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/materi"
	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/pengumuman"
	"github.com/pikip/lms/backend/internal/siswabab"
	"github.com/pikip/lms/backend/internal/soalbab"
	"github.com/pikip/lms/backend/internal/storage"
	"github.com/pikip/lms/backend/internal/submission"
	"github.com/pikip/lms/backend/internal/tugas"
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
	mountRoutes(rootCtx, app, cfg, gdb, objectStore)
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

func mountRoutes(rootCtx context.Context, app *fiber.App, cfg *config.Config, gdb *gorm.DB, objectStore storage.Storage) {
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
	importSvc.SetUserCreator(authRepo)
	importSvc.SetKelasRepo(kelasRepo)
	importSvc.SetPresignTTL(time.Duration(cfg.Storage.R2.PresignTTLSec) * time.Second)
	adminGroup.Post("/import-csv/upload", importHandler.PreviewUpload)
	adminGroup.Get("/import-csv/:job_id", importHandler.GetPreview)
	adminGroup.Post("/import-csv/:job_id/cancel", importHandler.Cancel)
	adminGroup.Post("/import-csv/:job_id/confirm", importHandler.Confirm)
	adminGroup.Get("/import-csv/:job_id/credentials.csv", importHandler.DownloadCredentials)

	// Hourly cleanup cron (Task 2.D.6): expire stale preview jobs + evict
	// post-TTL credentials.csv blobs from R2. Started here so it shares
	// the request-scoped context cancellation on shutdown.
	importCleaner := importjob.NewCleaner(importRepo, objectStore)
	go importCleaner.Run(rootCtx)
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

	// Bab (Phase 3): chapter CRUD per kelas. Mounted under same /kelas group
	// for list/create (POST /kelas/:id/bab) — but Get/Patch/Archive use the
	// flat /bab/:id form (Section 7) so the bab id alone is enough to route.
	babRepo := bab.NewRepo(gdb)
	babSvc := bab.NewService(babRepo, kelasRepo, authRepo)
	babHandler := bab.NewHandler(babSvc)
	kelasGroup.Get("/:id/bab", babHandler.ListByKelas)
	kelasGroup.Post("/:id/bab", babHandler.Create)
	kelasGroup.Post("/:id/bab/reorder", babHandler.Reorder)
	babGroup := api.Group("/bab",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	babGroup.Get("/:id", babHandler.Get)
	babGroup.Patch("/:id", babHandler.Update)
	babGroup.Post("/:id/archive", babHandler.Archive)
	babGroup.Post("/:id/duplicate", babHandler.Duplicate)

	// SoalBab (Task 5.B.1 — CRUD): multiple-choice 5-opsi soal per bab.
	// Latihan + Ulangan flow eligibility via mode field (locked #76).
	// Image upload (locked #78) + bulk paste (locked #77) di Task 5.B.2/5.B.3.
	// Owner-only mutations + siswa direct-list/get BLOCKED (siswa lewat
	// flow Latihan/Ulangan endpoint, locked #76 anti-cheat).
	soalbabRepo := soalbab.NewRepo(gdb)
	soalbabSvc := soalbab.NewService(soalbabRepo, kelasRepo, babRepo, authRepo, objectStore)
	soalbabHandler := soalbab.NewHandler(soalbabSvc, objectStore)
	babGroup.Post("/:id/soal", soalbabHandler.Create)
	babGroup.Get("/:id/soal", soalbabHandler.ListByBab)
	babGroup.Post("/:id/soal/bulk", soalbabHandler.BulkCreate)
	soalbabGroup := api.Group("/soal-bab",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	soalbabGroup.Get("/:id", soalbabHandler.Get)
	soalbabGroup.Patch("/:id", soalbabHandler.Update)
	soalbabGroup.Delete("/:id", soalbabHandler.Delete)
	// Task 5.B.2 — inline image slots (pertanyaan + opsi a..e).
	soalbabGroup.Post("/:id/image", soalbabHandler.UploadImage)
	soalbabGroup.Delete("/:id/image", soalbabHandler.DeleteImage)
	soalbabGroup.Get("/:id/image-url", soalbabHandler.ImageURL)

	// Materi (Task 3.C.2 + 3.C.3): learning content CRUD per kelas.
	// youtube + markdown via JSON (3.C.2); PDF via multipart upload (3.C.3
	// — mime sniff, 20MB cap, compensating R2 delete on DB fail/Delete).
	// MarkRead siswa flow di Task 3.C.4.
	materiRepo := materi.NewRepo(gdb)
	materiSvc := materi.NewService(materiRepo, kelasRepo, babRepo, authRepo, objectStore, kelasRepo)
	materiHandler := materi.NewHandler(materiSvc)
	kelasGroup.Get("/:id/materi", materiHandler.ListByKelas)
	kelasGroup.Post("/:id/materi", materiHandler.Create)
	kelasGroup.Post("/:id/materi/upload", materiHandler.Upload)
	materiGroup := api.Group("/materi",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	materiGroup.Get("/:id", materiHandler.Get)
	materiGroup.Get("/:id/file-url", materiHandler.FileURL)
	materiGroup.Patch("/:id", materiHandler.Update)
	materiGroup.Delete("/:id", materiHandler.Delete)

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

	// Materi MarkRead (Task 3.C.4): siswa-only idempotent mark-as-read.
	// Enrollment guard inside Service.MarkRead — no extra rate limit needed
	// (idempotent + cheap upsert).
	siswaGroup.Post("/materi/:id/read", materiHandler.MarkRead)

	// Siswa bab list + detail (Task 3.E.1): published-only + progress
	// fase-3-partial (materi_read / materi_total). Enrollment guard inside
	// siswabab.Service.ListSiswa/GetSiswa. Lives in its own package to
	// avoid an import cycle: materi → bab + kelas, jadi siswa-bab harus
	// di luar bab.
	siswaBabSvc := siswabab.NewService(babRepo, kelasRepo, kelasRepo, materiRepo)
	siswaBabHandler := siswabab.NewHandler(siswaBabSvc)
	siswaGroup.Get("/kelas/:id/bab", siswaBabHandler.ListSiswa)
	siswaGroup.Get("/bab/:id", siswaBabHandler.GetSiswa)

	// Pengumuman (Task 3.F.1): announcement CRUD per kelas. BabID nullable
	// — bisa kelas-wide atau bab-scoped. Status enum published|archived
	// (locked #66 passive timestamp). Kelas-scope routes (POST/GET) under
	// kelas group; flat routes (GET/PATCH/DELETE :id) under pengumuman group
	// untuk siswa enrolled + guru pemilik. Mirror pola materi.
	pengumumanRepo := pengumuman.NewRepo(gdb)
	pengumumanSvc := pengumuman.NewService(pengumumanRepo, kelasRepo, babRepo, kelasRepo, authRepo)
	pengumumanHandler := pengumuman.NewHandler(pengumumanSvc)
	kelasGroup.Post("/:id/pengumuman", pengumumanHandler.Create)
	kelasGroup.Get("/:id/pengumuman", pengumumanHandler.ListByKelas)
	pengumumanGroup := api.Group("/pengumuman",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru), string(auth.Siswa)),
	)
	pengumumanGroup.Get("/:id", pengumumanHandler.Get)
	pengumumanGroup.Patch("/:id", pengumumanHandler.Update)
	pengumumanGroup.Delete("/:id", pengumumanHandler.Delete)

	// Siswa-scope read for kelas pengumuman list — siswa enrolled needs
	// access to GET /kelas/:id/pengumuman. Above kelas group gates by
	// admin/guru roles. Add a separate siswa-scope alias under siswaGroup
	// reusing the same handler; service.ListByKelas branches by role to
	// force published-only + enrollment guard for siswa.
	siswaGroup.Get("/kelas/:id/pengumuman", pengumumanHandler.ListByKelas)

	// Tugas (Task 4.A.2): assignment CRUD per kelas. BabID nullable (locked
	// #20). Status enum draft|published|archived. Late policy via
	// IzinkanLate + PenaltyPersen (locked #71). Mirror pola pengumuman:
	// kelas-scope routes (POST/GET) under kelasGroup, flat routes
	// (GET/PATCH/DELETE :id) under tugasGroup terbuka untuk siswa enrolled
	// (service branches by role).
	tugasRepo := tugas.NewRepo(gdb)
	tugasSvc := tugas.NewService(tugasRepo, kelasRepo, babRepo, kelasRepo, authRepo, objectStore)
	tugasHandler := tugas.NewHandler(tugasSvc)
	kelasGroup.Post("/:id/tugas", tugasHandler.Create)
	kelasGroup.Get("/:id/tugas", tugasHandler.ListByKelas)
	tugasGroup := api.Group("/tugas",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru), string(auth.Siswa)),
	)
	tugasGroup.Get("/:id", tugasHandler.Get)
	tugasGroup.Patch("/:id", tugasHandler.Update)
	tugasGroup.Delete("/:id", tugasHandler.Delete)
	tugasGroup.Post("/:id/duplicate", tugasHandler.Duplicate)

	// Attachment endpoints (Task 4.A.3): multipart upload + presigned download.
	// Allowlist mime locked #46 (pdf, docx, jpg, png, zip), cap 5×20MB
	// (locked #74). Siswa enrolled bisa GET list + presigned URL untuk
	// download lampiran soal sebelum submit; guru/admin owner control upload+delete.
	tugasGroup.Post("/:id/attachments", tugasHandler.UploadAttachment)
	tugasGroup.Get("/:id/attachments", tugasHandler.ListAttachments)
	tugasGroup.Delete("/:id/attachments/:attID", tugasHandler.DeleteAttachment)
	tugasGroup.Get("/:id/attachments/:attID/url", tugasHandler.AttachmentURL)

	// Siswa-scope read alias mirror pengumuman pattern.
	siswaGroup.Get("/kelas/:id/tugas", tugasHandler.ListByKelas)

	// Submission (Task 4.C.2-4.C.4): siswa submit + guru grade. Single-row
	// per (tugas, siswa) with version bump on resubmit (locked #70). Late
	// submission gating via tugas.IzinkanLate + penalty calc on grade
	// (locked #71). Attachment policy mirror tugas: 0..N optional, cap 5×20MB
	// (locked #72). Submit + grade pakai SELECT FOR UPDATE + idempotent guard
	// (locked #73).
	submissionRepo := submission.NewRepo(gdb)
	submissionSvc := submission.NewService(submissionRepo, tugasRepo, kelasRepo, kelasRepo, authRepo, objectStore)
	submissionHandler := submission.NewHandler(submissionSvc)

	// Siswa: submit + view own + tugas info pre-fill.
	siswaGroup.Post("/tugas/:id/submit", submissionHandler.Submit)
	siswaGroup.Get("/tugas/:id/submission", submissionHandler.GetMySubmission)
	siswaGroup.Get("/submissions", submissionHandler.ListMine)

	// Guru/admin: rekap submission per tugas (status filter optional).
	tugasGroup.Get("/:id/submissions", submissionHandler.ListByTugas)

	// Submission flat group — guru/admin owner OR siswa pemilik untuk
	// GET (service branches by role); grade restricted ke guru/admin.
	submissionAllRolesGroup := api.Group("/submission",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru), string(auth.Siswa)),
	)
	submissionAllRolesGroup.Get("/:id", submissionHandler.Get)
	submissionAllRolesGroup.Get("/:id/attachments/:attID/url", submissionHandler.AttachmentURL)

	submissionStaffGroup := api.Group("/submission",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	submissionStaffGroup.Post("/:id/grade", submissionHandler.Grade)

	// Guru pending counters (Task 4.E.2 — partial; activity feed full
	// deferred ke Fase 7 locked #39). Cumulative across kelas yang dimiliki
	// guru; admin sees all kelas. Used untuk badge sidebar + dashboard.
	pendingCounter := submission.NewPendingCounter(submissionRepo, kelasRepo)
	pendingHandler := submission.NewPendingHandler(pendingCounter)
	guruGroup := api.Group("/guru",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	guruGroup.Get("/pending-counts", pendingHandler.Count)
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
