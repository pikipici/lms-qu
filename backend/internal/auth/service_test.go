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
	userByEmail          map[string]*User
	userByID             map[uuid.UUID]*User
	failedCount          int64
	findByEmailCalls     int
	findByIDCalls        int
	lastFindEmail        string
	countCalls           int
	countEmail           string
	countIP              *string
	countSince           time.Time
	incCalls             int
	resetCalls           int
	lockCalls            int
	lockReasons          []string
	issueRefreshCalls    int
	issuedRefreshes      []*RefreshToken
	refreshByJTI         map[uuid.UUID]*RefreshToken
	rotateCalls          int
	revokeCalls          int
	revokeAllUserCalls   int
	revokeAllReasons     []RevokedReason
	chainRevokeCalls     int
	chainRevokedJTIs     []uuid.UUID
	updatePasswordCalls  int
	clearMustChangeCalls int
	clearMustChangeErr   error
	lastPasswordHash     string
	listSessionsResult   []RefreshToken
	loginAttempts        []*LoginAttempt
	audits               []*AuditLog
	errFindByEmail       error
	selfRegistered       map[uuid.UUID]bool
	selfRegisteredErr    error
	selfRegisteredCalls  int
}

func newTestService(t *testing.T) (*Service, *mockRepo, func()) {
	t.Helper()

	repo := &mockRepo{
		userByEmail:    map[string]*User{},
		userByID:       map[uuid.UUID]*User{},
		selfRegistered: map[uuid.UUID]bool{},
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

func TestLogin_SelfRegisteredSiswa_DoesNotForcePasswordChange(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	user.Role = Siswa
	user.MustChangePassword = true
	repo.selfRegistered[user.ID] = true
	repo.addUser(user)

	got, err := svc.Login(context.Background(), user.Email, loginTestPassword, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if got.User.MustChangePassword {
		t.Fatal("Login() MustChangePassword = true, want false for self-registered siswa")
	}
	if repo.selfRegisteredCalls != 1 {
		t.Fatalf("IsSelfRegisteredSiswa calls = %d, want 1", repo.selfRegisteredCalls)
	}
	if repo.clearMustChangeCalls != 1 {
		t.Fatalf("ClearMustChangePassword calls = %d, want 1", repo.clearMustChangeCalls)
	}
	if user.MustChangePassword {
		t.Fatal("persisted user.MustChangePassword = true, want false")
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

func TestRefresh_Success_RotatesTokenAndReturnsNewPair(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	_, oldJTI, raw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		oldJTI: &RefreshToken{
			JTI:       oldJTI,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	got, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if got == nil {
		t.Fatal("Refresh() result is nil")
	}
	if got.User.ID != user.ID {
		t.Fatalf("Refresh() user ID = %s, want %s", got.User.ID, user.ID)
	}
	if got.AccessToken == "" {
		t.Fatal("Refresh() AccessToken is empty")
	}
	if got.RefreshToken == "" {
		t.Fatal("Refresh() RefreshToken is empty")
	}
	if got.RefreshJTI == oldJTI.String() {
		t.Fatal("Refresh() reused old refresh JTI")
	}
	newJTI, err := uuid.Parse(got.RefreshJTI)
	if err != nil {
		t.Fatalf("uuid.Parse(RefreshJTI) error = %v", err)
	}
	if repo.rotateCalls != 1 {
		t.Fatalf("RotateRefresh calls = %d, want 1", repo.rotateCalls)
	}
	old := repo.refreshByJTI[oldJTI]
	if old.RevokedAt == nil {
		t.Fatal("old refresh RevokedAt = nil, want set")
	}
	if old.ReplacedByJTI == nil || *old.ReplacedByJTI != newJTI {
		t.Fatalf("old refresh ReplacedByJTI = %v, want %s", old.ReplacedByJTI, newJTI)
	}
	if _, ok := repo.refreshByJTI[newJTI]; !ok {
		t.Fatalf("new refresh %s was not persisted", newJTI)
	}
	assertOneAuditAction(t, repo, "refresh_success")
}

func TestRefresh_InvalidJWT_ReturnsErrInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.Refresh(context.Background(), "garbage", "1.2.3.4", "ua")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "invalid_token")
}

func TestRefresh_WrongSecret_ReturnsErrInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	otherJWT := svc.cfg.JWT
	otherJWT.SecretKey = "other-test-secret-min-32-chars-123456"
	_, _, raw, _ := issueValidRefresh(t, otherJWT, uuid.New())

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "invalid_token")
}

func TestRefresh_UnknownJTI_ReturnsErrInvalidCredentialsNoChainRevoke(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	userID := uuid.New()
	_, _, raw, _ := issueValidRefresh(t, svc.cfg.JWT, userID)

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	if repo.chainRevokeCalls != 0 {
		t.Fatalf("RevokeRefreshChain calls = %d, want 0", repo.chainRevokeCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "unknown_jti")
	if repo.audits[0].ActorID == nil || *repo.audits[0].ActorID != userID {
		t.Fatalf("audit ActorID = %v, want %s", repo.audits[0].ActorID, userID)
	}
}

func TestRefresh_AlreadyRevokedToken_TriggersReuseDetectionAndChainRevoke(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	_, revokedJTI, revokedRaw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	_, activeJTI, _, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	revokedAt := fixedLoginNow.Add(-time.Minute)
	rotateReason := string(Rotate)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		revokedJTI: &RefreshToken{
			JTI:           revokedJTI,
			UserID:        user.ID,
			IssuedAt:      fixedLoginNow.Add(-time.Hour),
			ExpiresAt:     fixedLoginNow.Add(7 * 24 * time.Hour),
			RevokedAt:     &revokedAt,
			RevokedReason: &rotateReason,
		},
		activeJTI: &RefreshToken{
			JTI:       activeJTI,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	_, err := svc.Refresh(context.Background(), revokedRaw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrRefreshReuse) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrRefreshReuse)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	if repo.chainRevokeCalls != 1 {
		t.Fatalf("RevokeRefreshChain calls = %d, want 1", repo.chainRevokeCalls)
	}
	if len(repo.chainRevokedJTIs) != 1 || repo.chainRevokedJTIs[0] != revokedJTI {
		t.Fatalf("chainRevokedJTIs = %v, want [%s]", repo.chainRevokedJTIs, revokedJTI)
	}
	active := repo.refreshByJTI[activeJTI]
	if active.RevokedAt == nil {
		t.Fatal("active refresh RevokedAt = nil, want set")
	}
	if active.RevokedReason == nil || *active.RevokedReason != string(ReuseDetected) {
		t.Fatalf("active refresh RevokedReason = %v, want %s", active.RevokedReason, ReuseDetected)
	}
	assertOneAuditAction(t, repo, "refresh_reuse_detected")
}

func TestRefresh_ExpiredPersistedToken_ReturnsErrInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)
	_, oldJTI, raw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		oldJTI: &RefreshToken{
			JTI:       oldJTI,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow.Add(-8 * 24 * time.Hour),
			ExpiresAt: fixedLoginNow.Add(-time.Hour),
		},
	}

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	if repo.chainRevokeCalls != 0 {
		t.Fatalf("RevokeRefreshChain calls = %d, want 0", repo.chainRevokeCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "expired")
}

func TestRefresh_UserSuspended_ReturnsErrUserSuspended(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Suspended, 0)
	repo.addUser(user)
	_, oldJTI, raw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		oldJTI: &RefreshToken{
			JTI:       oldJTI,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrUserSuspended) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrUserSuspended)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "user_suspended")
}

func TestRefresh_UserLocked_ReturnsErrUserLocked(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Locked, 0)
	repo.addUser(user)
	_, oldJTI, raw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		oldJTI: &RefreshToken{
			JTI:       oldJTI,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrUserLocked) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrUserLocked)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "user_locked")
}

func TestRefresh_UserMismatch_ReturnsErrInvalidCredentials(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	claimUserID := uuid.New()
	persistedUserID := uuid.New()
	_, oldJTI, raw, _ := issueValidRefresh(t, svc.cfg.JWT, claimUserID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		oldJTI: &RefreshToken{
			JTI:       oldJTI,
			UserID:    persistedUserID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	_, err := svc.Refresh(context.Background(), raw, "1.2.3.4", "ua")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Refresh() error = %v, want %v", err, ErrInvalidCredentials)
	}
	if repo.rotateCalls != 0 {
		t.Fatalf("RotateRefresh calls = %d, want 0", repo.rotateCalls)
	}
	if repo.chainRevokeCalls != 0 {
		t.Fatalf("RevokeRefreshChain calls = %d, want 0", repo.chainRevokeCalls)
	}
	assertOneAuditAction(t, repo, "refresh_failed")
	assertAuditReason(t, repo.audits[0], "user_mismatch")
}

func TestLogout_KnownToken_RevokesAndAudits(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	_, jti, raw, _ := issueValidRefresh(t, svc.cfg.JWT, user.ID)
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		jti: &RefreshToken{
			JTI:       jti,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(7 * 24 * time.Hour),
		},
	}

	if err := svc.Logout(context.Background(), raw, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if repo.revokeCalls != 1 {
		t.Fatalf("RevokeRefresh calls = %d, want 1", repo.revokeCalls)
	}
	if repo.refreshByJTI[jti].RevokedAt == nil {
		t.Fatal("refresh RevokedAt = nil, want set")
	}
	if repo.refreshByJTI[jti].RevokedReason == nil || *repo.refreshByJTI[jti].RevokedReason != string(Logout) {
		t.Fatalf("refresh RevokedReason = %v, want %s", repo.refreshByJTI[jti].RevokedReason, Logout)
	}
	assertOneAuditAction(t, repo, "logout")
	if repo.audits[0].ActorID == nil || *repo.audits[0].ActorID != user.ID {
		t.Fatalf("audit ActorID = %v, want %s", repo.audits[0].ActorID, user.ID)
	}
}

func TestLogout_UnknownJWT_StillReturnsNilNoAudit(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()

	if err := svc.Logout(context.Background(), "garbage", "1.2.3.4", "ua"); err != nil {
		t.Fatalf("Logout() error = %v, want nil", err)
	}
	if repo.revokeCalls != 0 {
		t.Fatalf("RevokeRefresh calls = %d, want 0", repo.revokeCalls)
	}
	if len(repo.audits) != 0 {
		t.Fatalf("audits len = %d, want 0", len(repo.audits))
	}
}

func TestLogoutAll_RevokesAllUserTokens(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	userA := uuid.New()
	userB := uuid.New()
	jtiA1 := uuid.New()
	jtiA2 := uuid.New()
	jtiA3 := uuid.New()
	jtiB := uuid.New()
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		jtiA1: &RefreshToken{
			JTI:       jtiA1,
			UserID:    userA,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
		jtiA2: &RefreshToken{
			JTI:       jtiA2,
			UserID:    userA,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
		jtiA3: &RefreshToken{
			JTI:       jtiA3,
			UserID:    userA,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
		jtiB: &RefreshToken{
			JTI:       jtiB,
			UserID:    userB,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
	}

	if err := svc.LogoutAll(context.Background(), userA, "1.2.3.4", "ua"); err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}
	if repo.revokeAllUserCalls != 1 {
		t.Fatalf("RevokeAllRefreshByUser calls = %d, want 1", repo.revokeAllUserCalls)
	}
	for _, jti := range []uuid.UUID{jtiA1, jtiA2, jtiA3} {
		if repo.refreshByJTI[jti].RevokedAt == nil {
			t.Fatalf("userA token %s RevokedAt = nil, want set", jti)
		}
	}
	if repo.refreshByJTI[jtiB].RevokedAt != nil {
		t.Fatal("userB token RevokedAt set, want nil")
	}
	assertOneAuditAction(t, repo, "logout_all")
}

func TestMe_ReturnsUser(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	got, err := svc.Me(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("Me() user ID = %s, want %s", got.ID, user.ID)
	}
}

func TestMe_NotFound_ReturnsError(t *testing.T) {
	svc, _, cleanup := newTestService(t)
	defer cleanup()

	_, err := svc.Me(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("Me() error = nil, want non-nil")
	}
}

func TestChangePassword_Success_HashesAndRevokesAll(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	oldHash, err := HashPassword("OldPass1!", testServiceConfig().JWT.BcryptCost)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	user.PasswordHash = oldHash
	user.MustChangePassword = true
	repo.addUser(user)

	jtiA := uuid.New()
	jtiB := uuid.New()
	repo.refreshByJTI = map[uuid.UUID]*RefreshToken{
		jtiA: &RefreshToken{
			JTI:       jtiA,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
		jtiB: &RefreshToken{
			JTI:       jtiB,
			UserID:    user.ID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
	}

	err = svc.ChangePassword(context.Background(), user.ID, "OldPass1!", "NewPass2@", "1.2.3.4", "ua")
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if repo.updatePasswordCalls != 1 {
		t.Fatalf("UpdateUserPassword calls = %d, want 1", repo.updatePasswordCalls)
	}
	if user.MustChangePassword {
		t.Fatal("user.MustChangePassword = true, want false")
	}
	if err := VerifyPassword(user.PasswordHash, "NewPass2@"); err != nil {
		t.Fatalf("VerifyPassword(new hash) error = %v", err)
	}
	if user.PasswordHash == oldHash {
		t.Fatal("user.PasswordHash was not changed")
	}
	if repo.revokeAllUserCalls != 1 {
		t.Fatalf("RevokeAllRefreshByUser calls = %d, want 1", repo.revokeAllUserCalls)
	}
	if len(repo.revokeAllReasons) != 1 || repo.revokeAllReasons[0] != PasswordChanged {
		t.Fatalf("revokeAllReasons = %v, want [%s]", repo.revokeAllReasons, PasswordChanged)
	}
	for _, jti := range []uuid.UUID{jtiA, jtiB} {
		token := repo.refreshByJTI[jti]
		if token.RevokedAt == nil {
			t.Fatalf("refresh %s RevokedAt = nil, want set", jti)
		}
		if token.RevokedReason == nil || *token.RevokedReason != string(PasswordChanged) {
			t.Fatalf("refresh %s RevokedReason = %v, want %s", jti, token.RevokedReason, PasswordChanged)
		}
	}
	assertOneAuditAction(t, repo, "password_changed")
	if repo.audits[0].TargetID == nil || *repo.audits[0].TargetID != user.ID {
		t.Fatalf("audit TargetID = %v, want %s", repo.audits[0].TargetID, user.ID)
	}
}

func TestChangePassword_WrongCurrent_ReturnsErrCurrentPasswordIncorrect(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	err := svc.ChangePassword(context.Background(), user.ID, "wrong-current", "NewPass2@", "1.2.3.4", "ua")
	if !errors.Is(err, ErrCurrentPasswordIncorrect) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrCurrentPasswordIncorrect)
	}
	if repo.updatePasswordCalls != 0 {
		t.Fatalf("UpdateUserPassword calls = %d, want 0", repo.updatePasswordCalls)
	}
	if repo.revokeAllUserCalls != 0 {
		t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeAllUserCalls)
	}
	assertOneAuditAction(t, repo, "password_change_failed")
	assertAuditReason(t, repo.audits[0], "invalid_current_password")
}

func TestChangePassword_WeakPassword_ReturnsErrWeakPassword(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()

	err := svc.ChangePassword(context.Background(), uuid.New(), "OldPass1!", "short", "1.2.3.4", "ua")
	if !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrWeakPassword)
	}
	if repo.findByIDCalls != 0 {
		t.Fatalf("FindUserByID calls = %d, want 0", repo.findByIDCalls)
	}
	if repo.updatePasswordCalls != 0 {
		t.Fatalf("UpdateUserPassword calls = %d, want 0", repo.updatePasswordCalls)
	}
}

func TestChangePassword_SamePassword_ReturnsErrSamePassword(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	repo.addUser(user)

	err := svc.ChangePassword(context.Background(), user.ID, loginTestPassword, loginTestPassword, "1.2.3.4", "ua")
	if !errors.Is(err, ErrSamePassword) {
		t.Fatalf("ChangePassword() error = %v, want %v", err, ErrSamePassword)
	}
	if repo.updatePasswordCalls != 0 {
		t.Fatalf("UpdateUserPassword calls = %d, want 0", repo.updatePasswordCalls)
	}
	if repo.revokeAllUserCalls != 0 {
		t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeAllUserCalls)
	}
}

func TestListSessions_ReturnsRepoResult(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	userID := uuid.New()
	repo.listSessionsResult = []RefreshToken{
		{
			JTI:       uuid.New(),
			UserID:    userID,
			IssuedAt:  fixedLoginNow,
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
		{
			JTI:       uuid.New(),
			UserID:    userID,
			IssuedAt:  fixedLoginNow.Add(-time.Minute),
			ExpiresAt: fixedLoginNow.Add(time.Hour),
		},
	}

	got, err := svc.ListSessions(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListSessions() len = %d, want 2", len(got))
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
	m.findByIDCalls++
	user, ok := m.userByID[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (m *mockRepo) IsSelfRegisteredSiswa(ctx context.Context, userID uuid.UUID) (bool, error) {
	m.selfRegisteredCalls++
	if m.selfRegisteredErr != nil {
		return false, m.selfRegisteredErr
	}
	return m.selfRegistered[userID], nil
}

func (m *mockRepo) UpdateUserPassword(ctx context.Context, userID uuid.UUID, newHash string) error {
	m.updatePasswordCalls++
	m.lastPasswordHash = newHash
	if user, ok := m.userByID[userID]; ok {
		user.PasswordHash = newHash
		user.MustChangePassword = false
	}
	return nil
}

func (m *mockRepo) ClearMustChangePassword(ctx context.Context, userID uuid.UUID) error {
	m.clearMustChangeCalls++
	if m.clearMustChangeErr != nil {
		return m.clearMustChangeErr
	}
	if user, ok := m.userByID[userID]; ok {
		user.MustChangePassword = false
	}
	return nil
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

func (m *mockRepo) FindRefreshByJTI(ctx context.Context, jti uuid.UUID) (*RefreshToken, error) {
	if m.refreshByJTI == nil {
		return nil, gorm.ErrRecordNotFound
	}
	t, ok := m.refreshByJTI[jti]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return t, nil
}

func (m *mockRepo) RotateRefresh(ctx context.Context, oldJTI uuid.UUID, newToken *RefreshToken) error {
	m.rotateCalls++
	if old, ok := m.refreshByJTI[oldJTI]; ok {
		rev := time.Now()
		old.RevokedAt = &rev
		rotateReason := string(Rotate)
		old.RevokedReason = &rotateReason
		old.ReplacedByJTI = &newToken.JTI
	}
	if m.refreshByJTI == nil {
		m.refreshByJTI = map[uuid.UUID]*RefreshToken{}
	}
	m.refreshByJTI[newToken.JTI] = newToken
	return nil
}

func (m *mockRepo) RevokeRefresh(ctx context.Context, jti uuid.UUID, reason RevokedReason) error {
	m.revokeCalls++
	if m.refreshByJTI == nil {
		return nil
	}
	if t, ok := m.refreshByJTI[jti]; ok && t.RevokedAt == nil {
		rev := time.Now()
		t.RevokedAt = &rev
		r := string(reason)
		t.RevokedReason = &r
	}
	return nil
}

func (m *mockRepo) RevokeAllRefreshByUser(ctx context.Context, userID uuid.UUID, reason RevokedReason) (int64, error) {
	m.revokeAllUserCalls++
	m.revokeAllReasons = append(m.revokeAllReasons, reason)
	var n int64
	for _, t := range m.refreshByJTI {
		if t.UserID == userID && t.RevokedAt == nil {
			rev := time.Now()
			t.RevokedAt = &rev
			r := string(reason)
			t.RevokedReason = &r
			n++
		}
	}
	return n, nil
}

func (m *mockRepo) RevokeRefreshChain(ctx context.Context, jti uuid.UUID) error {
	m.chainRevokeCalls++
	m.chainRevokedJTIs = append(m.chainRevokedJTIs, jti)
	if t, ok := m.refreshByJTI[jti]; ok {
		for _, candidate := range m.refreshByJTI {
			if candidate.UserID == t.UserID && candidate.RevokedAt == nil {
				rev := time.Now()
				candidate.RevokedAt = &rev
				reuseReason := string(ReuseDetected)
				candidate.RevokedReason = &reuseReason
			}
		}
	}
	return nil
}

func (m *mockRepo) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error) {
	return m.listSessionsResult, nil
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

func (m *mockRepo) ClearRecentFailedAttempts(ctx context.Context, email string, since time.Time) error {
	return nil
}

func (m *mockRepo) LogAudit(ctx context.Context, entry *AuditLog) error {
	m.audits = append(m.audits, entry)
	return nil
}

func TestNewService_WiresRepoAndConfig(t *testing.T) {
	repo := &Repo{}
	cfg := testServiceConfig()

	svc := NewService(repo, cfg)
	if svc == nil {
		t.Fatal("NewService() returned nil")
	}
	if svc.repo != repo {
		t.Fatal("NewService() did not wire repo")
	}
	if svc.cfg != cfg {
		t.Fatal("NewService() did not wire config")
	}
	if svc.now == nil {
		t.Fatal("NewService() now is nil")
	}
}

func TestVerifyAccessToken_Success(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Active, 0)
	user.MustChangePassword = true
	repo.addUser(user)
	raw, _, err := IssueAccess(svc.cfg.JWT, user.ID, user.Role)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}

	gotID, gotRole, gotEmail, gotMustChange, err := svc.VerifyAccessToken(raw)
	if err != nil {
		t.Fatalf("VerifyAccessToken() error = %v", err)
	}
	if gotID != user.ID || gotRole != string(user.Role) || gotEmail != user.Email || !gotMustChange {
		t.Fatalf("VerifyAccessToken() = (%s,%q,%q,%v), want (%s,%q,%q,true)", gotID, gotRole, gotEmail, gotMustChange, user.ID, user.Role, user.Email)
	}
}

func TestVerifyAccessToken_InvalidToken(t *testing.T) {
	svc, _, cleanup := newTestService(t)
	defer cleanup()

	_, _, _, _, err := svc.VerifyAccessToken("not-a-token")
	if err == nil {
		t.Fatal("VerifyAccessToken() error = nil, want error")
	}
}

func TestVerifyAccessToken_InactiveUser(t *testing.T) {
	svc, repo, cleanup := newTestService(t)
	defer cleanup()
	user := newLoginTestUser(t, Suspended, 0)
	repo.addUser(user)
	raw, _, err := IssueAccess(svc.cfg.JWT, user.ID, user.Role)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}

	_, _, _, _, err = svc.VerifyAccessToken(raw)
	if err == nil {
		t.Fatal("VerifyAccessToken() error = nil, want inactive user error")
	}
}

func TestAuditAndPointerHelpers_EmptyValues(t *testing.T) {
	if got := auditMeta(nil); got != nil {
		t.Fatalf("auditMeta(nil) = %s, want nil", string(got))
	}
	if got := auditMeta(map[string]string{}); got != nil {
		t.Fatalf("auditMeta(empty) = %s, want nil", string(got))
	}
	if got := ipPtr(""); got != nil {
		t.Fatalf("ipPtr(empty) = %v, want nil", got)
	}
	if got := uaPtr(""); got != nil {
		t.Fatalf("uaPtr(empty) = %v, want nil", got)
	}
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

func issueValidRefresh(t *testing.T, cfg config.JWTConfig, userID uuid.UUID) (jti string, parsedJTI uuid.UUID, raw string, expiresAt time.Time) {
	t.Helper()

	jti, raw, expiresAt, err := IssueRefresh(cfg, userID)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}
	parsedJTI, err = uuid.Parse(jti)
	if err != nil {
		t.Fatalf("uuid.Parse: %v", err)
	}
	return jti, parsedJTI, raw, expiresAt
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
