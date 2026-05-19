// Package db sets up the GORM connection pool used by the LMS backend.
//
// In Fase 0 this only opens the connection and verifies it via Ping. Schema
// migration is handled by golang-migrate (locked decision #35). Models will
// be added progressively starting Fase 1.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pikip/lms/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Open returns a configured *gorm.DB ready to use, plus a close func that
// callers should defer.
func Open(ctx context.Context, cfg *config.Config) (*gorm.DB, func() error, error) {
	gormCfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormLogLevel(cfg)),
		NowFunc: func() time.Time {
			return time.Now().In(cfg.Timezone)
		},
	}

	gdb, err := gorm.Open(postgres.Open(cfg.Database.URL), gormCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("db: open: %w", err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("db: pool handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetimeMin) * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("db: ping: %w", err)
	}

	slog.Info("db connected",
		"max_open", cfg.Database.MaxOpenConns,
		"max_idle", cfg.Database.MaxIdleConns,
	)
	return gdb, sqlDB.Close, nil
}

// Ping is used by /readyz to verify the database is reachable.
func Ping(ctx context.Context, gdb *gorm.DB) error {
	if gdb == nil {
		return fmt.Errorf("db: not initialised")
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return sqlDB.PingContext(pingCtx)
}

func gormLogLevel(cfg *config.Config) gormlogger.LogLevel {
	if cfg.IsProduction() {
		return gormlogger.Warn
	}
	return gormlogger.Info
}
