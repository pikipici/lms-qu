// cmd/seed-admin bootstraps the very first admin user.
//
// Locked decisions:
//   - #11 No public self-register
//   - #17.1 / #53 Admin lock-out recovery: this CLI runs only when there
//     is no admin yet. Subsequent admin creation goes through /admin/users.
//
// Usage (env vars, recommended on server):
//
//	ADMIN_EMAIL=admin@sekolah.id ADMIN_PASSWORD='ganti-cepat-123' ./bin/seed-admin
//
// Usage (interactive, dev):
//
//	go run ./cmd/seed-admin
//
// Fase 0 status: this binary connects to the DB and validates configuration.
// The actual user insertion lands in Fase 1 once the User model is committed.
// Until then it prints the planned action so we can verify the wiring.
package main

import (
	"bufio"
	"context"
	"errors"
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
		slog.Error("seed-admin", slog.String("err", err.Error()))
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gdb, closeDB, err := db.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeDB() }()
	_ = gdb // Fase 1 will use it to insert the user.

	email, password, name, err := collectInputs(cfg)
	if err != nil {
		return err
	}

	if err := validateAdminInput(email, password); err != nil {
		return err
	}

	// TODO(Fase 1): replace stub with real flow once User model exists:
	//   1. SELECT count from users WHERE role='admin'; reject if >0.
	//   2. bcrypt password (cost from cfg.JWT.BcryptCost).
	//   3. INSERT user with role=admin, status=active, must_change_password=true.
	//   4. AuditLog: actor_id=NULL, action='admin_seeded'.
	logger.Info("seed-admin (stub)",
		slog.String("email", email),
		slog.String("name", name),
		slog.Int("password_len", len(password)),
		slog.String("note", "user insertion happens in Fase 1; this run only validated config + DB"),
	)
	fmt.Println("seed-admin: configuration & DB OK. Insert logic lands in Fase 1.")
	return nil
}

func collectInputs(cfg *config.Config) (email, password, name string, err error) {
	email = strings.TrimSpace(cfg.SeedAdminEmail)
	password = cfg.SeedAdminPassword
	name = strings.TrimSpace(cfg.SeedAdminName)
	if name == "" {
		name = "Administrator"
	}

	if email != "" && password != "" {
		return email, password, name, nil
	}

	// Interactive fallback. Prompts use stderr so they don't pollute stdout
	// (handy if the binary is wrapped by another tool).
	reader := bufio.NewReader(os.Stdin)

	if email == "" {
		fmt.Fprint(os.Stderr, "Admin email: ")
		line, rerr := reader.ReadString('\n')
		if rerr != nil {
			return "", "", "", fmt.Errorf("read email: %w", rerr)
		}
		email = strings.TrimSpace(line)
	}

	if password == "" {
		fmt.Fprint(os.Stderr, "Admin password: ")
		pw, rerr := readPassword()
		if rerr != nil {
			return "", "", "", fmt.Errorf("read password: %w", rerr)
		}
		fmt.Fprintln(os.Stderr)
		password = pw
	}

	if cfg.SeedAdminName == "" {
		fmt.Fprintf(os.Stderr, "Admin name [%s]: ", name)
		line, rerr := reader.ReadString('\n')
		if rerr != nil {
			return "", "", "", fmt.Errorf("read name: %w", rerr)
		}
		if v := strings.TrimSpace(line); v != "" {
			name = v
		}
	}
	return email, password, name, nil
}

func readPassword() (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		return string(b), err
	}
	// Non-TTY (CI / piped input): read a line.
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n"), err
}

func validateAdminInput(email, password string) error {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return errors.New("seed-admin: email is required and must look like an email")
	}
	if len(password) < 8 {
		return errors.New("seed-admin: password must be at least 8 characters")
	}
	return nil
}
