package config

import (
	"strings"
	"testing"
	"time"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://lms:test@localhost:5435/lms?sslmode=disable")
	t.Setenv("JWT_SECRET_KEY", "12345678901234567890123456789012")
}

func TestLoadUsesDefaultsWithRequiredEnv(t *testing.T) {
	setRequiredEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Env != "development" || cfg.IsProduction() {
		t.Fatalf("Env = %q IsProduction=%v, want development false", cfg.Env, cfg.IsProduction())
	}
	if cfg.Port != 8200 {
		t.Fatalf("Port = %d, want 8200", cfg.Port)
	}
	if cfg.Timezone.String() != "Asia/Jakarta" || time.Local.String() != "Asia/Jakarta" {
		t.Fatalf("timezone = %q local=%q, want Asia/Jakarta", cfg.Timezone, time.Local)
	}
	if cfg.Database.MaxOpenConns != 25 || cfg.Database.MaxIdleConns != 5 || cfg.Database.ConnMaxLifetimeMin != 30 {
		t.Fatalf("database pool defaults = %+v", cfg.Database)
	}
	if cfg.JWT.AccessTTLMin != 15 || cfg.JWT.RefreshTTLDays != 7 || cfg.JWT.BcryptCost != 12 {
		t.Fatalf("jwt defaults = %+v", cfg.JWT)
	}
	if cfg.Storage.Dir != "./storage/uploads" || cfg.Storage.MaxTugasFileMB != 20 || cfg.Storage.MaxGambarSoalMB != 5 {
		t.Fatalf("storage defaults = %+v", cfg.Storage)
	}
	if cfg.Storage.R2.PresignTTLSec != 900 {
		t.Fatalf("R2 presign default = %d, want 900", cfg.Storage.R2.PresignTTLSec)
	}
	if cfg.RateLimit.GlobalPerMin != 120 || cfg.RateLimit.LoginPer15Min != 10 || cfg.RateLimit.LoginIPPer15Min != 100 || cfg.RateLimit.UploadPerMin != 30 {
		t.Fatalf("rate limit defaults = %+v", cfg.RateLimit)
	}
	if cfg.FrontendDir != "./frontend/out" || cfg.AutoMigrate {
		t.Fatalf("frontend/automigrate defaults = %q/%v", cfg.FrontendDir, cfg.AutoMigrate)
	}
}

func TestLoadParsesOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("ENV", "production")
	t.Setenv("PORT", "9001")
	t.Setenv("TIMEZONE", "UTC")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DB_MAX_OPEN_CONNS", "50")
	t.Setenv("DB_MAX_IDLE_CONNS", "11")
	t.Setenv("DB_CONN_MAX_LIFETIME_MIN", "99")
	t.Setenv("JWT_ACCESS_TTL_MIN", "3")
	t.Setenv("JWT_REFRESH_TTL_DAY", "14")
	t.Setenv("BCRYPT_COST", "10")
	t.Setenv("STORAGE_DIR", "/tmp/lms-storage")
	t.Setenv("MAX_TUGAS_FILE_MB", "42")
	t.Setenv("MAX_GAMBAR_SOAL_MB", "8")
	t.Setenv("R2_ACCOUNT_ID", "acct")
	t.Setenv("R2_ACCESS_KEY_ID", "access")
	t.Setenv("R2_SECRET_ACCESS_KEY", "secret")
	t.Setenv("R2_BUCKET", "bucket")
	t.Setenv("R2_PRESIGN_TTL_SECONDS", "123")
	t.Setenv("RATE_LIMIT_GLOBAL_PER_MIN", "121")
	t.Setenv("RATE_LIMIT_LOGIN_PER_15MIN", "6")
	t.Setenv("RATE_LIMIT_LOGIN_IP_PER_15MIN", "60")
	t.Setenv("RATE_LIMIT_REFRESH_PER_MIN", "7")
	t.Setenv("RATE_LIMIT_KELAS_JOIN_PER_MIN", "8")
	t.Setenv("RATE_LIMIT_UPLOAD_PER_MIN", "9")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://one.example, https://two.example")
	t.Setenv("FRONTEND_DIR", "/srv/lms/public")
	t.Setenv("AUTOMIGRATE", "false")
	t.Setenv("ADMIN_EMAIL", "admin@example.test")
	t.Setenv("ADMIN_PASSWORD", "secret-password")
	t.Setenv("ADMIN_NAME", "Admin Test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if !cfg.IsProduction() || cfg.Port != 9001 || cfg.Timezone.String() != "UTC" || cfg.LogLevel != "debug" {
		t.Fatalf("top-level overrides not applied: %+v", cfg)
	}
	if cfg.Database.MaxOpenConns != 50 || cfg.Database.MaxIdleConns != 11 || cfg.Database.ConnMaxLifetimeMin != 99 {
		t.Fatalf("database overrides = %+v", cfg.Database)
	}
	if cfg.JWT.AccessTTLMin != 3 || cfg.JWT.RefreshTTLDays != 14 || cfg.JWT.BcryptCost != 10 {
		t.Fatalf("jwt overrides = %+v", cfg.JWT)
	}
	if cfg.Storage.Dir != "/tmp/lms-storage" || cfg.Storage.MaxTugasFileMB != 42 || cfg.Storage.MaxGambarSoalMB != 8 {
		t.Fatalf("storage overrides = %+v", cfg.Storage)
	}
	if cfg.Storage.R2.AccountID != "acct" || cfg.Storage.R2.AccessKeyID != "access" || cfg.Storage.R2.SecretAccessKey != "secret" || cfg.Storage.R2.Bucket != "bucket" || cfg.Storage.R2.PresignTTLSec != 123 {
		t.Fatalf("r2 overrides = %+v", cfg.Storage.R2)
	}
	if cfg.RateLimit.GlobalPerMin != 121 || cfg.RateLimit.LoginPer15Min != 6 || cfg.RateLimit.LoginIPPer15Min != 60 || cfg.RateLimit.RefreshPerMin != 7 || cfg.RateLimit.KelasJoinPerMin != 8 || cfg.RateLimit.UploadPerMin != 9 {
		t.Fatalf("rate limit overrides = %+v", cfg.RateLimit)
	}
	if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[1] != " https://two.example" {
		t.Fatalf("cors origins = %#v", cfg.CORS.AllowedOrigins)
	}
	if cfg.FrontendDir != "/srv/lms/public" || cfg.AutoMigrate {
		t.Fatalf("frontend/automigrate overrides = %q/%v", cfg.FrontendDir, cfg.AutoMigrate)
	}
	if cfg.SeedAdminEmail != "admin@example.test" || cfg.SeedAdminPassword != "secret-password" || cfg.SeedAdminName != "Admin Test" {
		t.Fatalf("seed admin overrides = %q/%q/%q", cfg.SeedAdminEmail, cfg.SeedAdminPassword, cfg.SeedAdminName)
	}
}

func TestLoadFallsBackOnInvalidNumericAndBooleanOverrides(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("PORT", "not-a-number")
	t.Setenv("AUTOMIGRATE", "maybe")
	t.Setenv("R2_PRESIGN_TTL_SECONDS", "bad")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != 8200 || cfg.AutoMigrate || cfg.Storage.R2.PresignTTLSec != 900 {
		t.Fatalf("fallbacks = port %d automigrate %v presign %d", cfg.Port, cfg.AutoMigrate, cfg.Storage.R2.PresignTTLSec)
	}
}

func TestLoadValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "missing database url",
			env:  map[string]string{"JWT_SECRET_KEY": "12345678901234567890123456789012"},
			want: "DATABASE_URL is required",
		},
		{
			name: "missing jwt secret",
			env:  map[string]string{"DATABASE_URL": "postgres://example"},
			want: "JWT_SECRET_KEY is required",
		},
		{
			name: "short jwt secret",
			env:  map[string]string{"DATABASE_URL": "postgres://example", "JWT_SECRET_KEY": "short"},
			want: "at least 32 bytes",
		},
		{
			name: "automigrate production",
			env: map[string]string{
				"DATABASE_URL":   "postgres://example",
				"JWT_SECRET_KEY": "12345678901234567890123456789012",
				"ENV":            "production",
				"AUTOMIGRATE":    "true",
			},
			want: "AUTOMIGRATE must be false in production",
		},
		{
			name: "invalid port",
			env: map[string]string{
				"DATABASE_URL":   "postgres://example",
				"JWT_SECRET_KEY": "12345678901234567890123456789012",
				"PORT":           "70000",
			},
			want: "invalid PORT 70000",
		},
		{
			name: "invalid timezone",
			env: map[string]string{
				"DATABASE_URL":   "postgres://example",
				"JWT_SECRET_KEY": "12345678901234567890123456789012",
				"TIMEZONE":       "Mars/Phobos",
			},
			want: "invalid TIMEZONE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Load() error = %v, want contains %q", err, tt.want)
			}
		})
	}
}
