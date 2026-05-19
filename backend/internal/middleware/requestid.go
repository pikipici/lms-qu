// Package middleware contains shared HTTP middleware for the LMS backend:
// request-id propagation, structured logging, panic recovery, rate limiting,
// and (later) auth + role guards.
package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// requestIDKey is the context key under which the request ID is stored.
type requestIDKey struct{}

// HeaderRequestID is the canonical header name for the request ID.
// We accept both incoming and emit it on every response (locked #49).
const HeaderRequestID = "X-Request-ID"

// RequestID returns a Fiber middleware that ensures every request has a
// stable identifier — taken from `X-Request-ID` header if present, otherwise
// generated as a UUIDv4. The ID is stored in fiber Locals and echoed back
// in the response header so clients can correlate logs.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		c.Locals("request_id", id)
		c.Set(HeaderRequestID, id)
		return c.Next()
	}
}

// FromContext returns the request ID stored in a stdlib context, if any.
// Used by goroutines spawned off a request lifecycle (e.g. async logging).
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDFromFiber pulls the request ID from a Fiber context (set by
// the RequestID middleware).
func RequestIDFromFiber(c *fiber.Ctx) string {
	if v, ok := c.Locals("request_id").(string); ok {
		return v
	}
	return ""
}

// Logger returns a structured access-log middleware that emits one slog
// entry per request, including request_id, method, path, status, and
// duration. We deliberately do NOT log full bodies — request_id lets ops
// look up details without leaking PII.
func Logger(log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		dur := time.Since(start)

		status := c.Response().StatusCode()
		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 {
			level = slog.LevelWarn
		}

		log.LogAttrs(c.UserContext(), level, "http",
			slog.String("request_id", RequestIDFromFiber(c)),
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", status),
			slog.Int64("duration_ms", dur.Milliseconds()),
			slog.String("ip", c.IP()),
			slog.String("ua", c.Get("User-Agent")),
		)
		return err
	}
}

// Recover catches panics in handlers and returns a 500 with the request ID
// so the user can report it to admin.
func Recover(log *slog.Logger) fiber.Handler {
	return func(c *fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				rid := RequestIDFromFiber(c)
				log.Error("panic recovered",
					slog.String("request_id", rid),
					slog.Any("panic", r),
					slog.String("path", c.Path()),
				)
				err = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":      "internal_server_error",
					"code":       "internal",
					"request_id": rid,
				})
			}
		}()
		return c.Next()
	}
}
