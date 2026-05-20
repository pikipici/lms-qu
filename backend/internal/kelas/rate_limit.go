// Rate-limit middlewares scoped to enrollment flows.
package kelas

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/pikip/lms/backend/internal/middleware"
)

// JoinKodeRateLimit caps siswa join attempts per (IP, authenticated user) to
// `perWindow` requests per minute. Brute-force kode-invite scraping → 429.
//
// Key includes user_id from middleware.UserIDFromCtx so two users behind a
// shared NAT don't poison each other's quota. Falls back to IP only if user
// context isn't populated (shouldn't happen — BearerAuth runs first).
func JoinKodeRateLimit(perMin int) fiber.Handler {
	if perMin <= 0 {
		perMin = 10
	}
	return limiter.New(limiter.Config{
		Max:        perMin,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			ip := c.IP()
			uid, err := middleware.UserIDFromCtx(c)
			if err != nil {
				return ip + "|(anon)"
			}
			return ip + "|" + strings.ToLower(uid.String())
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":      "too many join attempts; try again in a minute",
				"code":       "too_many_requests",
				"request_id": middleware.RequestIDFromFiber(c),
			})
		},
		SkipFailedRequests: false,
	})
}
