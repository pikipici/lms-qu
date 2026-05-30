package auth

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openAuthRepoTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("AUTH_REPO_TEST_DSN"))
	if dsn == "" {
		t.Skip("set AUTH_REPO_TEST_DSN to run PostgreSQL auth repo integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	adminDB.SetMaxOpenConns(1)
	adminDB.SetMaxIdleConns(1)
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Fatalf("ping auth repo test db: %v", err)
	}

	schema := fmt.Sprintf("auth_repo_test_%s", strings.ReplaceAll(uuid.NewString(), "-", "_"))
	if _, err := adminDB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create test schema: %v", err)
	}

	dropSchema := func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(cleanupCtx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		_ = adminDB.Close()
	}

	if _, err := adminDB.ExecContext(ctx, `SET search_path TO `+schema+`, public`); err != nil {
		dropSchema()
		t.Fatalf("set search_path: %v", err)
	}
	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "000001_init.up.sql"),
		filepath.Join("..", "..", "migrations", "000002_auth_schema.up.sql"),
	} {
		sqlBytes, err := os.ReadFile(migration)
		if err != nil {
			dropSchema()
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := adminDB.ExecContext(ctx, string(sqlBytes)); err != nil {
			dropSchema()
			t.Fatalf("run migration %s: %v", migration, err)
		}
	}

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: adminDB}), &gorm.Config{})
	if err != nil {
		dropSchema()
		t.Fatalf("open gorm db: %v", err)
	}

	return db, dropSchema
}

func TestRepoIntegration_UserLifecycleAndListFilters(t *testing.T) {
	db, cleanup := openAuthRepoTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepo(db)

	admin := &User{
		Name:         "Admin Repo",
		Email:        "Admin.Repo@example.test",
		PasswordHash: "hash-admin",
		Role:         Admin,
		Status:       Active,
	}
	guru := &User{
		Name:         "Guru Repo",
		Email:        "guru.repo@example.test",
		PasswordHash: "hash-guru",
		Role:         Guru,
		Status:       Active,
	}
	if err := repo.CreateUser(ctx, admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := repo.CreateUser(ctx, guru); err != nil {
		t.Fatalf("create guru: %v", err)
	}

	byEmail, err := repo.FindUserByEmail(ctx, "admin.repo@example.test")
	if err != nil {
		t.Fatalf("find user by citext email: %v", err)
	}
	if byEmail.ID != admin.ID || byEmail.Role != Admin {
		t.Fatalf("unexpected email lookup result: %#v", byEmail)
	}

	if err := repo.UpdateUserName(ctx, guru.ID, "Guru Repo Updated"); err != nil {
		t.Fatalf("update user name: %v", err)
	}
	if err := repo.UpdateUserRole(ctx, guru.ID, Siswa); err != nil {
		t.Fatalf("update user role: %v", err)
	}
	if err := repo.SuspendUser(ctx, guru.ID); err != nil {
		t.Fatalf("suspend user: %v", err)
	}

	users, total, err := repo.ListUsers(ctx, UserListFilter{Role: string(Siswa), Status: string(Suspended), SearchEmail: "GURU.REPO", SearchName: "updated"}, 10, 0)
	if err != nil {
		t.Fatalf("list users with postgres filters: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != guru.ID {
		t.Fatalf("unexpected filtered users total=%d users=%#v", total, users)
	}

	names, err := repo.BulkUserNames(ctx, []uuid.UUID{admin.ID, guru.ID, uuid.New()})
	if err != nil {
		t.Fatalf("bulk user names: %v", err)
	}
	if names[admin.ID] != "Admin Repo" || names[guru.ID] != "Guru Repo Updated" || len(names) != 2 {
		t.Fatalf("unexpected names map: %#v", names)
	}

	count, err := repo.CountAdmins(ctx)
	if err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 admin, got %d", count)
	}
}

func TestRepoIntegration_RefreshTokenLifecycle(t *testing.T) {
	db, cleanup := openAuthRepoTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepo(db)
	user := &User{Name: "Session User", Email: "session@example.test", PasswordHash: "hash", Role: Siswa, Status: Active}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	oldJTI := uuid.New()
	issued := time.Now().Add(-time.Minute).UTC()
	oldToken := &RefreshToken{JTI: oldJTI, UserID: user.ID, IssuedAt: issued, ExpiresAt: time.Now().Add(time.Hour).UTC()}
	if err := repo.IssueRefresh(ctx, oldToken); err != nil {
		t.Fatalf("issue refresh: %v", err)
	}

	newJTI := uuid.New()
	newToken := &RefreshToken{JTI: newJTI, UserID: user.ID, IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(2 * time.Hour).UTC()}
	if err := repo.RotateRefresh(ctx, oldJTI, newToken); err != nil {
		t.Fatalf("rotate refresh: %v", err)
	}

	oldStored, err := repo.FindRefreshByJTI(ctx, oldJTI)
	if err != nil {
		t.Fatalf("find old refresh: %v", err)
	}
	if oldStored.RevokedAt == nil || oldStored.RevokedReason == nil || *oldStored.RevokedReason != string(Rotate) || oldStored.ReplacedByJTI == nil || *oldStored.ReplacedByJTI != newJTI {
		t.Fatalf("old token not rotated correctly: %#v", oldStored)
	}

	sessions, err := repo.ListUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].JTI != newJTI {
		t.Fatalf("expected only replacement session, got %#v", sessions)
	}

	if err := repo.RevokeRefresh(ctx, newJTI, Logout); err != nil {
		t.Fatalf("revoke refresh: %v", err)
	}
	sessions, err = repo.ListUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("list sessions after revoke: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no active sessions after revoke, got %#v", sessions)
	}

	another := &RefreshToken{JTI: uuid.New(), UserID: user.ID, IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour).UTC()}
	if err := repo.IssueRefresh(ctx, another); err != nil {
		t.Fatalf("issue another refresh: %v", err)
	}
	if err := repo.LockUser(ctx, user.ID, "too_many_failures"); err != nil {
		t.Fatalf("lock user: %v", err)
	}
	lockedUser, err := repo.FindUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find locked user: %v", err)
	}
	if lockedUser.Status != Locked {
		t.Fatalf("expected locked status, got %s", lockedUser.Status)
	}
	sessions, err = repo.ListUserSessions(ctx, user.ID)
	if err != nil {
		t.Fatalf("list sessions after lock: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected lock to revoke sessions, got %#v", sessions)
	}
}

func TestRepoIntegration_LoginAttemptsAndAuditFilters(t *testing.T) {
	db, cleanup := openAuthRepoTestDB(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewRepo(db)
	user := &User{Name: "Audit User", Email: "audit@example.test", PasswordHash: "hash", Role: Guru, Status: Active}
	if err := repo.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	ip := "127.0.0.1"
	reason := "bad_password"
	rateLimitedReason := "rate_limited"
	oldAttempt := &LoginAttempt{Email: "audit@example.test", IP: &ip, Success: false, Reason: &reason, At: time.Now().Add(-30 * time.Minute).UTC()}
	recentAttempt := &LoginAttempt{Email: "audit@example.test", IP: &ip, Success: false, Reason: &reason, At: time.Now().Add(-time.Minute).UTC()}
	rateLimitedAttempt := &LoginAttempt{Email: "audit@example.test", IP: &ip, Success: false, Reason: &rateLimitedReason, At: time.Now().Add(-time.Minute).UTC()}
	if err := repo.LogLoginAttempt(ctx, oldAttempt); err != nil {
		t.Fatalf("log old attempt: %v", err)
	}
	if err := repo.LogLoginAttempt(ctx, recentAttempt); err != nil {
		t.Fatalf("log recent attempt: %v", err)
	}
	if err := repo.LogLoginAttempt(ctx, rateLimitedAttempt); err != nil {
		t.Fatalf("log rate-limited attempt: %v", err)
	}

	since := time.Now().Add(-15 * time.Minute).UTC()
	failedCount, err := repo.CountRecentFailedAttempts(ctx, "audit@example.test", &ip, since)
	if err != nil {
		t.Fatalf("count recent failed attempts: %v", err)
	}
	if failedCount != 1 {
		t.Fatalf("expected 1 recent failed attempt, got %d", failedCount)
	}

	attempts, total, err := repo.ListLoginAttempts(ctx, LoginAttemptFilter{Email: "AUDIT", Success: boolPtr(false), Since: &since}, 10, 0)
	if err != nil {
		t.Fatalf("list login attempts: %v", err)
	}
	if total != 1 || len(attempts) != 1 || attempts[0].ID != recentAttempt.ID {
		t.Fatalf("unexpected attempts total=%d attempts=%#v", total, attempts)
	}

	if err := repo.ClearRecentFailedAttempts(ctx, "audit@example.test", since); err != nil {
		t.Fatalf("clear recent failed attempts: %v", err)
	}
	failedCount, err = repo.CountRecentFailedAttempts(ctx, "audit@example.test", &ip, since)
	if err != nil {
		t.Fatalf("count after clear: %v", err)
	}
	if failedCount != 0 {
		t.Fatalf("expected recent failed attempts to be cleared, got %d", failedCount)
	}

	targetID := uuid.New()
	kelasID := uuid.New()
	if err := repo.LogAudit(ctx, &AuditLog{ActorID: &user.ID, ActorRole: stringPtr(string(Guru)), Action: "nilai.update", TargetType: stringPtr("nilai"), TargetID: &targetID, TargetKelasID: &kelasID}); err != nil {
		t.Fatalf("log matching audit: %v", err)
	}
	if err := repo.LogAudit(ctx, &AuditLog{ActorID: &user.ID, ActorRole: stringPtr(string(Guru)), Action: "kelas.view", TargetType: stringPtr("kelas")}); err != nil {
		t.Fatalf("log extra audit: %v", err)
	}

	logs, logTotal, err := repo.ListAuditLogs(ctx, AuditLogFilter{ActorID: &user.ID, TargetID: &targetID, TargetKelasID: &kelasID, Actions: []string{"nilai.update", "nilai.delete"}}, 10, 0)
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if logTotal != 1 || len(logs) != 1 || logs[0].Action != "nilai.update" {
		t.Fatalf("unexpected audit logs total=%d logs=%#v", logTotal, logs)
	}
}

func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
