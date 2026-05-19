package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestRoleGuard_AllowsAdmin(t *testing.T) {
	app := testRoleGuardApp("admin", RoleGuard("admin"))

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/protected")
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestRoleGuard_RejectsGuru(t *testing.T) {
	app := testRoleGuardApp("guru", RoleGuard("admin"))

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/protected")
	defer resp.Body.Close()

	assertMiddlewareErrorCode(t, resp, fiber.StatusForbidden, "forbidden")
}

func TestRoleGuard_NoLocals_403(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/protected", RoleGuard("admin"), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	resp := testMiddlewareRequest(t, app, http.MethodGet, "/protected")
	defer resp.Body.Close()

	assertMiddlewareErrorCode(t, resp, fiber.StatusForbidden, "forbidden")
}

func testRoleGuardApp(role string, guard fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Use(RequestID())
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(LocalsUserRole, role)
		return c.Next()
	})
	app.Get("/protected", guard, func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func testMiddlewareRequest(t *testing.T, app *fiber.App, method, path string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return resp
}

func assertMiddlewareErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
	t.Helper()

	if resp.StatusCode != wantStatus {
		t.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Code != wantCode {
		t.Fatalf("code = %q, want %q", body.Code, wantCode)
	}
}
