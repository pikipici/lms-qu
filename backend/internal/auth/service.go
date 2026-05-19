package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/config"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const maxCumulativeFailedLogins = 10

// authRepo is the subset of *Repo behavior the Service depends on.
// Defined here so tests can substitute a mock without a real DB.
type authRepo interface {
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, newHash string) error
	IncFailedLogin(ctx context.Context, userID uuid.UUID) error
	ResetFailedLogin(ctx context.Context, userID uuid.UUID) error
	LockUser(ctx context.Context, userID uuid.UUID, reason string) error
	IssueRefresh(ctx context.Context, t *RefreshToken) error
	FindRefreshByJTI(ctx context.Context, jti uuid.UUID) (*RefreshToken, error)
	RotateRefresh(ctx context.Context, oldJTI uuid.UUID, newToken *RefreshToken) error
	RevokeRefresh(ctx context.Context, jti uuid.UUID, reason RevokedReason) error
	RevokeAllRefreshByUser(ctx context.Context, userID uuid.UUID, reason RevokedReason) (int64, error)
	RevokeRefreshChain(ctx context.Context, jti uuid.UUID) error
	ListUserSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error)
	LogLoginAttempt(ctx context.Context, attempt *LoginAttempt) error
	CountRecentFailedAttempts(ctx context.Context, email string, ip *string, since time.Time) (int64, error)
	LogAudit(ctx context.Context, entry *AuditLog) error
}

// Service coordinates auth domain behavior.
type Service struct {
	repo authRepo
	cfg  *config.Config
	now  func() time.Time
}

// Sentinel errors returned by Service methods.
var (
	ErrInvalidCredentials       = errors.New("auth: invalid credentials")
	ErrUserSuspended            = errors.New("auth: user suspended")
	ErrUserLocked               = errors.New("auth: user locked")
	ErrRateLimited              = errors.New("auth: too many failed attempts; try again later")
	ErrRefreshReuse             = errors.New("auth: refresh token reuse detected")
	ErrCurrentPasswordIncorrect = errors.New("auth: current password incorrect")
	ErrWeakPassword             = errors.New("auth: new password must be at least 8 characters")
	ErrSamePassword             = errors.New("auth: new password must differ from current")
)

// LoginResult contains the authenticated user and issued token pair.
type LoginResult struct {
	User             *User
	AccessToken      string
	AccessExpiresAt  time.Time
	RefreshToken     string
	RefreshJTI       string
	RefreshExpiresAt time.Time
}

// NewService creates an auth service backed by repo and cfg.
func NewService(repo *Repo, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg, now: time.Now}
}

// Login authenticates a user, enforcing the login rate limit before lookup,
// rejecting suspended or locked users before bcrypt verification, incrementing
// cumulative failed-login state on bad passwords, locking accounts at 10
// cumulative failures, issuing access and refresh tokens on success, and
// best-effort logging every non-empty-email attempt to LoginAttempt and
// AuditLog. It returns ErrInvalidCredentials for unknown users and bad
// passwords, ErrUserSuspended, ErrUserLocked, or ErrRateLimited for policy
// denials.
func (s *Service) Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, ErrInvalidCredentials
	}

	now := s.now()
	ipAddr := ipPtr(ip)
	ua := uaPtr(userAgent)

	failedAttempts, err := s.repo.CountRecentFailedAttempts(ctx, email, ipAddr, now.Add(-15*time.Minute))
	if err != nil {
		return nil, fmt.Errorf("auth: count recent failed attempts: %w", err)
	}
	if failedAttempts >= int64(s.cfg.RateLimit.LoginPer15Min) {
		s.logLoginAttempt(ctx, email, ipAddr, ua, false, strPtr("rate_limited"), now)
		s.logAudit(ctx, "login_failed", nil, nil, auditMeta(map[string]string{
			"reason": "rate_limited",
			"email":  email,
		}), ipAddr, ua, now)
		return nil, ErrRateLimited
	}

	user, err := s.repo.FindUserByEmail(ctx, email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.logLoginAttempt(ctx, email, ipAddr, ua, false, strPtr("user_not_found"), now)
		s.logAudit(ctx, "login_failed", nil, nil, auditMeta(map[string]string{
			"reason": "user_not_found",
			"email":  email,
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}

	switch user.Status {
	case Suspended:
		s.logLoginAttempt(ctx, email, ipAddr, ua, false, strPtr("user_suspended"), now)
		s.logAudit(ctx, "login_failed", &user.ID, roleStrPtr(user.Role), auditMeta(map[string]string{
			"reason": "user_suspended",
		}), ipAddr, ua, now)
		return nil, ErrUserSuspended
	case Locked:
		s.logLoginAttempt(ctx, email, ipAddr, ua, false, strPtr("user_locked"), now)
		s.logAudit(ctx, "login_failed", &user.ID, roleStrPtr(user.Role), auditMeta(map[string]string{
			"reason": "user_locked",
		}), ipAddr, ua, now)
		return nil, ErrUserLocked
	}

	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		auditReason := "invalid_credentials"
		previousFailedCount := user.FailedLoginCount
		if incErr := s.repo.IncFailedLogin(ctx, user.ID); incErr == nil {
			failedCount := previousFailedCount + 1
			if reloaded, findErr := s.repo.FindUserByID(ctx, user.ID); findErr == nil {
				failedCount = reloaded.FailedLoginCount
			}
			if failedCount >= maxCumulativeFailedLogins {
				if lockErr := s.repo.LockUser(ctx, user.ID, "exceeded_failed_attempts"); lockErr == nil {
					auditReason = "user_locked_after_attempt"
				}
			}
		}

		s.logLoginAttempt(ctx, email, ipAddr, ua, false, strPtr("invalid_credentials"), now)
		s.logAudit(ctx, "login_failed", &user.ID, roleStrPtr(user.Role), auditMeta(map[string]string{
			"reason": auditReason,
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}

	_ = s.repo.ResetFailedLogin(ctx, user.ID)

	accessToken, accessExpiresAt, err := IssueAccess(s.cfg.JWT, user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("auth: issue access token: %w", err)
	}
	refreshJTI, refreshToken, refreshExpiresAt, err := IssueRefresh(s.cfg.JWT, user.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: issue refresh token: %w", err)
	}

	parsedJTI, err := uuid.Parse(refreshJTI)
	if err != nil {
		return nil, fmt.Errorf("auth: parse refresh jti: %w", err)
	}
	rt := &RefreshToken{
		JTI:       parsedJTI,
		UserID:    user.ID,
		ExpiresAt: refreshExpiresAt,
		IssuedAt:  now,
		IP:        ipAddr,
		UserAgent: ua,
	}
	if err := s.repo.IssueRefresh(ctx, rt); err != nil {
		return nil, fmt.Errorf("auth: persist refresh token: %w", err)
	}

	s.logLoginAttempt(ctx, email, ipAddr, ua, true, nil, now)
	s.logAudit(ctx, "login_success", &user.ID, roleStrPtr(user.Role), nil, ipAddr, ua, now)

	return &LoginResult{
		User:             user,
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshToken:     refreshToken,
		RefreshJTI:       refreshJTI,
		RefreshExpiresAt: refreshExpiresAt,
	}, nil
}

// Refresh rotates a refresh token. It verifies the JWT, looks up the
// persisted RefreshToken by JTI, detects reuse (token already revoked) by
// revoking the entire refresh chain for that user with reason=reuse_detected,
// rejects expired or unknown tokens, and otherwise issues a fresh access and
// refresh pair atomically via repo.RotateRefresh.
//
// It returns ErrInvalidCredentials for verify failures, expired or unknown
// tokens; ErrRefreshReuse when an already-revoked token is presented;
// ErrUserSuspended or ErrUserLocked if the user's status no longer permits
// login.
func (s *Service) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*LoginResult, error) {
	now := s.now()
	ipAddr := ipPtr(ip)
	ua := uaPtr(userAgent)

	jti, userID, err := VerifyRefresh(s.cfg.JWT, refreshToken)
	if err != nil {
		s.logAudit(ctx, "refresh_failed", nil, nil, auditMeta(map[string]string{
			"reason": "invalid_token",
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}

	parsedJTI, err := uuid.Parse(jti)
	if err != nil {
		s.logAudit(ctx, "refresh_failed", nil, nil, auditMeta(map[string]string{
			"reason": "invalid_jti",
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}

	persisted, err := s.repo.FindRefreshByJTI(ctx, parsedJTI)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		s.logAudit(ctx, "refresh_failed", &userID, nil, auditMeta(map[string]string{
			"reason": "unknown_jti",
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: find refresh: %w", err)
	}

	if persisted.UserID != userID {
		s.logAudit(ctx, "refresh_failed", &persisted.UserID, nil, auditMeta(map[string]string{
			"reason": "user_mismatch",
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}

	if persisted.RevokedAt != nil {
		_ = s.repo.RevokeRefreshChain(ctx, parsedJTI)
		s.logAudit(ctx, "refresh_reuse_detected", &persisted.UserID, nil, auditMeta(map[string]string{
			"jti": parsedJTI.String(),
		}), ipAddr, ua, now)
		return nil, ErrRefreshReuse
	}

	if !persisted.ExpiresAt.After(now) {
		s.logAudit(ctx, "refresh_failed", &persisted.UserID, nil, auditMeta(map[string]string{
			"reason": "expired",
		}), ipAddr, ua, now)
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.FindUserByID(ctx, persisted.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth: find user: %w", err)
	}

	switch user.Status {
	case Suspended:
		s.logAudit(ctx, "refresh_failed", &user.ID, roleStrPtr(user.Role), auditMeta(map[string]string{
			"reason": "user_suspended",
		}), ipAddr, ua, now)
		return nil, ErrUserSuspended
	case Locked:
		s.logAudit(ctx, "refresh_failed", &user.ID, roleStrPtr(user.Role), auditMeta(map[string]string{
			"reason": "user_locked",
		}), ipAddr, ua, now)
		return nil, ErrUserLocked
	}

	accessToken, accessExpiresAt, err := IssueAccess(s.cfg.JWT, user.ID, user.Role)
	if err != nil {
		return nil, fmt.Errorf("auth: issue access token: %w", err)
	}
	newRefreshJTI, newRefreshToken, newRefreshExpiresAt, err := IssueRefresh(s.cfg.JWT, user.ID)
	if err != nil {
		return nil, fmt.Errorf("auth: issue refresh token: %w", err)
	}

	newParsedJTI, err := uuid.Parse(newRefreshJTI)
	if err != nil {
		return nil, fmt.Errorf("auth: parse refresh jti: %w", err)
	}
	newPersistedRefresh := &RefreshToken{
		JTI:       newParsedJTI,
		UserID:    user.ID,
		IssuedAt:  now,
		ExpiresAt: newRefreshExpiresAt,
		IP:        ipAddr,
		UserAgent: ua,
	}
	if err := s.repo.RotateRefresh(ctx, parsedJTI, newPersistedRefresh); err != nil {
		return nil, fmt.Errorf("auth: rotate refresh: %w", err)
	}

	s.logAudit(ctx, "refresh_success", &user.ID, roleStrPtr(user.Role), nil, ipAddr, ua, now)

	return &LoginResult{
		User:             user,
		AccessToken:      accessToken,
		AccessExpiresAt:  accessExpiresAt,
		RefreshToken:     newRefreshToken,
		RefreshJTI:       newRefreshJTI,
		RefreshExpiresAt: newRefreshExpiresAt,
	}, nil
}

// VerifyAccessToken parses a bearer access token, verifies its signature,
// loads the user, status-checks (Active only), and returns the identity
// tuple plus must-change-password flag needed by the middleware layer.
func (s *Service) VerifyAccessToken(rawToken string) (uuid.UUID, string, string, bool, error) {
	claims, err := VerifyAccess(s.cfg.JWT, rawToken)
	if err != nil {
		return uuid.Nil, "", "", false, fmt.Errorf("auth: verify access: %w", err)
	}

	// Middleware currently only passes the raw token, so this lookup cannot use
	// the request context. Acceptable for the MVP; we lose request-scoped values.
	user, err := s.repo.FindUserByID(context.Background(), claims.UserID)
	if err != nil {
		return uuid.Nil, "", "", false, fmt.Errorf("auth: load user: %w", err)
	}
	if user.Status != Active {
		return uuid.Nil, "", "", false, errors.New("auth: user not active")
	}

	return user.ID, string(user.Role), user.Email, user.MustChangePassword, nil
}

// Logout revokes a single refresh token by JWT. It is idempotent: invalid,
// unknown, expired, or already-revoked tokens still return nil so clients do
// not learn token validity.
func (s *Service) Logout(ctx context.Context, refreshToken, ip, userAgent string) error {
	jtiStr, userID, err := VerifyRefresh(s.cfg.JWT, refreshToken)
	if err != nil {
		return nil
	}

	jti, err := uuid.Parse(jtiStr)
	if err != nil {
		return nil
	}

	_ = s.repo.RevokeRefresh(ctx, jti, Logout)
	s.logAudit(ctx, "logout", &userID, nil, nil, ipPtr(ip), uaPtr(userAgent), s.now())
	return nil
}

// LogoutAll revokes every active refresh token for a user.
func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID, ip, userAgent string) error {
	if _, err := s.repo.RevokeAllRefreshByUser(ctx, userID, Logout); err != nil {
		return fmt.Errorf("auth: revoke all refresh: %w", err)
	}

	s.logAudit(ctx, "logout_all", &userID, nil, nil, ipPtr(ip), uaPtr(userAgent), s.now())
	return nil
}

// Me returns the authenticated user's profile.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: load user: %w", err)
	}
	return user, nil
}

// ChangePassword verifies the current password, hashes the new one, persists
// it (clearing must_change_password), revokes all refresh tokens for the user,
// and audits the action. It returns ErrCurrentPasswordIncorrect when the
// current password does not match.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword, ip, userAgent string) error {
	if len(newPassword) < 8 {
		return ErrWeakPassword
	}

	user, err := s.repo.FindUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("auth: load user: %w", err)
	}

	now := s.now()
	ipAddr := ipPtr(ip)
	ua := uaPtr(userAgent)
	if err := VerifyPassword(user.PasswordHash, currentPassword); err != nil {
		s.logAuditTarget(ctx, "password_change_failed", &user.ID, roleStrPtr(user.Role), &user.ID, auditMeta(map[string]string{
			"reason": "invalid_current_password",
		}), ipAddr, ua, now)
		return ErrCurrentPasswordIncorrect
	}
	if newPassword == currentPassword {
		return ErrSamePassword
	}

	newHash, err := HashPassword(newPassword, s.cfg.JWT.BcryptCost)
	if err != nil {
		return fmt.Errorf("auth: hash new password: %w", err)
	}
	if err := s.repo.UpdateUserPassword(ctx, user.ID, newHash); err != nil {
		return fmt.Errorf("auth: update password: %w", err)
	}

	// Conservative default: revoke all refresh tokens, including this device,
	// because the endpoint is authenticated by access token without refresh JTI.
	_, _ = s.repo.RevokeAllRefreshByUser(ctx, user.ID, PasswordChanged)
	s.logAuditTarget(ctx, "password_changed", &user.ID, roleStrPtr(user.Role), &user.ID, nil, ipAddr, ua, now)
	return nil
}

// ListSessions returns active, unexpired refresh sessions for a user.
func (s *Service) ListSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error) {
	sessions, err := s.repo.ListUserSessions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: list sessions: %w", err)
	}
	return sessions, nil
}

func (s *Service) logLoginAttempt(ctx context.Context, email string, ip, userAgent *string, success bool, reason *string, at time.Time) {
	_ = s.repo.LogLoginAttempt(ctx, &LoginAttempt{
		Email:     email,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		Reason:    reason,
		At:        at,
	})
}

func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole *string, meta datatypes.JSON, ip, userAgent *string, at time.Time) {
	_ = s.repo.LogAudit(ctx, &AuditLog{
		ActorID:   actorID,
		ActorRole: actorRole,
		Action:    action,
		Meta:      meta,
		IP:        ip,
		UserAgent: userAgent,
		At:        at,
	})
}

func (s *Service) logAuditTarget(ctx context.Context, action string, actorID *uuid.UUID, actorRole *string, targetID *uuid.UUID, meta datatypes.JSON, ip, userAgent *string, at time.Time) {
	_ = s.repo.LogAudit(ctx, &AuditLog{
		ActorID:   actorID,
		ActorRole: actorRole,
		Action:    action,
		TargetID:  targetID,
		Meta:      meta,
		IP:        ip,
		UserAgent: userAgent,
		At:        at,
	})
}

func auditMeta(fields map[string]string) datatypes.JSON {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func ipPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func uaPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func roleStrPtr(r UserRole) *string {
	v := string(r)
	return &v
}

func strPtr(s string) *string {
	v := s
	return &v
}
