package middleware

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestForceChange_AllowsWhenFlagFalse(t *testing.T) {
	app := testForceChangeApp(false, "/api/v1/auth/sessions")

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/api/v1/auth/sessions")
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestForceChange_BlocksProtectedRoute(t *testing.T) {
	app := testForceChangeApp(true, "/api/v1/auth/sessions")

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/api/v1/auth/sessions")
	defer resp.Body.Close()

	assertMiddlewareErrorCode(t, resp, fiber.StatusForbidden, "must_change_password")
}

func TestForceChange_AllowsWhitelist(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{name: "me", method: http.MethodGet, path: "/api/v1/auth/me"},
		{name: "change password", method: http.MethodPost, path: "/api/v1/auth/change-password"},
		{name: "logout", method: http.MethodPost, path: "/api/v1/auth/logout"},
		{name: "logout all", method: http.MethodPost, path: "/api/v1/auth/logout-all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := testForceChangeApp(true, tc.path)

			resp := testMiddlewareRequest(t, app, tc.method, tc.path)
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
			}
		})
	}
}

func TestForceChange_NoLocalsMeansFalse(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/api/v1/auth/sessions", ForceChangePassword(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/api/v1/auth/sessions")
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func testForceChangeApp(mustChange bool, path string) *fiber.App {
	app := fiber.New()
	app.Use(RequestID())
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(LocalsMustChangePassword, mustChange)
		return c.Next()
	})
	app.All(path, ForceChangePassword(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}
