package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type stubSvc struct {
	result *LoginResult
	err    error

	calls     int
	email     string
	password  string
	ip        string
	userAgent string
}

func (s *stubSvc) Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error) {
	s.calls++
	s.email = email
	s.password = password
	s.ip = ip
	s.userAgent = userAgent
	return s.result, s.err
}

func TestHandler_Login_Success(t *testing.T) {
	svc := &stubSvc{result: testLoginResult()}
	app := testLoginApp(svc)

	resp := postLogin(t, app, `{"email":"user@example.com","password":"secret"}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	var body struct {
		User struct {
			Email string `json:"email"`
		} `json:"user"`
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.User.Email != "user@example.com" {
		t.Fatalf("user.email = %q, want user@example.com", body.User.Email)
	}
	if body.Tokens.AccessToken != "fake.access.token" {
		t.Fatalf("tokens.access_token = %q, want fake.access.token", body.Tokens.AccessToken)
	}
	if svc.calls != 1 {
		t.Fatalf("Login calls = %d, want 1", svc.calls)
	}
	if svc.email != "user@example.com" {
		t.Fatalf("Login email = %q, want user@example.com", svc.email)
	}
	if svc.password != "secret" {
		t.Fatalf("Login password = %q, want secret", svc.password)
	}
	if svc.userAgent != "handler-test" {
		t.Fatalf("Login userAgent = %q, want handler-test", svc.userAgent)
	}
	if svc.ip == "" {
		t.Fatal("Login ip is empty")
	}
}

func TestHandler_Login_InvalidCredentials(t *testing.T) {
	assertLoginError(t, ErrInvalidCredentials, fiber.StatusUnauthorized, "invalid_credentials")
}

func TestHandler_Login_Suspended(t *testing.T) {
	assertLoginError(t, ErrUserSuspended, fiber.StatusForbidden, "user_suspended")
}

func TestHandler_Login_Locked(t *testing.T) {
	assertLoginError(t, ErrUserLocked, fiber.StatusForbidden, "user_locked")
}

func TestHandler_Login_RateLimited(t *testing.T) {
	assertLoginError(t, ErrRateLimited, fiber.StatusTooManyRequests, "too_many_requests")
}

func TestHandler_Login_BadJSON(t *testing.T) {
	app := testLoginApp(&stubSvc{result: testLoginResult()})

	resp := postLogin(t, app, `{not json`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestHandler_Login_MissingFields(t *testing.T) {
	app := testLoginApp(&stubSvc{result: testLoginResult()})

	resp := postLogin(t, app, `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func assertLoginError(t *testing.T, err error, wantStatus int, wantCode string) {
	t.Helper()

	app := testLoginApp(&stubSvc{err: err})
	resp := postLogin(t, app, `{"email":"user@example.com","password":"secret"}`)
	defer resp.Body.Close()

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

func testLoginApp(svc *stubSvc) *fiber.App {
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Post("/login", h.Login)
	return app
}

func postLogin(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "handler-test")
	req.RemoteAddr = "203.0.113.10:1234"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return resp
}

func testLoginResult() *LoginResult {
	userID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	return &LoginResult{
		User: &User{
			ID:                 userID,
			Name:               "Test User",
			Email:              "user@example.com",
			Role:               Guru,
			Status:             Active,
			MustChangePassword: false,
		},
		AccessToken:      "fake.access.token",
		AccessExpiresAt:  now.Add(15 * time.Minute),
		RefreshToken:     "fake.refresh.token",
		RefreshJTI:       "22222222-2222-2222-2222-222222222222",
		RefreshExpiresAt: now.Add(7 * 24 * time.Hour),
	}
}
