// Timer expire cron for Ujian — Task 6.D.4.
//
// Periodically scans hasil_ujian rows where status='berlangsung' and
// deadline_at <= now(), then auto-grades each row using the same logic
// as FlowService.Submit. The cron shares a single advisory lock key per
// hasil_id with Submit (submitLockKey) so siswa klik submit and cron
// sweep are mutually exclusive.
//
// Locked decision #87 — cron reuse + advisory lock + 30s tick.
//
// Pattern adopted from soalbab.TimerCron (commit 2587526):
//   - rootCtx-bound goroutine started after FlowService construction.
//   - Initial sweep on boot (catches downtime backlog).
//   - Per-row best-effort: errors counted, never abort the tick.
//   - Logs `[ujian timer-cron] swept N expired (graded=X errors=Y)` on
//     non-zero ticks.
//
// Idempotent contract:
//   - Inside per-row tx, after lock acquire, re-check status; if
//     row was finalized by Submit (siswa beat us), skip + commit empty.
//   - selesai_at = deadline_at (NOT now) — locked policy: cron records
//     the moment the timer actually expired, not the sweep moment.
//   - Action 'auto_grade' on EventUjian (Submit uses 'submit') so audit
//     trail differentiates triggers.
package ujian

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/banksoal"
)

// UjianTimerCronInterval — how often the cron sweeps for expired
// attempts. Locked #87 → 30 seconds (mirror SoalBab #80).
const UjianTimerCronInterval = 30 * time.Second

// UjianTimerCronBatchSize — max rows touched per tick. Keeps each tick
// bounded so per-row failures can't starve newly-expiring attempts.
const UjianTimerCronBatchSize = 100

// timerCronRepoUjian is the subset of *Repo TimerCron depends on.
type timerCronRepoUjian interface {
	DB() *gorm.DB
	ScanExpiredHasilIDs(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
}

// timerCronBank is the subset of banksoal.Repo needed for cron grading.
type timerCronBank interface {
	FindSoalsByIDs(ctx context.Context, ids []uuid.UUID) ([]banksoal.BankSoal, error)
}

// timerCronUjianLookup loads Ujian for review-gating fields.
type timerCronUjianLookup interface {
	FindUjianByID(ctx context.Context, id uuid.UUID) (*Ujian, error)
}

// TimerCron auto-grades expired ujian attempts.
type TimerCron struct {
	repo  timerCronRepoUjian
	bank  timerCronBank
	ujian timerCronUjianLookup
	now   func() time.Time
}

// NewTimerCron wires a TimerCron. now() injection seam for tests.
func NewTimerCron(repo *Repo, bank timerCronBank) *TimerCron {
	return &TimerCron{repo: repo, bank: bank, ujian: repo, now: time.Now}
}

// SetClock replaces the cron's clock (test hook).
func (c *TimerCron) SetClock(now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	c.now = now
}

// TickResult captures per-tick stats.
type TickResult struct {
	Scanned int
	Graded  int
	Skipped int // already finalized between scan + lock
	Errors  int
}

// RunOnce performs one sweep pass. Best-effort: per-row errors are
// counted + logged but never abort the tick.
func (c *TimerCron) RunOnce(ctx context.Context) (TickResult, error) {
	res := TickResult{}
	if c == nil || c.repo == nil {
		return res, nil
	}

	now := c.now()
	ids, err := c.repo.ScanExpiredHasilIDs(ctx, now, UjianTimerCronBatchSize)
	if err != nil {
		return res, fmt.Errorf("ujian timer-cron scan: %w", err)
	}
	res.Scanned = len(ids)
	if len(ids) == 0 {
		return res, nil
	}

	var perRowErrs []error
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		graded, err := c.gradeOne(ctx, id)
		if err != nil {
			res.Errors++
			perRowErrs = append(perRowErrs, fmt.Errorf("hasil=%s: %w", id, err))
			slog.Warn("ujian timer-cron grade failed",
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

	// Same key as Submit (locked #87) — cron and siswa mutex pada row sama.
	key := submitLockKey(hasilID)
	if err := tx.Exec("SELECT pg_advisory_xact_lock(?::bigint)", key).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("advisory lock: %w", err)
	}

	var hasil HasilUjian
	if err := tx.Where("id = ? AND deleted_at IS NULL", hasilID).First(&hasil).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Row deleted/cancelled between scan + lock — skip cleanly.
			return false, nil
		}
		return false, fmt.Errorf("reload hasil: %w", err)
	}
	// Idempotent skip: Submit already finalized, atau previous tick.
	if hasil.Status != HasilBerlangsung {
		if err := tx.Commit().Error; err != nil {
			return false, fmt.Errorf("commit skip: %w", err)
		}
		return false, nil
	}
	if hasil.DeadlineAt == nil {
		tx.Rollback()
		return false, errors.New("hasil missing deadline_at")
	}

	// Decode pool snapshot.
	pool, perr := decodeSoalIDsJSONUjian(hasil.SoalIDsJSON)
	if perr != nil {
		tx.Rollback()
		return false, fmt.Errorf("pool decode: %w", perr)
	}
	if len(pool) == 0 {
		tx.Rollback()
		return false, errors.New("empty pool snapshot")
	}

	// Load BankSoal — source of truth for grading.
	soals, err := c.bank.FindSoalsByIDs(ctx, pool)
	if err != nil {
		tx.Rollback()
		return false, fmt.Errorf("load soals: %w", err)
	}
	soalByID := make(map[uuid.UUID]*banksoal.BankSoal, len(soals))
	for i := range soals {
		soalByID[soals[i].ID] = &soals[i]
	}

	var jawabans []JawabanUjian
	if err := tx.Where("hasil_id = ?", hasilID).Find(&jawabans).Error; err != nil {
		tx.Rollback()
		return false, fmt.Errorf("load jawabans: %w", err)
	}

	benar, nilaiTotal, err := gradeAttemptInTx(tx, jawabans, soalByID)
	if err != nil {
		tx.Rollback()
		return false, err
	}
	jumlahTotal := int16(len(pool))
	// Locked policy: selesai_at = deadline_at (timer expired moment),
	// NOT now() — preserves the actual deadline truth for rekap UI.
	finalSelesaiAt := *hasil.DeadlineAt

	if err := tx.Model(&HasilUjian{}).
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

	if err := tx.Create(&EventUjian{
		HasilID: hasilID,
		Action:  "auto_grade",
		Meta: marshalMeta(map[string]any{
			"reason":              "timer_expired",
			"nilai_total":         nilaiTotal,
			"jawaban_benar_count": benar,
			"jawaban_total":       jumlahTotal,
			"deadline_at":         hasil.DeadlineAt.UTC().Format(time.RFC3339Nano),
			"trigger":             "cron",
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
		slog.Warn("ujian timer-cron: not configured, exit")
		return
	}
	slog.Info("ujian timer-cron: starting",
		slog.Duration("interval", UjianTimerCronInterval),
		slog.Int("batch", UjianTimerCronBatchSize))

	// Initial sweep — catch up after restart.
	if res, err := c.RunOnce(ctx); err != nil {
		slog.Warn("ujian timer-cron: initial sweep error",
			slog.Int("scanned", res.Scanned),
			slog.Int("graded", res.Graded),
			slog.Int("errors", res.Errors),
			slog.String("err", err.Error()))
	} else if res.Scanned > 0 {
		slog.Info("ujian timer-cron: initial swept",
			slog.Int("scanned", res.Scanned),
			slog.Int("graded", res.Graded),
			slog.Int("skipped", res.Skipped))
	}

	t := time.NewTicker(UjianTimerCronInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("ujian timer-cron: shutdown")
			return
		case <-t.C:
			res, err := c.RunOnce(ctx)
			if err != nil {
				slog.Warn("ujian timer-cron: sweep error",
					slog.Int("scanned", res.Scanned),
					slog.Int("graded", res.Graded),
					slog.Int("errors", res.Errors),
					slog.String("err", err.Error()))
			}
			if res.Scanned > 0 {
				slog.Info("ujian timer-cron: swept",
					slog.Int("scanned", res.Scanned),
					slog.Int("graded", res.Graded),
					slog.Int("skipped", res.Skipped),
					slog.Int("errors", res.Errors))
			}
		}
	}
}

// gradeAttemptInTx applies grading rules to all jawaban rows + persists
// updates inside the given tx. Shared between FlowService.Submit (6.D.3,
// siswa-driven) and TimerCron.gradeOne (6.D.4, cron-driven) so grading
// logic stays consistent.
//
// Returns (benarCount, nilaiTotal, error).
func gradeAttemptInTx(tx *gorm.DB, jawabans []JawabanUjian, soalByID map[uuid.UUID]*banksoal.BankSoal) (int16, float64, error) {
	var benar int16
	var nilaiTotal float64
	for i := range jawabans {
		j := &jawabans[i]
		soal, ok := soalByID[j.SoalID]
		falseVal := false
		if !ok {
			// Soal soft-deleted post-snapshot; defensive false.
			j.IsBenar = &falseVal
			j.PoinDapat = 0
		} else if j.Jawaban == nil || *j.Jawaban == "" {
			j.IsBenar = &falseVal
			j.PoinDapat = 0
		} else {
			isBenar := banksoal.Jawaban(*j.Jawaban) == soal.Jawaban
			j.IsBenar = &isBenar
			if isBenar {
				j.PoinDapat = soal.Poin
				benar++
				nilaiTotal += float64(soal.Poin)
			} else {
				j.PoinDapat = 0
			}
		}
		if err := tx.Model(&JawabanUjian{}).
			Where("id = ?", j.ID).
			Updates(map[string]any{
				"is_benar":   j.IsBenar,
				"poin_dapat": j.PoinDapat,
			}).Error; err != nil {
			return 0, 0, fmt.Errorf("grade jawaban: %w", err)
		}
	}
	return benar, nilaiTotal, nil
}
