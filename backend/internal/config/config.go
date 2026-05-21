// Package config loads environment configuration into a strongly-typed struct.
//
// Locked decisions referenced:
//   - #2 Backend = Go + Fiber + GORM + PostgreSQL
//   - #29 Timezone server = Asia/Jakarta
//   - #35 Migration via golang-migrate; AUTOMIGRATE flag for dev only
//   - #44 healthz/readyz endpoints
//   - #47 Global rate limit
//   - #51 Data retention policy (caller wires cleanup jobs)
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the top-level runtime configuration for the LMS backend.
//
// All values are populated from environment variables (and `.env` if present
// via godotenv). Secrets must NEVER be logged in plaintext; callers should
// reference fields by name rather than dumping the struct.
type Config struct {
	Env       string // development | production
	Port      int
	Timezone  *time.Location
	LogLevel  string
	StartedAt time.Time

	Database  DatabaseConfig
	JWT       JWTConfig
	Storage   StorageConfig
	RateLimit RateLimitConfig
	CORS      CORSConfig

	FrontendDir  string
	AutoMigrate  bool

	// Optional seed-admin parameters (only used by cmd/seed-admin).
	SeedAdminEmail    string
	SeedAdminPassword string
	SeedAdminName     string
}

type DatabaseConfig struct {
	URL                string
	MaxOpenConns       int
	MaxIdleConns       int
	ConnMaxLifetimeMin int
}

type JWTConfig struct {
	SecretKey      string
	AccessTTLMin   int
	RefreshTTLDays int
	BcryptCost     int
}

type StorageConfig struct {
	// Dir is the legacy local-disk root. Retained for backward compat with
	// pre-v0.8.0 wiring; new code uses R2 (locked decision #61).
	Dir             string
	MaxTugasFileMB  int
	MaxGambarSoalMB int

	// R2 holds Cloudflare R2 connection settings. May be empty in dev/CI;
	// callers fall back to MockStorage when R2.IsConfigured() is false
	// and AllowMockFallback is enabled.
	R2 R2Config
}

// R2Config mirrors storage.R2Config (kept as a value here so the config
// package doesn't import internal/storage and create a cycle). The wiring
// layer (cmd/server/main.go) translates between the two.
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PresignTTLSec   int
}

type RateLimitConfig struct {
	GlobalPerMin     int
	LoginPer15Min    int
	RefreshPerMin    int
	KelasJoinPerMin  int
	UploadPerMin     int
}

type CORSConfig struct {
	AllowedOrigins []string
}

// Load reads environment configuration. It loads `.env` from the current
// working directory if present (best-effort; missing file is not an error).
//
// Returns the populated Config plus the first hard validation error
// (missing required fields, invalid values, etc).
func Load() (*Config, error) {
	// Best-effort .env loading. We don't fail if it doesn't exist — production
	// passes env vars via systemd EnvironmentFile.
	_ = godotenv.Load()

	cfg := &Config{
		Env:       getEnv("ENV", "development"),
		LogLevel:  getEnv("LOG_LEVEL", "info"),
		StartedAt: time.Now(),
	}

	cfg.Port = getInt("PORT", 8200)

	tzName := getEnv("TIMEZONE", "Asia/Jakarta")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("config: invalid TIMEZONE %q: %w", tzName, err)
	}
	cfg.Timezone = loc
	time.Local = loc // server-wide TZ lock (#29)

	cfg.Database = DatabaseConfig{
		URL:                getEnv("DATABASE_URL", ""),
		MaxOpenConns:       getInt("DB_MAX_OPEN_CONNS", 25),
		MaxIdleConns:       getInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetimeMin: getInt("DB_CONN_MAX_LIFETIME_MIN", 30),
	}

	cfg.JWT = JWTConfig{
		SecretKey:      getEnv("JWT_SECRET_KEY", ""),
		AccessTTLMin:   getInt("JWT_ACCESS_TTL_MIN", 15),
		RefreshTTLDays: getInt("JWT_REFRESH_TTL_DAY", 7),
		BcryptCost:     getInt("BCRYPT_COST", 12),
	}

	cfg.Storage = StorageConfig{
		Dir:             getEnv("STORAGE_DIR", "./storage/uploads"),
		MaxTugasFileMB:  getInt("MAX_TUGAS_FILE_MB", 20),
		MaxGambarSoalMB: getInt("MAX_GAMBAR_SOAL_MB", 5),
		R2: R2Config{
			AccountID:       getEnv("R2_ACCOUNT_ID", ""),
			AccessKeyID:     getEnv("R2_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("R2_SECRET_ACCESS_KEY", ""),
			Bucket:          getEnv("R2_BUCKET", ""),
			PresignTTLSec:   getInt("R2_PRESIGN_TTL_SECONDS", 900),
		},
	}

	cfg.RateLimit = RateLimitConfig{
		GlobalPerMin:    getInt("RATE_LIMIT_GLOBAL_PER_MIN", 120),
		LoginPer15Min:   getInt("RATE_LIMIT_LOGIN_PER_15MIN", 10),
		RefreshPerMin:   getInt("RATE_LIMIT_REFRESH_PER_MIN", 10),
		KelasJoinPerMin: getInt("RATE_LIMIT_KELAS_JOIN_PER_MIN", 10),
		UploadPerMin:    getInt("RATE_LIMIT_UPLOAD_PER_MIN", 30),
	}

	cors := strings.TrimSpace(getEnv("CORS_ALLOWED_ORIGINS", ""))
	if cors != "" {
		cfg.CORS.AllowedOrigins = strings.Split(cors, ",")
	}

	cfg.FrontendDir = getEnv("FRONTEND_DIR", "./frontend/out")
	cfg.AutoMigrate = getBool("AUTOMIGRATE", false)

	cfg.SeedAdminEmail = getEnv("ADMIN_EMAIL", "")
	cfg.SeedAdminPassword = getEnv("ADMIN_PASSWORD", "")
	cfg.SeedAdminName = getEnv("ADMIN_NAME", "")

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Database.URL == "" {
		return errors.New("config: DATABASE_URL is required")
	}
	if c.JWT.SecretKey == "" {
		return errors.New("config: JWT_SECRET_KEY is required")
	}
	if len(c.JWT.SecretKey) < 32 {
		return errors.New("config: JWT_SECRET_KEY must be at least 32 bytes (256 bits)")
	}
	if c.Env == "production" && c.AutoMigrate {
		return errors.New("config: AUTOMIGRATE must be false in production (#35)")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("config: invalid PORT %d", c.Port)
	}
	return nil
}

// IsProduction reports whether the runtime is configured for production.
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}

// --- helpers ---

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return fallback
}
