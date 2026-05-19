// Package middleware — rate limiting helpers.
//
// In MVP we use Fiber's in-memory limiter (#47). When notifications later
// adopt Redis (v0.8+), the same key strategy can move to a Redis store.
package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// GlobalRateLimit returns a per-IP rate limiter applied to every request.
// Default: 120 req/min per IP (config-driven).
func GlobalRateLimit(perMin int) fiber.Handler {
	if perMin <= 0 {
		// Defensive: a misconfigured 0 would lock everyone out.
		perMin = 120
	}
	return limiter.New(limiter.Config{
		Max:        perMin,
		Expiration: time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":      "rate_limit_exceeded",
				"code":       "too_many_requests",
				"request_id": RequestIDFromFiber(c),
			})
		},
	})
}
