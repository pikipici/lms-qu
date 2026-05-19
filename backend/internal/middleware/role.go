// Package middleware: role guard
//
// RoleGuard rejects requests whose authenticated role is not in the
// allowlist. Must be mounted AFTER BearerAuth so user_role is set.
package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// RoleGuard returns a Fiber handler that allows only the specified roles.
// 403 forbidden if the role is missing or not in allowedRoles.
func RoleGuard(allowedRoles ...string) fiber.Handler {
	allowSet := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowSet[r] = struct{}{}
	}
	return func(c *fiber.Ctx) error {
		role, ok := c.Locals(LocalsUserRole).(string)
		if !ok || role == "" {
			return forbidden(c, "role not present in context")
		}
		if _, allowed := allowSet[role]; !allowed {
			return forbidden(c, "insufficient role")
		}
		return c.Next()
	}
}

func forbidden(c *fiber.Ctx, message string) error {
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"error":      message,
		"code":       "forbidden",
		"request_id": RequestIDFromFiber(c),
	})
}
