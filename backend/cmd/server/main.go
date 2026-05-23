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
	"github.com/pikip/lms/backend/internal/audit"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/banksoal"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"github.com/pikip/lms/backend/internal/feed"
	"github.com/pikip/lms/backend/internal/health"
	"github.com/pikip/lms/backend/internal/importjob"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/materi"
	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/nilai"
	"github.com/pikip/lms/backend/internal/pengumuman"
	"github.com/pikip/lms/backend/internal/siswabab"
	"github.com/pikip/lms/backend/internal/soalbab"
	"github.com/pikip/lms/backend/internal/storage"
	"github.com/pikip/lms/backend/internal/submission"
	"github.com/pikip/lms/backend/internal/tugas"
	"github.com/pikip/lms/backend/internal/ujian"
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

	// Task 5.C.1 — UlanganBabSetting GET + PUT (upsert).
	// Guru/admin GET via /bab/:id/ulangan-setting (full payload incl.
	// pool_size + version). Siswa GET via /siswa/bab/:id/ulangan-setting
	// (trimmed lobby payload, requires active enrollment). PUT
	// guru/admin only with optimistic concurrency (locked #56) +
	// jumlah_soal pool validation.
	soalbabSettingSvc := soalbab.NewSettingService(soalbabRepo, babRepo, kelasRepo, kelasRepo, authRepo)
	soalbabSettingHandler := soalbab.NewSettingHandler(soalbabSettingSvc)
	babGroup.Get("/:id/ulangan-setting", soalbabSettingHandler.Get)
	babGroup.Put("/:id/ulangan-setting", soalbabSettingHandler.Upsert)

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
	// Task 5.C.1 — siswa lobby payload (trimmed). Reuses the same
	// SettingHandler.Get; the handler branches on role internally and
	// returns SiswaLobbyView when caller is siswa.
	siswaGroup.Get("/bab/:id/ulangan-setting", soalbabSettingHandler.Get)

	// Task 5.C.2 — Latihan flow (formative, no nilai persist). Siswa
	// enrolled mengerjakan soal mode IN ('latihan','keduanya') tanpa
	// timer + tanpa nilai persist. Tiap answer balikin is_benar +
	// jawaban_benar untuk feedback langsung. Re-attempt unlimited per
	// locked #81 (resume otomatis kalau sudah ada attempt berlangsung).
	soalbabLatihanSvc := soalbab.NewLatihanService(soalbabRepo, babRepo, kelasRepo)
	soalbabLatihanHandler := soalbab.NewLatihanHandler(soalbabLatihanSvc)
	siswaGroup.Post("/bab/:id/latihan/start", soalbabLatihanHandler.Start)
	siswaGroup.Post("/hasil-soal-bab/:id/finish", soalbabLatihanHandler.Finish)

	// Task 5.D.1 — Ulangan Bab start (random pool deterministic seed).
	// Pool snapshot via sha256(mulai_at_micro || siswa || bab)[:8] LE
	// → math/rand source (locked #79). Single-flight per (bab, siswa)
	// via pg_advisory_xact_lock — concurrent Start calls return same
	// hasil. attempt_no enforced ≤ Setting.BatasAttempt (locked #76).
	// Submit/cron land in 5.D.3-5.D.4.
	soalbabUlanganSvc := soalbab.NewUlanganService(soalbabRepo, babRepo, kelasRepo, authRepo)
	soalbabUlanganHandler := soalbab.NewUlanganHandler(soalbabUlanganSvc)
	siswaGroup.Post("/bab/:id/ulangan/start", soalbabUlanganHandler.Start)
	// Task 5.D.3 — Ulangan submit + auto-grade tx (advisory lock per
	// hasil_id). Idempotent: row sudah selesai balikin existing rekap.
	siswaGroup.Post("/hasil-soal-bab/:id/submit", soalbabUlanganHandler.Submit)

	// Task 5.D.4 — Timer expire cron 30s. Periodic sweep mark hasil
	// 'berlangsung' yang sudah lewat deadline_at jadi 'selesai' +
	// auto-grade. Share advisory lock key dengan Submit (per hasil_id)
	// jadi siswa klik submit dan cron sweep mutually exclusive. Initial
	// sweep on boot catches downtime backlog. Cancellable via rootCtx.
	timerCron := soalbab.NewTimerCron(soalbabRepo)
	go timerCron.Run(rootCtx)

	// Task 6.A.1 + 6.B.1 — Fase 6 BankSoal foundation + CRUD endpoints.
	// BankSoal per-guru pribadi (locked #84): tidak ada share antar-guru.
	// Tag mapel/tingkat/topik free-form text — random-mode Ujian filter
	// di Task 6.C.2. Image upload + bulk paste di Task 6.B.2 + 6.B.3.
	// Ujian repo skeleton tetap underscore-bound sampai 6.C.1 wire.
	bankSoalRepo := banksoal.NewRepo(gdb)
	bankSoalSvc := banksoal.NewService(bankSoalRepo, authRepo, objectStore)
	bankSoalHandler := banksoal.NewHandler(bankSoalSvc, objectStore)
	ujianRepo := ujian.NewRepo(gdb)
	ujianSvc := ujian.NewService(ujianRepo, kelasRepo, bankSoalRepo, authRepo)
	ujianHandler := ujian.NewHandler(ujianSvc)
	ujianFlowSvc := ujian.NewFlowService(ujianRepo, bankSoalRepo, kelasRepo, authRepo)
	ujianItemsSvc := ujian.NewItemsService(ujianRepo, bankSoalRepo, objectStore)
	ujianFlowHandler := ujian.NewFlowHandler(ujianFlowSvc, ujianItemsSvc)

	// Task 6.D.4 — Ujian timer expire cron. Mirrors soalbab.TimerCron
	// (locked #87): 30s tick, advisory lock per hasil_id (sha256
	// "hasil-submit:" key) shared with FlowService.Submit so cron and
	// siswa submit are mutually exclusive on the same row.
	ujianTimerCron := ujian.NewTimerCron(ujianRepo, bankSoalRepo)
	go ujianTimerCron.Run(rootCtx)

	// guruGroup belum di-register di sini — register di bawah setelah
	// pendingHandler block. Wire route di sana supaya satu group definition.

	// Task 5.D.2 — Answer endpoint dispatcher. Latihan dapat immediate
	// is_benar feedback (locked #81), Ulangan delayed grade dengan
	// is_benar=NULL + 410 timer_expired guard (locked #76). FE hit satu
	// endpoint shared, dispatcher peek hasil.mode lalu route.
	soalbabAnswerHandler := soalbab.NewAttemptAnswerHandler(soalbabRepo, soalbabLatihanHandler, soalbabUlanganHandler)
	siswaGroup.Post("/hasil-soal-bab/:id/answer", soalbabAnswerHandler.Answer)

	// Task 5.E.1 — Hasil + Review + Cancel + Rekap consolidated endpoints.
	// Siswa-side: review jawaban setelah submit (gated locked #81),
	// list hasil sendiri di bab (untuk lobby/resume hint).
	// Guru-side: soft-cancel attempt buat remedial reset (locked #76 —
	// dibatalkan tidak count terhadap batas_attempt), rekap dashboard
	// per-siswa nilai_terbaik+terakhir.
	soalbabHasilSvc := soalbab.NewHasilService(soalbabRepo, babRepo, kelasRepo, kelasRepo, authRepo, authRepo, objectStore)
	soalbabHasilHandler := soalbab.NewHasilHandler(soalbabHasilSvc)
	siswaGroup.Get("/hasil-soal-bab/:id/review", soalbabHasilHandler.Review)
	siswaGroup.Get("/hasil-soal-bab/:id/items", soalbabHasilHandler.Items)
	siswaGroup.Get("/bab/:id/hasil", soalbabHasilHandler.ListSiswa)
	babGroup.Get("/:id/hasil-rekap", soalbabHasilHandler.Rekap)
	// Cancel: guru/admin route via babGroup wrapper (sudah punya
	// RoleGuard admin|guru). Pakai path flat hasil-soal-bab supaya FE
	// gampang dispatch. Wire under api group dengan auth + role guard.
	hasilGuruGroup := api.Group("/hasil-soal-bab",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	hasilGuruGroup.Post("/:id/cancel", soalbabHasilHandler.Cancel)

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

	// Task 7.C — Activity feed guru (locked #39+#55). UNION ALL aggregator
	// across submission_baru / ulangan_selesai / siswa_join with opaque
	// base64 cursor `(at_unix_micro DESC, id DESC)`. Polling 30s + load-
	// more pakai cursor.
	feedRepo := feed.NewRepo(gdb)
	feedSvc := feed.NewService(feedRepo)
	feedHandler := feed.NewHandler(feedSvc)
	guruGroup.Get("/feed", feedHandler.List)

	// Task 7.E — Guru audit log scope per kelas (locked #59). Endpoint:
	//   GET /api/v1/guru/kelas/:id/audit?action=&limit=&offset=
	//   GET /api/v1/guru/audit-actions
	// Hard scope: WHERE target_kelas_id=:id; guru ownership check.
	auditSvc := audit.NewService(
		authRepo,
		audit.KelasFinderAdapter{Repo: kelasRepo},
		authRepo, // BulkUserNames adapter satisfies userLookup
	)
	auditHandler := audit.NewHandler(auditSvc)
	guruGroup.Get("/audit-actions", auditHandler.ListActions)
	guruGroup.Get("/kelas/:id/audit", auditHandler.ListByKelas)

	// Task 6.B.1 — BankSoal CRUD endpoints (per-guru pribadi locked #84).
	// Mounted under /api/v1/bank-soal (admin/guru only). Siswa BLOCKED at
	// service boundary — siswa baca soal lewat HasilUjian flow di Fase 6.D.
	bankSoalGroup := api.Group("/bank-soal",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	bankSoalGroup.Post("/", bankSoalHandler.Create)
	bankSoalGroup.Get("/", bankSoalHandler.List)
	// Static prefixes BEFORE /:id supaya tidak conflict dengan UUID matcher.
	bankSoalGroup.Post("/bulk", bankSoalHandler.BulkCreate)
	bankSoalGroup.Get("/:id", bankSoalHandler.Get)
	bankSoalGroup.Patch("/:id", bankSoalHandler.Update)
	bankSoalGroup.Delete("/:id", bankSoalHandler.Delete)

	// Task 6.B.2 — image upload (6 slot inline) per-guru pribadi.
	// Mirror SoalBab Task 5.B.2 — multipart upload, R2 prefix soal-bank/.
	bankSoalGroup.Post("/:id/image", bankSoalHandler.UploadImage)
	bankSoalGroup.Delete("/:id/image", bankSoalHandler.DeleteImage)
	bankSoalGroup.Get("/:id/image-url", bankSoalHandler.ImageURL)

	// Task 6.C.1 + 6.C.2 — Ujian setup (CRUD + duplicate + source dispatch).
	// Kelas-scope routes (POST/GET kelas/:id/ujian) under kelasGroup
	// dengan service-level role branching (siswa published-only).
	// Flat /ujian/:id routes admin/guru-only (siswa baca lewat HasilUjian
	// flow di Fase 6.D).
	kelasGroup.Post("/:id/ujian", ujianHandler.Create)
	kelasGroup.Get("/:id/ujian", ujianHandler.ListByKelas)

	ujianGroup := api.Group("/ujian",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru), string(auth.Siswa)),
	)
	ujianGroup.Get("/:id", ujianHandler.Get)
	ujianStaffGroup := api.Group("/ujian",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	// Static prefixes BEFORE /:id supaya tidak conflict dengan UUID matcher.
	ujianStaffGroup.Post("/:id/duplicate", ujianHandler.Duplicate)
	ujianStaffGroup.Post("/:id/source/preview", ujianHandler.PreviewSource)
	ujianStaffGroup.Patch("/:id", ujianHandler.Update)
	ujianStaffGroup.Delete("/:id", ujianHandler.Delete)

	// Task 6.D.1 — Ujian flow start + items (siswa).
	// Start: POST /siswa/ujian/:id/start (deterministic seed locked #86,
	//        single-flight via pg_advisory_xact_lock(ujian_id||siswa_id),
	//        single-attempt enforcement via partial-unique index).
	// Items: GET /siswa/hasil-ujian/:id/items (anti-cheat — strip
	//        jawaban_benar; presigned image slots TTL 15m).
	// Answer: POST /siswa/hasil-ujian/:id/answer (6.D.2 — UPSERT
	//        JawabanUjian dgn is_benar=NULL+poin_dapat=0; delayed
	//        grade locked #76 mirror; cron locked #87 grade nanti).
	// Submit: POST /siswa/hasil-ujian/:id/submit (6.D.3 — single-tx
	//        auto-grade, pg_advisory_xact_lock per hasil_id; idempotent;
	//        late-submit grace 5s past deadline; mirror 5.D.3 d262ea3).
	siswaGroup.Post("/ujian/:id/start", ujianFlowHandler.Start)
	siswaGroup.Get("/hasil-ujian/:id/items", ujianFlowHandler.Items)
	siswaGroup.Post("/hasil-ujian/:id/answer", ujianFlowHandler.Answer)
	siswaGroup.Post("/hasil-ujian/:id/submit", ujianFlowHandler.Submit)

	// Task 6.E.1 — Ujian Hasil + Review + Cancel + Rekap consolidated.
	// Mirror soalbab.HasilService (commit 8c55651) adapted untuk Ujian:
	// review gating embedded di Ujian (IzinkanReviewSetelahSubmit +
	// WaktuBukaReview, locked #81); UjianID-based scope; BankSoal
	// source. Soft-cancel: Status='dibatalkan' + DeletedAt — partial-
	// unique (ujian_id, siswa_id) WHERE deleted_at IS NULL membebaskan
	// slot supaya siswa boleh start fresh attempt (locked #76).
	ujianHasilSvc := ujian.NewHasilService(ujianRepo, bankSoalRepo, kelasRepo, kelasRepo, authRepo, authRepo)
	ujianHasilHandler := ujian.NewHasilHandler(ujianHasilSvc)
	siswaGroup.Get("/hasil-ujian/:id/review", ujianHasilHandler.Review)
	siswaGroup.Get("/kelas/:id/ujian/hasil", ujianHasilHandler.ListSiswa)
	// Task 6.G.1 — siswa list ujian per kelas. Reuse ujianHandler.ListByKelas;
	// service-level role-branch (callerRole=siswa) auto-filter status=published
	// only + verify enrollment.
	siswaGroup.Get("/kelas/:id/ujian", ujianHandler.ListByKelas)
	ujianStaffGroup.Get("/:id/hasil-rekap", ujianHasilHandler.Rekap)
	// Cancel: flat /hasil-ujian/:id/cancel under guru/admin role guard.
	hasilUjianGuruGroup := api.Group("/hasil-ujian",
		middleware.BearerAuth(authSvc),
		middleware.ForceChangePassword(),
		middleware.RoleGuard(string(auth.Admin), string(auth.Guru)),
	)
	hasilUjianGuruGroup.Post("/:id/cancel", ujianHasilHandler.Cancel)

	// Task 7.A.1 — Nilai siswa endpoints (locked #89-#91).
	// Read-only aggregator (locked #90): per-kelas + lintas-kelas
	// returns NilaiBab + NilaiUlanganHarian + TotalKelas pakai single-pass
	// JOINs ke HasilSoalBab/Submission/HasilUjian. Authorization
	// callerRole=siswa + active enrollment (defensive — service & route
	// double-check). Routing locked #91: /siswa/nilai (lintas) +
	// /siswa/kelas/:id/nilai (per-kelas).
	//
	// Task 7.B — Guru rekap matrix (locked #91+#94): GET /kelas/:id/rekap
	// dgn ?format=json|csv. Reuse aggregator per siswa (loop bounded by
	// active enrollment count, MVP cap 10K).
	nilaiRepo := nilai.NewRepo(gdb)
	nilaiSvc := nilai.NewService(nilaiRepo, kelasRepo, kelasRepo)
	nilaiUserAdapter := nilai.UserNameAdapter{Repo: authRepo}
	nilaiHandler := nilai.NewHandler(nilaiSvc, kelasRepo, nilaiUserAdapter)
	siswaGroup.Get("/nilai", nilaiHandler.SiswaList)
	siswaGroup.Get("/kelas/:id/nilai", nilaiHandler.SiswaKelasNilai)
	kelasGroup.Get("/:id/rekap", nilaiHandler.GuruKelasRekap)
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
	// Next static export writes route files as /route.html. Fiber static can
	// fall through to index.html for extensionless routes, so resolve exported
	// route files before mounting the static handler.
	app.Use(func(c *fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/api/") || strings.Contains(filepath.Base(path), ".") {
			return c.Next()
		}
		candidate := filepath.Join(cfg.FrontendDir, strings.TrimPrefix(path, "/")+".html")
		if _, err := os.Stat(candidate); err == nil {
			return c.SendFile(candidate)
		}
		return c.Next()
	})
	app.Static("/", cfg.FrontendDir, fiber.Static{
		Compress:      true,
		CacheDuration: 60 * time.Second,
	})
	// SPA fallback for client-side routes. API routes are matched first because
	// they're registered before this Use().
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
