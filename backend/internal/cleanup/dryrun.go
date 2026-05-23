// Package cleanup provides conservative dry-run reports for retention jobs.
package cleanup

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	ScopeLoginAttempts       = "login_attempts_old"
	ScopeRefreshTokens       = "refresh_tokens_expired_revoked"
	ScopeHasilSoalBabDeleted = "hasil_soal_bab_deleted_old"
	ScopeHasilUjianDeleted   = "hasil_ujian_deleted_old"
)

// Report is the structured output of a dry-run cleanup pass. It only contains
// counts and metadata; no cleanup package code deletes data.
type Report struct {
	GeneratedAt time.Time    `json:"generated_at"`
	DryRun      bool         `json:"dry_run"`
	Items       []ReportItem `json:"items"`
}

// ReportItem describes one retention scope. Unavailable scopes are expected
// while older schemas do not yet have the needed soft-delete columns.
type ReportItem struct {
	Scope          string     `json:"scope"`
	CandidateCount int64      `json:"candidate_count"`
	Cutoff         *time.Time `json:"cutoff,omitempty"`
	Available      bool       `json:"available"`
	Reason         string     `json:"reason,omitempty"`
}

type Options struct {
	Now time.Time
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

// RunOnce counts retention candidates. It is intentionally dry-run only;
// destructive cleanup must live behind a separate explicit implementation.
func (s *Service) RunOnce(ctx context.Context, opts Options) (Report, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	report := Report{GeneratedAt: now, DryRun: true}
	var firstErr error
	add := func(item ReportItem, err error) {
		report.Items = append(report.Items, item)
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}

	loginCutoff := now.AddDate(0, 0, -30)
	add(s.countWhere(ctx, ScopeLoginAttempts, "login_attempts", "at", loginCutoff,
		"at < ?", loginCutoff))

	refreshCutoff := now.AddDate(0, 0, -7)
	add(s.countWhere(ctx, ScopeRefreshTokens, "refresh_tokens", "expires_at", refreshCutoff,
		"(expires_at < ? OR revoked_at IS NOT NULL) AND COALESCE(revoked_at, expires_at) < ?", now, refreshCutoff))

	hasilCutoff := now.AddDate(-1, 0, 0)
	add(s.countWhere(ctx, ScopeHasilSoalBabDeleted, "hasil_soal_bab", "deleted_at", hasilCutoff,
		"deleted_at IS NOT NULL AND deleted_at < ?", hasilCutoff))
	add(s.countWhere(ctx, ScopeHasilUjianDeleted, "hasil_ujian", "deleted_at", hasilCutoff,
		"deleted_at IS NOT NULL AND deleted_at < ?", hasilCutoff))

	return report, firstErr
}

func (s *Service) countWhere(ctx context.Context, scope, table, requiredColumn string, cutoff time.Time, where string, args ...any) (ReportItem, error) {
	item := ReportItem{Scope: scope, Cutoff: &cutoff, Available: true}
	if s == nil || s.db == nil {
		item.Available = false
		item.Reason = "db not configured"
		return item, fmt.Errorf("cleanup: %s: db not configured", scope)
	}
	if ok, err := s.tableExists(ctx, table); err != nil {
		item.Available = false
		item.Reason = err.Error()
		return item, err
	} else if !ok {
		item.Available = false
		item.Reason = "table missing"
		return item, nil
	}
	if ok, err := s.columnExists(ctx, table, requiredColumn); err != nil {
		item.Available = false
		item.Reason = err.Error()
		return item, err
	} else if !ok {
		item.Available = false
		item.Reason = "required column missing: " + requiredColumn
		return item, nil
	}

	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", table, where)
	if err := s.db.WithContext(ctx).Raw(query, args...).Scan(&item.CandidateCount).Error; err != nil {
		item.Available = false
		item.Reason = err.Error()
		return item, err
	}
	return item, nil
}

func (s *Service) tableExists(ctx context.Context, table string) (bool, error) {
	var exists bool
	err := s.db.WithContext(ctx).Raw("SELECT to_regclass(?) IS NOT NULL", "public."+table).Scan(&exists).Error
	return exists, err
}

func (s *Service) columnExists(ctx context.Context, table, column string) (bool, error) {
	var exists bool
	err := s.db.WithContext(ctx).Raw(`
SELECT EXISTS (
  SELECT 1
  FROM information_schema.columns
  WHERE table_schema = 'public' AND table_name = ? AND column_name = ?
)`, table, column).Scan(&exists).Error
	return exists, err
}
