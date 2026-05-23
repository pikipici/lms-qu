// cmd/cleanup-dry-run prints retention cleanup candidate counts without deleting anything.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/pikip/lms/backend/internal/cleanup"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
)

func main() {
	if err := run(); err != nil {
		slog.Error("cleanup-dry-run", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	format := flag.String("format", "text", "output format: text or json")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gdb, closeDB, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()

	report, err := cleanup.NewService(gdb).RunOnce(ctx, cleanup.Options{})
	if err != nil {
		slog.Warn("cleanup dry-run completed with partial errors", slog.String("err", err.Error()))
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	case "text":
		fmt.Printf("cleanup dry-run generated_at=%s dry_run=%t\n", report.GeneratedAt.Format("2006-01-02T15:04:05Z07:00"), report.DryRun)
		for _, item := range report.Items {
			if item.Available {
				fmt.Printf("%-32s candidates=%d cutoff=%s\n", item.Scope, item.CandidateCount, item.Cutoff.Format("2006-01-02T15:04:05Z07:00"))
				continue
			}
			fmt.Printf("%-32s unavailable reason=%q\n", item.Scope, item.Reason)
		}
		return nil
	default:
		return fmt.Errorf("unsupported --format %q", *format)
	}
}
