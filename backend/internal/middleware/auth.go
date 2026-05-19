package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	LocalsUserID             = "user_id"
	LocalsUserRole           = "user_role"
	LocalsUserEmail          = "user_email"
	LocalsMustChangePassword = "user_must_change_password"
)

// UserVerifier verifies a raw bearer token and returns user identity fields.
type UserVerifier interface {
	VerifyAccessToken(rawToken string) (userID uuid.UUID, role string, email string, mustChange bool, err error)
}

// BearerAuth requires a valid Authorization: Bearer <token> header.
func BearerAuth(verifier UserVerifier) fiber.Handler {
	return func(c *fiber.Ctx) error {
		h := c.Get(fiber.HeaderAuthorization)
		if h == "" {
			return unauthorized(c, "authorization header required")
		}

		parts := strings.SplitN(h, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			return unauthorized(c, "invalid authorization header")
		}

		userID, role, email, mustChange, err := verifier.VerifyAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			return unauthorized(c, "invalid or expired token")
		}

		c.Locals(LocalsUserID, userID)
		c.Locals(LocalsUserRole, role)
		c.Locals(LocalsUserEmail, email)
		c.Locals(LocalsMustChangePassword, mustChange)
		return c.Next()
	}
}

// ErrNoUserContext means the bearer middleware did not populate user locals.
var ErrNoUserContext = errors.New("middleware: no user context")

// UserIDFromCtx pulls the authenticated user ID from request locals.
func UserIDFromCtx(c *fiber.Ctx) (uuid.UUID, error) {
	v := c.Locals(LocalsUserID)
	if v == nil {
		return uuid.Nil, ErrNoUserContext
	}
	id, ok := v.(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrNoUserContext
	}
	return id, nil
}

// MustChangePasswordFromCtx reports whether the authenticated user is
// required to change their password before accessing non-self-service routes.
func MustChangePasswordFromCtx(c *fiber.Ctx) bool {
	v, _ := c.Locals(LocalsMustChangePassword).(bool)
	return v
}

func unauthorized(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error":      message,
		"code":       "unauthorized",
		"request_id": RequestIDFromFiber(c),
	})
}
