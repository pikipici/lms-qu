// Timer expire cron for Ulangan Bab — Task 5.D.4.
//
// Periodically scans hasil_soal_bab rows where mode='ulangan',
// status='berlangsung', and deadline_at <= now(), then auto-grades
// each row using the same logic as UlanganService.Submit. The cron
// shares a single advisory lock key per hasil_id with Submit so siswa
// klik submit dan cron sweep mutually exclusive.
//
// Locked decision #80: tick interval 30s, batch limit 100,
// FOR UPDATE SKIP LOCKED so concurrent ticks (rolling deploy edge case)
// don't double-grade.
//
// Pattern adopted from skill `go-cleanup-cron-ctx-bound`:
//   - rootCtx-bound goroutine started after DB ready in main.go.
//   - Initial sweep on boot (catches downtime backlog).
//   - Per-row best-effort: errors counted, never abort the tick.
//   - Logs `[timer-cron] swept N expired (graded=X errors=Y)` on
//     non-zero ticks.
//
// Idempotent contract:
//   - Inside per-row tx, after lock acquire, re-check status; if
//     row was finalized by Submit (siswa beat us), skip + commit empty.
//   - selesai_at = deadline_at (NOT now) — locked policy: cron records
//     the moment the timer actually expired, not the sweep moment.
package soalbab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TimerCronInterval — how often the cron sweeps for expired attempts.
// Locked #80 → 30 seconds.
const TimerCronInterval = 30 * time.Second

// TimerCronBatchSize — max rows touched per tick. Keeps each tick
// bounded so per-row failures can't starve newly-expiring attempts.
const TimerCronBatchSize = 100

// timerCronRepo is the subset of *Repo TimerCron depends on.
type timerCronRepo interface {
	DB() *gorm.DB
}

// TimerCron auto-grades expired ulangan attempts.
type TimerCron struct {
	repo timerCronRepo
	now  func() time.Time
}

// NewTimerCron wires a TimerCron. now() injection seam for tests.
func NewTimerCron(repo *Repo) *TimerCron {
	return &TimerCron{repo: repo, now: time.Now}
}

// SetClock replaces the cron's clock (test hook).
func (c *TimerCron) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	c.now = now
}

// TickResult captures per-tick stats. Returned by RunOnce so tests can
// assert; production logging derives directly from it.
type TickResult struct {
	Scanned int
	Graded  int
	Skipped int // already finalized between scan + lock
	Errors  int
}

// RunOnce performs one sweep pass. Best-effort: per-row errors are
// counted + logged but never abort the tick. Returns the aggregated
// joined error from per-row failures (nil on full success).
func (c *TimerCron) RunOnce(ctx context.Context) (TickResult, error) {
	res := TickResult{}
	if c == nil || c.repo == nil {
		return res, nil
	}

	// Pick candidate ids using FOR UPDATE SKIP LOCKED so concurrent
	// ticks (e.g. rolling deploy with two instances briefly overlapping)
	// don't fight for the same rows. The OUTER select returns ids only;
	// the per-row tx below acquires the advisory lock + re-checks.
	var ids []uuid.UUID
	now := c.now()
	if err := c.repo.DB().WithContext(ctx).
		Raw(`
			SELECT id FROM hasil_soal_bab
			 WHERE status = ?
			   AND mode   = ?
			   AND deadline_at IS NOT NULL
			   AND deadline_at <= ?
			 ORDER BY deadline_at
			 LIMIT ?
			 FOR UPDATE SKIP LOCKED
		`, HasilBerlangsung, HasilModeUlangan, now, TimerCronBatchSize).
		Scan(&ids).Error; err != nil {
		return res, fmt.Errorf("soalbab timer-cron scan: %w", err)
	}
	res.Scanned = len(ids)
	if len(ids) == 0 {
		return res, nil
	}

	var perRowErrs []error
	for _, id := range ids {
		// Stop early if shutting down — current row gets re-picked next
		// boot's initial sweep.
		if ctx.Err() != nil {
			break
		}
		graded, err := c.gradeOne(ctx, id)
		if err != nil {
			res.Errors++
			perRowErrs = append(perRowErrs, fmt.Errorf("hasil=%s: %w", id, err))
			slog.Warn("soalbab timer-cron grade failed",
				slog.String("hasil_id", id.String()),
				slog.String("err", err.Error()))
			continue
		}
		if graded {
			res.Graded++
		} else {
			res.Skipped++
		}
	}
	return res, errors.Join(perRowErrs...)
}

// gradeOne grades a single hasil row inside a fresh tx with the same
// advisory lock key as Submit. Returns (graded, err) where graded=false
// means the row was already finalized (idempotent skip), graded=true
// means we actually wrote the grade.
func (c *TimerCron) gradeOne(ctx context.Context, hasilID uuid.UUID) (bool, error) {
	tx := c.repo.DB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return false, fmt.Errorf("tx begin: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	key := hasilLockKey(hasilID)
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", key).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("advisory lock: %w", err)
	}

	var hasil HasilSoalBab
	if err := tx.Where("id = ?", hasilID).First(&hasil).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Row deleted between scan + lock — skip cleanly.
			return false, nil
		}
		return false, fmt.Errorf("reload hasil: %w", err)
	}
	// Idempotent skip: Submit already finalized this row, atau a
	// previous tick of cron already graded it (would be unusual since
	// the OUTER select filters status='berlangsung').
	if hasil.Status != HasilBerlangsung {
		if err := tx.Commit().Error; err != nil {
			return false, fmt.Errorf("commit skip: %w", err)
		}
		return false, nil
	}
	if hasil.Mode != HasilModeUlangan {
		// Defensive: scan filter sudah enforce mode='ulangan' tapi tx
		// reload defensive in case schema migration mid-flight.
		if err := tx.Commit().Error; err != nil {
			return false, fmt.Errorf("commit non-ulangan: %w", err)
		}
		return false, nil
	}
	if hasil.DeadlineAt == nil {
		tx.Rollback()
		return false, errors.New("hasil missing deadline_at")
	}

	pool, perr := decodeSoalIDsJSON(hasil.SoalIDsJSON)
	if perr != nil {
		tx.Rollback()
		return false, fmt.Errorf("pool decode: %w", perr)
	}
	if len(pool) == 0 {
		tx.Rollback()
		return false, errors.New("empty pool snapshot")
	}

	var jawabans []JawabanBab
	if err := tx.Where("hasil_id = ?", hasilID).Find(&jawabans).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("load jawabans: %w", err)
	}

	benar, nilaiTotal, err := gradeAttemptInTx(tx, hasilID, pool, jawabans)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	jumlahTotal := int16(len(pool))
	// Locked policy: selesai_at = deadline_at (timer expired moment),
	// NOT now() — preserves the actual deadline truth for rekap UI.
	finalSelesaiAt := *hasil.DeadlineAt

	if err := tx.Model(&HasilSoalBab{}).
		Where("id = ?", hasilID).
		Updates(map[string]any{
			"status":              HasilSelesai,
			"selesai_at":          finalSelesaiAt,
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"updated_at":          gorm.Expr("now()"),
		}).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("update hasil: %w", err)
	}

	if err := tx.Create(&EventBab{
		HasilID: hasilID,
		Action:  "ulangan_bab_auto_graded",
		Meta: marshalMeta(map[string]any{
			"reason":              "timer_expired",
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"deadline_at":         hasil.DeadlineAt.UTC().Format(time.RFC3339Nano),
		}),
	}).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("event append: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// Run starts the periodic ticker. Returns when ctx is canceled. Runs
// an initial sweep immediately on entry so a freshly-restarted service
// catches the backlog without waiting a full interval.
func (c *TimerCron) Run(ctx context.Context) {
	if c == nil || c.repo == nil {
		slog.Warn("soalbab timer-cron: not configured, exit")
		return
	}
	slog.Info("soalbab timer-cron: starting",
		slog.Duration("interval", TimerCronInterval),
		slog.Int("batch", TimerCronBatchSize))

	// Initial sweep — catch up after restart.
	if res, err := c.RunOnce(ctx); err != nil {
		slog.Warn("soalbab timer-cron: initial sweep error",
			slog.Int("scanned", res.Scanned),
			slog.Int("graded", res.Graded),
			slog.Int("errors", res.Errors),
			slog.String("err", err.Error()))
	} else if res.Scanned > 0 {
		slog.Info("soalbab timer-cron: initial swept",
			slog.Int("scanned", res.Scanned),
			slog.Int("graded", res.Graded),
			slog.Int("skipped", res.Skipped))
	}

	t := time.NewTicker(TimerCronInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("soalbab timer-cron: shutdown")
			return
		case <-t.C:
			res, err := c.RunOnce(ctx)
			if err != nil {
				slog.Warn("soalbab timer-cron: sweep error",
					slog.Int("scanned", res.Scanned),
					slog.Int("graded", res.Graded),
					slog.Int("errors", res.Errors),
					slog.String("err", err.Error()))
			}
			if res.Scanned > 0 {
				slog.Info("soalbab timer-cron: swept",
					slog.Int("scanned", res.Scanned),
					slog.Int("graded", res.Graded),
					slog.Int("skipped", res.Skipped),
					slog.Int("errors", res.Errors))
			}
		}
	}
}
