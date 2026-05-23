package cleanup

import (
	"context"
	"testing"
	"time"
)

func TestRunOnceNilDBReturnsUnavailableItems(t *testing.T) {
	now := time.Date(2026, 5, 23, 8, 0, 0, 0, time.UTC)
	report, err := NewService(nil).RunOnce(context.Background(), Options{Now: now})
	if err == nil {
		t.Fatal("expected nil DB error")
	}
	if !report.DryRun {
		t.Fatal("report must always be dry-run")
	}
	if !report.GeneratedAt.Equal(now) {
		t.Fatalf("GeneratedAt = %s, want %s", report.GeneratedAt, now)
	}
	if len(report.Items) != 4 {
		t.Fatalf("items len = %d, want 4", len(report.Items))
	}
	for _, item := range report.Items {
		if item.Available {
			t.Fatalf("%s available=true, want false", item.Scope)
		}
		if item.Reason != "db not configured" {
			t.Fatalf("%s reason = %q", item.Scope, item.Reason)
		}
	}
}
