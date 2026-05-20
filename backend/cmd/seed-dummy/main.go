// cmd/seed-dummy populates dev/QA databases with realistic-looking
// users, audit logs, and login attempts so the admin panel has data
// to render.
//
// Idempotent: re-running keeps existing dummy users (matched by email)
// and only appends new audit/login_attempt rows. Safe to invoke many
// times during QA cycles.
//
// Usage (server, after seed-admin):
//
//	cd /home/ubuntu/lms/backend
//	go run ./cmd/seed-dummy
//
// Locked decisions:
//   - #11 No public self-register: this is a dev-only data seeder, gated
//     behind APP_ENV != "production".
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type dummyUser struct {
	Name     string
	Email    string
	Role     auth.UserRole
	Password string // pre-set, so QA can log in
}

var dummies = []dummyUser{
	{Name: "Budi Guru", Email: "guru1@sekolah.id", Role: auth.Guru, Password: "guru1pass"},
	{Name: "Citra Guru", Email: "guru2@sekolah.id", Role: auth.Guru, Password: "guru2pass"},
	{Name: "Dewi Siswa", Email: "siswa1@sekolah.id", Role: auth.Siswa, Password: "siswa1pass"},
	{Name: "Eko Siswa", Email: "siswa2@sekolah.id", Role: auth.Siswa, Password: "siswa2pass"},
	{Name: "Fani Siswa", Email: "siswa3@sekolah.id", Role: auth.Siswa, Password: "siswa3pass"},
}

func main() {
	if err := run(); err != nil {
		slog.Error("seed-dummy", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		return errors.New("seed-dummy: refusing to run with APP_ENV=production")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gdb, closeDB, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()

	repo := auth.NewRepo(gdb)

	// We need a primary admin to attribute audit events to. If none
	// exists yet, abort — run cmd/seed-admin first.
	adminCount, err := repo.CountAdmins(ctx)
	if err != nil {
		return fmt.Errorf("count admins: %w", err)
	}
	if adminCount == 0 {
		return errors.New("seed-dummy: no admin user found; run cmd/seed-admin first")
	}

	admin, err := findFirstAdmin(ctx, gdb)
	if err != nil {
		return fmt.Errorf("find admin: %w", err)
	}
	logger.Info("seed-dummy: actor admin", slog.String("email", admin.Email), slog.String("id", admin.ID.String()))

	// 1. Ensure dummy users.
	created := 0
	skipped := 0
	createdUsers := make([]auth.User, 0, len(dummies))
	for _, d := range dummies {
		existing, ferr := repo.FindUserByEmail(ctx, d.Email)
		if ferr == nil && existing != nil {
			createdUsers = append(createdUsers, *existing)
			skipped++
			continue
		}
		if ferr != nil && !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find user %s: %w", d.Email, ferr)
		}

		hash, herr := auth.HashPassword(d.Password, cfg.JWT.BcryptCost)
		if herr != nil {
			return fmt.Errorf("hash password %s: %w", d.Email, herr)
		}
		newU := &auth.User{
			Name:               d.Name,
			Email:              strings.ToLower(d.Email),
			PasswordHash:       hash,
			Role:               d.Role,
			Status:             auth.Active,
			MustChangePassword: true,
			CreatedByID:        &admin.ID,
		}
		if cerr := repo.CreateUser(ctx, newU); cerr != nil {
			return fmt.Errorf("create %s: %w", d.Email, cerr)
		}

		// Audit: admin_user_created (matches handler behavior).
		_ = repo.LogAudit(ctx, &auth.AuditLog{
			ActorID:    &admin.ID,
			ActorRole:  ptrStr(string(auth.Admin)),
			Action:     "admin_user_created",
			TargetType: ptrStr("user"),
			TargetID:   &newU.ID,
			Meta: datatypes.JSON([]byte(fmt.Sprintf(
				`{"email":%q,"role":%q,"strategy":"manual"}`, newU.Email, string(newU.Role),
			))),
			At: time.Now().Add(-time.Duration(created+1) * 6 * time.Hour),
		})
		createdUsers = append(createdUsers, *newU)
		created++
	}
	logger.Info("seed-dummy: dummy users", slog.Int("created", created), slog.Int("skipped", skipped))

	// 2. Generate login attempts (mix of success/failure across last 7 days).
	loginAttempts := 0
	for i, u := range createdUsers {
		// Successful login
		_ = repo.LogLoginAttempt(ctx, &auth.LoginAttempt{
			Email:     u.Email,
			IP:        ptrStr(fakeIP(i, 0)),
			UserAgent: ptrStr(fakeUA(i)),
			Success:   true,
			At:        time.Now().Add(-time.Duration(i*8) * time.Hour),
		})
		loginAttempts++

		// 1-2 failed attempts before success
		fails := 1
		if i%2 == 0 {
			fails = 2
		}
		for j := 0; j < fails; j++ {
			_ = repo.LogLoginAttempt(ctx, &auth.LoginAttempt{
				Email:     u.Email,
				IP:        ptrStr(fakeIP(i, j+1)),
				UserAgent: ptrStr(fakeUA(i)),
				Success:   false,
				Reason:    ptrStr("invalid_password"),
				At:        time.Now().Add(-time.Duration(i*8+j+1) * time.Hour),
			})
			loginAttempts++
		}
	}

	// Some random failed attempts on bogus emails (attack-like).
	for i, em := range []string{"hacker@example.com", "admin@root.local", "test@test.test"} {
		_ = repo.LogLoginAttempt(ctx, &auth.LoginAttempt{
			Email:     em,
			IP:        ptrStr(fmt.Sprintf("203.0.113.%d", 10+i)),
			UserAgent: ptrStr("curl/8.4.0"),
			Success:   false,
			Reason:    ptrStr("user_not_found"),
			At:        time.Now().Add(-time.Duration(2+i) * time.Hour),
		})
		loginAttempts++
	}
	logger.Info("seed-dummy: login attempts", slog.Int("inserted", loginAttempts))

	// 3. Sample admin actions audit (only if we just created them; idempotent re-runs skip).
	auditExtras := 0
	if created > 0 && len(createdUsers) >= 2 {
		// Pretend admin reset password for first dummy
		first := createdUsers[0]
		_ = repo.LogAudit(ctx, &auth.AuditLog{
			ActorID:    &admin.ID,
			ActorRole:  ptrStr(string(auth.Admin)),
			Action:     "admin_user_password_reset",
			TargetType: ptrStr("user"),
			TargetID:   &first.ID,
			Meta:       datatypes.JSON([]byte(`{"strategy":"generate"}`)),
			At:         time.Now().Add(-2 * time.Hour),
		})
		auditExtras++

		// Pretend admin revoked sessions for a siswa
		var siswa *auth.User
		for i := range createdUsers {
			if createdUsers[i].Role == auth.Siswa {
				siswa = &createdUsers[i]
				break
			}
		}
		if siswa != nil {
			_ = repo.LogAudit(ctx, &auth.AuditLog{
				ActorID:    &admin.ID,
				ActorRole:  ptrStr(string(auth.Admin)),
				Action:     "admin_user_sessions_revoked",
				TargetType: ptrStr("user"),
				TargetID:   &siswa.ID,
				Meta:       datatypes.JSON([]byte(`{"revoked_count":2,"reason":"perangkat hilang"}`)),
				At:         time.Now().Add(-30 * time.Minute),
			})
			auditExtras++
		}
	}
	logger.Info("seed-dummy: extra audit events", slog.Int("inserted", auditExtras))

	fmt.Println("seed-dummy: done")
	for _, d := range dummies {
		fmt.Printf("  %s  password=%s  role=%s\n", d.Email, d.Password, d.Role)
	}
	return nil
}

func findFirstAdmin(ctx context.Context, gdb *gorm.DB) (*auth.User, error) {
	var u auth.User
	if err := gdb.WithContext(ctx).
		Where("role = ?", string(auth.Admin)).
		Order("created_at ASC").
		First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func ptrStr(s string) *string { return &s }

func fakeIP(i, j int) string {
	octets := []int{100 + i*7%150, 50 + j*3%150, 1 + (i*j)%200, 1 + (i+j)%240}
	return fmt.Sprintf("10.%d.%d.%d", octets[1], octets[2], octets[3])
}

func fakeUA(i int) string {
	uas := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Mobile Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	}
	return uas[i%len(uas)]
}
