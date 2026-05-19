// cmd/reset-admin is the emergency password reset for an admin account
// (locked decision #53). It bypasses the normal login flow and writes a new
// bcrypt hash directly. Trust boundary: SSH access to the server.
//
// Usage:
//
//	./bin/reset-admin --email admin@sekolah.id
//	./bin/reset-admin --email admin@sekolah.id --password new-secret-123
//
// Fase 0 status: stub. Wires config + DB; the actual UPDATE lands in Fase 1
// once the User model exists. AuditLog row will use actor_id=NULL with
// action='admin_reset_via_cli' (#53).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/db"
	"golang.org/x/term"
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
	if email == "" {
		flag.Usage()
		return errors.New("reset-admin: --email is required")
	}
	if !strings.Contains(email, "@") {
		return errors.New("reset-admin: email looks malformed")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, closeDB, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()

	password := *pwFlag
	if password == "" {
		fmt.Fprint(os.Stderr, "New password: ")
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(fd) {
			return errors.New("reset-admin: stdin is not a TTY; pass --password or run interactively")
		}
		b, perr := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if perr != nil {
			return fmt.Errorf("read password: %w", perr)
		}
		password = string(b)
	}
	if len(password) < 8 {
		return errors.New("reset-admin: password must be at least 8 characters")
	}

	// TODO(Fase 1): real flow:
	//   1. SELECT id, role FROM users WHERE email = $1; ensure role='admin'.
	//   2. bcrypt password.
	//   3. UPDATE users SET password_hash=$1, must_change_password=TRUE,
	//      failed_login_count=0, status='active' WHERE id=$2.
	//   4. INSERT INTO refresh_tokens revoke-all for this user (#53).
	//   5. AuditLog: actor_id=NULL, action='admin_reset_via_cli', target_user_id=$2.
	slog.Info("reset-admin (stub)",
		slog.String("email", email),
		slog.Int("password_len", len(password)),
		slog.String("note", "real update happens in Fase 1; this run only validated config + DB"),
	)
	fmt.Println("reset-admin: configuration & DB OK. Update logic lands in Fase 1.")
	return nil
}
