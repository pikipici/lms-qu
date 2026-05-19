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

	"github.com/pikip/lms/backend/internal/middleware"
)

type loginService interface {
	Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error)
}

// Handler owns HTTP handlers for the auth domain.
type Handler struct {
	svc loginService
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
		// Fiber's limiter counts every request in the window, not only failed
		// logins. Service.Login remains the precise 5-failed-attempts policy.
		SkipFailedRequests: false,
	})
}

func authError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
