// Package auth exposes authentication handlers and middleware for the LMS API.
//
// HTTP handlers keep request parsing, small validation, and domain sentinel
// error mapping close to Fiber. Login rate limiting is intentionally layered:
// this package's Fiber middleware is a coarse in-memory guard keyed by IP and
// email, while Service.Login enforces the precise failed-attempt policy from
// persisted LoginAttempt rows.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

type authService interface {
	Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error)
	Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*LoginResult, error)
	Logout(ctx context.Context, refreshToken, ip, userAgent string) error
	LogoutAll(ctx context.Context, userID uuid.UUID, ip, userAgent string) error
	Me(ctx context.Context, userID uuid.UUID) (*User, error)
	ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword, ip, userAgent string) error
	ListSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error)
}

// Handler owns HTTP handlers for the auth domain.
type Handler struct {
	svc authService
}

// NewHandler creates an auth HTTP handler backed by svc.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenPair struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type loginResponse struct {
	User   *User     `json:"user"`
	Tokens tokenPair `json:"tokens"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Login authenticates a user and returns the public user fields plus tokens.
func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email and password are required")
	}

	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	result, err := h.svc.Login(c.UserContext(), req.Email, req.Password, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			return authError(c, fiber.StatusUnauthorized, "invalid credentials", "invalid_credentials")
		case errors.Is(err, ErrUserSuspended):
			return authError(c, fiber.StatusForbidden, "account suspended", "user_suspended")
		case errors.Is(err, ErrUserLocked):
			return authError(c, fiber.StatusForbidden, "account locked", "user_locked")
		case errors.Is(err, ErrRateLimited):
			return authError(c, fiber.StatusTooManyRequests, "too many failed attempts; try again in 15 minutes", "too_many_requests")
		default:
			return fmt.Errorf("auth login: %w", err)
		}
	}

	return c.Status(fiber.StatusOK).JSON(loginResponse{
		User: result.User,
		Tokens: tokenPair{
			AccessToken:      result.AccessToken,
			AccessExpiresAt:  result.AccessExpiresAt,
			RefreshToken:     result.RefreshToken,
			RefreshExpiresAt: result.RefreshExpiresAt,
		},
	})
}

// Refresh rotates a valid refresh token and returns a fresh token pair.
func (h *Handler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "refresh_token is required")
	}

	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	result, err := h.svc.Refresh(c.UserContext(), req.RefreshToken, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials),
			errors.Is(err, ErrRefreshReuse):
			return authError(c, fiber.StatusUnauthorized, "invalid refresh token", "invalid_credentials")
		case errors.Is(err, ErrUserSuspended):
			return authError(c, fiber.StatusForbidden, "account suspended", "user_suspended")
		case errors.Is(err, ErrUserLocked):
			return authError(c, fiber.StatusForbidden, "account locked", "user_locked")
		default:
			return fmt.Errorf("auth refresh: %w", err)
		}
	}

	return c.Status(fiber.StatusOK).JSON(loginResponse{
		User: result.User,
		Tokens: tokenPair{
			AccessToken:      result.AccessToken,
			AccessExpiresAt:  result.AccessExpiresAt,
			RefreshToken:     result.RefreshToken,
			RefreshExpiresAt: result.RefreshExpiresAt,
		},
	})
}

// Logout revokes the provided refresh token if it can be identified.
func (h *Handler) Logout(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "refresh_token is required")
	}

	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	_ = h.svc.Logout(c.UserContext(), req.RefreshToken, ip, userAgent)
	return c.SendStatus(fiber.StatusNoContent)
}

// LogoutAll revokes every active refresh token for the authenticated user.
func (h *Handler) LogoutAll(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	if err := h.svc.LogoutAll(c.UserContext(), userID, ip, userAgent); err != nil {
		return fmt.Errorf("auth logout-all: %w", err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type sessionsResponse struct {
	Sessions []RefreshToken `json:"sessions"`
}

// Sessions lists active refresh sessions for the authenticated user.
func (h *Handler) Sessions(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	sessions, err := h.svc.ListSessions(c.UserContext(), userID)
	if err != nil {
		return fmt.Errorf("auth sessions: %w", err)
	}
	return c.Status(fiber.StatusOK).JSON(sessionsResponse{Sessions: sessions})
}

// Me returns the authenticated user's profile.
func (h *Handler) Me(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	user, err := h.svc.Me(c.UserContext(), userID)
	if err != nil {
		return fmt.Errorf("auth me: %w", err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": user})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword updates the authenticated user's password.
func (h *Handler) ChangePassword(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	var req changePasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.CurrentPassword) == "" || strings.TrimSpace(req.NewPassword) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "current_password and new_password are required")
	}

	ip := c.IP()
	userAgent := string(c.Request().Header.UserAgent())
	err = h.svc.ChangePassword(c.UserContext(), userID, req.CurrentPassword, req.NewPassword, ip, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, ErrCurrentPasswordIncorrect):
			return authError(c, fiber.StatusBadRequest, "current password is incorrect", "invalid_current_password")
		case errors.Is(err, ErrWeakPassword):
			return authError(c, fiber.StatusBadRequest, "new password must be at least 8 characters", "weak_password")
		case errors.Is(err, ErrSamePassword):
			return authError(c, fiber.StatusBadRequest, "new password must differ from current", "same_password")
		default:
			return fmt.Errorf("auth change-password: %w", err)
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// LoginRateLimit returns a coarse per-IP-and-email limiter for /auth/login.
func LoginRateLimit(perWindow int) fiber.Handler {
	if perWindow <= 0 {
		perWindow = 5
	}

	return limiter.New(limiter.Config{
		Max:        perWindow,
		Expiration: 15 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			var req struct {
				Email string `json:"email"`
			}
			_ = json.Unmarshal(c.Body(), &req)

			email := strings.ToLower(strings.TrimSpace(req.Email))
			if email == "" {
				email = "(empty)"
			}
			return c.IP() + "|" + email
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":      "too many login attempts; try again later",
				"code":       "too_many_requests",
				"request_id": middleware.RequestIDFromFiber(c),
			})
		},
		// Only failed login responses (>=400) count toward the limiter, mirroring
		// the DB-backed policy in Service.Login (failed_login_attempts cleared on
		// success/change-password/admin-reset). Successful logins must not consume
		// budget — otherwise typo + correct combos drain the window unfairly.
		SkipSuccessfulRequests: true,
		SkipFailedRequests:     false,
	})
}

// RefreshRateLimit returns a per-IP-and-refresh-token limiter for /auth/refresh.
func RefreshRateLimit(perMin int) fiber.Handler {
	if perMin <= 0 {
		perMin = 10
	}

	return limiter.New(limiter.Config{
		Max:        perMin,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			var req struct {
				RefreshToken string `json:"refresh_token"`
			}
			_ = json.Unmarshal(c.Body(), &req)

			token := strings.TrimSpace(req.RefreshToken)
			if token == "" {
				return c.IP() + "|(anon)"
			}

			claims := &RefreshClaims{}
			if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err == nil {
				jti := strings.TrimSpace(claims.ID)
				if _, err := uuid.Parse(jti); err == nil {
					return c.IP() + "|" + jti
				}
			}
			return c.IP() + "|(anon)"
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":      "too many refresh attempts; try again later",
				"code":       "too_many_requests",
				"request_id": middleware.RequestIDFromFiber(c),
			})
		},
	})
}

func authError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
