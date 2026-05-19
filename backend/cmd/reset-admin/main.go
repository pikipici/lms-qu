// cmd/reset-admin is the emergency password reset for an admin account
// (locked decision #53). It bypasses the normal login flow and writes a new
// bcrypt hash directly. Trust boundary: SSH access to the server.
//
// Usage:
//
//	./bin/reset-admin --email admin@sekolah.id
//	./bin/reset-admin --email admin@sekolah.id --password new-secret-123
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"golang.org/x/term"
	"gorm.io/gorm"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reset-admin", slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	emailFlag := flag.String("email", "", "admin email to reset (required)")
	pwFlag := flag.String("password", "", "new password; if empty, prompted via TTY")
	flag.Parse()

	email := strings.TrimSpace(*emailFlag)
	if err := validateResetEmail(email); err != nil {
		if email == "" {
			flag.Usage()
		}
		return err
	}

	password := *pwFlag
	if password == "" {
		var err error
		password, err = promptPassword()
		if err != nil {
			return err
		}
	}

	if err := validateResetInput(email, password); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gdb, closeDB, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()

	repo := auth.NewRepo(gdb)

	user, err := repo.FindUserByEmail(ctx, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("reset-admin: no user with that email")
	}
	if err != nil {
		return fmt.Errorf("reset-admin: find user by email: %w", err)
	}

	if user.Role != auth.Admin {
		return fmt.Errorf("reset-admin: user %s is not an admin (role=%s); refusing", email, user.Role)
	}

	hash, err := auth.HashPassword(password, cfg.JWT.BcryptCost)
	if err != nil {
		return fmt.Errorf("reset-admin: hash password: %w", err)
	}

	if err := repo.UpdateUserPassword(ctx, user.ID, hash); err != nil {
		return fmt.Errorf("reset-admin: update password: %w", err)
	}

	if user.Status == auth.Locked {
		// TODO(#53): unlock if locked - requires new repo method.
		// UpdateUserPassword clears must_change_password, but not status.
		logger.Warn("reset-admin: user remains locked after password reset",
			slog.String("email", user.Email),
			slog.String("user_id", user.ID.String()),
		)
	}

	if err := repo.ResetFailedLogin(ctx, user.ID); err != nil {
		logger.Warn("reset-admin: reset failed login failed",
			slog.String("email", user.Email),
			slog.String("user_id", user.ID.String()),
			slog.String("err", err.Error()),
		)
	}

	revokedCount, err := repo.RevokeAllRefreshByUser(ctx, user.ID, auth.AdminReset)
	if err != nil {
		logger.Warn("reset-admin: revoke refresh tokens failed",
			slog.String("email", user.Email),
			slog.String("user_id", user.ID.String()),
			slog.String("err", err.Error()),
		)
	}
	logger.Info("reset-admin: revoked refresh tokens",
		slog.String("email", user.Email),
		slog.String("user_id", user.ID.String()),
		slog.Int64("revoked_count", revokedCount),
	)

	action := "admin_reset_via_cli"
	targetType := "user"
	var targetID uuid.UUID = user.ID
	if err := repo.LogAudit(ctx, &auth.AuditLog{
		ActorID:    nil,
		Action:     action,
		TargetType: &targetType,
		TargetID:   &targetID,
		At:         time.Now(),
	}); err != nil {
		logger.Warn("reset-admin: audit log failed",
			slog.String("email", user.Email),
			slog.String("user_id", user.ID.String()),
			slog.String("action", action),
			slog.String("err", err.Error()),
		)
	}

	fmt.Printf("reset-admin: password reset for %s (id=%s). Revoked %d refresh tokens. User must log in again with new password.\n", user.Email, user.ID, revokedCount)
	return nil
}

func validateResetEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("reset-admin: --email is required")
	}
	if !strings.Contains(email, "@") {
		return errors.New("reset-admin: email looks malformed")
	}
	return nil
}

func validateResetInput(email, password string) error {
	if err := validateResetEmail(email); err != nil {
		return err
	}
	if len(password) < 8 {
		return errors.New("reset-admin: password must be at least 8 characters")
	}
	return nil
}

func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "New password: ")
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", errors.New("reset-admin: stdin is not a TTY; pass --password or run interactively")
	}
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}
