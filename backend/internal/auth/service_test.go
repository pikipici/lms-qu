package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/config"
	"gorm.io/gorm"
)

const loginTestPassword = "S3cret!Pass"

var fixedLoginNow = time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)

type mockRepo struct {
	userByEmail       map[string]*User
	userByID          map[uuid.UUID]*User
	failedCount       int64
	findByEmailCalls  int
	lastFindEmail     string
	countCalls        int
	countEmail        string
	countIP           *string
	countSince        time.Time
	incCalls          int
	resetCalls        int
	lockCalls         int
	lockReasons       []string
	issueRefreshCalls int
	issuedRefreshes   []*RefreshToken
	loginAttempts     []*LoginAttempt
	audits            []*AuditLog
	errFindByEmail    error
}

func newTestService(t *testing.T) (*Service, *mockRepo, func()) {
	t.Helper()

	repo := &mockRepo{
		userByEmail: map[string]*User{},
		userByID:    map[uuid.UUID]*User{},
	}
	svc := &Service{
		repo: repo,
		cfg:  testServiceConfig(),
		now:  func() time.Time { return fixedLoginNow },
	}
	return svc, repo, func() {}
}

func TestLogin_Success_IssuesTokens(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 4)
	repo.addUser(user)

	got, err := svc.Login(context.Background(), user.Email, loginTestPassword, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if got.User.ID != user.ID {
		t.Fatalf("Login() user ID = %s, want %s", got.User.ID, user.ID)
	}
	if got.AccessToken == "" {
		t.Fatal("Login() AccessToken is empty")
	}
	if got.RefreshToken == "" {
		t.Fatal("Login() RefreshToken is empty")
	}
	if _, err := uuid.Parse(got.RefreshJTI); err != nil {
		t.Fatalf("uuid.Parse(RefreshJTI) error = %v", err)
	}
	if got.AccessExpiresAt.IsZero() {
		t.Fatal("Login() AccessExpiresAt is zero")
	}
	if got.RefreshExpiresAt.IsZero() {
		t.Fatal("Login() RefreshExpiresAt is zero")
	}
	if repo.resetCalls != 1 {
		t.Fatalf("ResetFailedLogin calls = %d, want 1", repo.resetCalls)
	}
	if repo.issueRefreshCalls != 1 {
		t.Fatalf("IssueRefresh calls = %d, want 1", repo.issueRefreshCalls)
	}
	if len(repo.loginAttempts) != 1 {
		t.Fatalf("loginAttempts len = %d, want 1", len(repo.loginAttempts))
	}
	if !repo.loginAttempts[0].Success {
		t.Fatal("login attempt Success = false, want true")
	}
	if repo.loginAttempts[0].Reason != nil {
		t.Fatalf("login attempt Reason = %q, want nil", *repo.loginAttempts[0].Reason)
	}
	if len(repo.audits) != 1 {
		t.Fatalf("audits len = %d, want 1", len(repo.audits))
	}
	if repo.audits[0].Action != "login_success" {
		t.Fatalf("audit action = %q, want login_success", repo.audits[0].Action)
	}
	if repo.audits[0].ActorID == nil || *repo.audits[0].ActorID != user.ID {
		t.Fatalf("audit ActorID = %v, want %s", repo.audits[0].ActorID, user.ID)
	}
	if repo.audits[0].ActorRole == nil || *repo.audits[0].ActorRole != string(user.Role) {
		t.Fatalf("audit ActorRole = %v, want %s", repo.audits[0].ActorRole, user.Role)
	}
}

func TestLogin_WrongPassword_ReturnsInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	_, err := svc.Login(context.Background(), user.Email, "wrong-password", "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.incCalls != 1 {
		t.Fatalf("IncFailedLogin calls = %d, want 1", repo.incCalls)
	}
	assertOneFailedAttemptReason(t, repo, "invalid_credentials")
	assertOneAuditAction(t, repo, "login_failed")
	assertAuditReason(t, repo.audits[0], "invalid_credentials")
}

func TestLogin_UserNotFound_ReturnsInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.Login(context.Background(), "missing@example.com", loginTestPassword, "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
	assertOneFailedAttemptReason(t, repo, "user_not_found")
	assertOneAuditAction(t, repo, "login_failed")
	assertAuditReason(t, repo.audits[0], "user_not_found")
}

func TestLogin_Suspended_ReturnsErrUserSuspended(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Suspended, 0)
	repo.addUser(user)

	_, err := svc.Login(context.Background(), user.Email, loginTestPassword, "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrUserSuspended) {
		t.Fatalf("Login() error = %v, want %v", err, ErrUserSuspended)
	}
	if repo.incCalls != 0 {
		t.Fatalf("IncFailedLogin calls = %d, want 0", repo.incCalls)
	}
	assertOneFailedAttemptReason(t, repo, "user_suspended")
	assertOneAuditAction(t, repo, "login_failed")
}

func TestLogin_Locked_ReturnsErrUserLocked(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Locked, 0)
	repo.addUser(user)

	_, err := svc.Login(context.Background(), user.Email, loginTestPassword, "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrUserLocked) {
		t.Fatalf("Login() error = %v, want %v", err, ErrUserLocked)
	}
	if repo.incCalls != 0 {
		t.Fatalf("IncFailedLogin calls = %d, want 0", repo.incCalls)
	}
	assertOneFailedAttemptReason(t, repo, "user_locked")
	assertOneAuditAction(t, repo, "login_failed")
}

func TestLogin_RateLimited_BeforeLookup(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	repo.failedCount = 5

	_, err := svc.Login(context.Background(), "user@example.com", loginTestPassword, "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("Login() error = %v, want %v", err, ErrRateLimited)
	}
	if repo.findByEmailCalls != 0 {
		t.Fatalf("FindUserByEmail calls = %d, want 0", repo.findByEmailCalls)
	}
	assertOneFailedAttemptReason(t, repo, "rate_limited")
	assertOneAuditAction(t, repo, "login_failed")
	assertAuditReason(t, repo.audits[0], "rate_limited")
}

func TestLogin_LocksUserAfter10FailedAttempts(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 9)
	repo.addUser(user)

	_, err := svc.Login(context.Background(), user.Email, "wrong-password", "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.lockCalls != 1 {
		t.Fatalf("LockUser calls = %d, want 1", repo.lockCalls)
	}
	if user.Status != Locked {
		t.Fatalf("user.Status = %s, want %s", user.Status, Locked)
	}
	assertOneFailedAttemptReason(t, repo, "invalid_credentials")
	assertOneAuditAction(t, repo, "login_failed")
	assertAuditReason(t, repo.audits[0], "user_locked_after_attempt")
}

func TestLogin_NormalizesEmail(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	user.Email = "foo@example.com"
	repo.addUser(user)

	_, err := svc.Login(context.Background(), "  Foo@Example.COM ", loginTestPassword, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if repo.lastFindEmail != "foo@example.com" {
		t.Fatalf("FindUserByEmail email = %q, want foo@example.com", repo.lastFindEmail)
	}
}

func TestLogin_EmptyEmail_NoLogging(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.Login(context.Background(), " ", loginTestPassword, "127.0.0.1", "test-agent")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if len(repo.loginAttempts) != 0 {
		t.Fatalf("loginAttempts len = %d, want 0", len(repo.loginAttempts))
	}
	if len(repo.audits) != 0 {
		t.Fatalf("audits len = %d, want 0", len(repo.audits))
	}
	if repo.countCalls != 0 {
		t.Fatalf("CountRecentFailedAttempts calls = %d, want 0", repo.countCalls)
	}
	if repo.findByEmailCalls != 0 {
		t.Fatalf("FindUserByEmail calls = %d, want 0", repo.findByEmailCalls)
	}
}

func (m *mockRepo) addUser(user *User) {
	m.userByEmail[user.Email] = user
	m.userByID[user.ID] = user
}

func (m *mockRepo) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	m.findByEmailCalls++
	m.lastFindEmail = email
	if m.errFindByEmail != nil {
		return nil, m.errFindByEmail
	}
	user, ok := m.userByEmail[email]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (m *mockRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user, ok := m.userByID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (m *mockRepo) IncFailedLogin(ctx context.Context, userID uuid.UUID) error {
	m.incCalls++
	if user, ok := m.userByID[userID]; ok {
		user.FailedLoginCount++
	}
	return nil
}

func (m *mockRepo) ResetFailedLogin(ctx context.Context, userID uuid.UUID) error {
	m.resetCalls++
	if user, ok := m.userByID[userID]; ok {
		user.FailedLoginCount = 0
		user.LastFailedLoginAt = nil
		lastLogin := fixedLoginNow
		user.LastLoginAt = &lastLogin
	}
	return nil
}

func (m *mockRepo) LockUser(ctx context.Context, userID uuid.UUID, reason string) error {
	m.lockCalls++
	m.lockReasons = append(m.lockReasons, reason)
	if user, ok := m.userByID[userID]; ok {
		user.Status = Locked
	}
	return nil
}

func (m *mockRepo) IssueRefresh(ctx context.Context, t *RefreshToken) error {
	m.issueRefreshCalls++
	m.issuedRefreshes = append(m.issuedRefreshes, t)
	return nil
}

func (m *mockRepo) LogLoginAttempt(ctx context.Context, attempt *LoginAttempt) error {
	m.loginAttempts = append(m.loginAttempts, attempt)
	return nil
}

func (m *mockRepo) CountRecentFailedAttempts(ctx context.Context, email string, ip *string, since time.Time) (int64, error) {
	m.countCalls++
	m.countEmail = email
	m.countIP = ip
	m.countSince = since
	return m.failedCount, nil
}

func (m *mockRepo) LogAudit(ctx context.Context, entry *AuditLog) error {
	m.audits = append(m.audits, entry)
	return nil
}

func newLoginTestUser(t *testing.T, status UserStatus, failedLoginCount int) *User {
	t.Helper()

	hashed, err := HashPassword(loginTestPassword, testServiceConfig().JWT.BcryptCost)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	return &User{
		ID:               uuid.New(),
		Name:             "Test User",
		Email:            "user@example.com",
		PasswordHash:     hashed,
		Role:             Guru,
		Status:           status,
		FailedLoginCount: failedLoginCount,
	}
}

func testServiceConfig() *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			SecretKey:      "test-secret-min-32-chars-1234567890",
			AccessTTLMin:   15,
			RefreshTTLDays: 7,
			BcryptCost:     4,
		},
		RateLimit: config.RateLimitConfig{LoginPer15Min: 5},
	}
}

func assertOneFailedAttemptReason(t *testing.T, repo *mockRepo, reason string) {
	t.Helper()

	if len(repo.loginAttempts) != 1 {
		t.Fatalf("loginAttempts len = %d, want 1", len(repo.loginAttempts))
	}
	attempt := repo.loginAttempts[0]
	if attempt.Success {
		t.Fatal("login attempt Success = true, want false")
	}
	if attempt.Reason == nil {
		t.Fatalf("login attempt Reason = nil, want %q", reason)
	}
	if *attempt.Reason != reason {
		t.Fatalf("login attempt Reason = %q, want %q", *attempt.Reason, reason)
	}
}

func assertOneAuditAction(t *testing.T, repo *mockRepo, action string) {
	t.Helper()

	if len(repo.audits) != 1 {
		t.Fatalf("audits len = %d, want 1", len(repo.audits))
	}
	if repo.audits[0].Action != action {
		t.Fatalf("audit action = %q, want %q", repo.audits[0].Action, action)
	}
}

func assertAuditReason(t *testing.T, audit *AuditLog, reason string) {
	t.Helper()

	var meta map[string]string
	if err := json.Unmarshal(audit.Meta, &meta); err != nil {
		t.Fatalf("json.Unmarshal(audit.Meta) error = %v", err)
	}
	if meta["reason"] != reason {
		t.Fatalf("audit reason = %q, want %q", meta["reason"], reason)
	}
}
