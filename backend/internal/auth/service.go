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
	IncFailedLogin(ctx context.Context, userID uuid.UUID) error
	ResetFailedLogin(ctx context.Context, userID uuid.UUID) error
	LockUser(ctx context.Context, userID uuid.UUID, reason string) error
	IssueRefresh(ctx context.Context, t *RefreshToken) error
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

// Login sentinel errors returned by Service methods.
var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUserSuspended      = errors.New("auth: user suspended")
	ErrUserLocked         = errors.New("auth: user locked")
	ErrRateLimited        = errors.New("auth: too many failed attempts; try again later")
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
