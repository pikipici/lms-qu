// Package middleware: force-change-password gate
//
// ForceChangePassword blocks non-self-service routes when the authenticated
// user has must_change_password=true. Must be mounted AFTER BearerAuth.
//
// Whitelist (always allowed even with mustChange=true):
//   GET  /api/v1/auth/me
//   POST /api/v1/auth/change-password
//   POST /api/v1/auth/logout
//   POST /api/v1/auth/logout-all
package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

var forceChangeWhitelist = map[string]struct{}{
	"/api/v1/auth/me":              {},
	"/api/v1/auth/change-password": {},
	"/api/v1/auth/logout":          {},
	"/api/v1/auth/logout-all":      {},
}

// ForceChangePassword gates protected routes for users who must update
// their password. Returns 403 with code="must_change_password".
func ForceChangePassword() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !MustChangePasswordFromCtx(c) {
			return c.Next()
		}
		if _, ok := forceChangeWhitelist[strings.TrimRight(c.Path(), "/")]; ok {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":      "password change required; POST /api/v1/auth/change-password first",
			"code":       "must_change_password",
			"request_id": RequestIDFromFiber(c),
		})
	}
}
