package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

type stubSvc struct {
	result *LoginResult
	err    error

	refreshResult *LoginResult
	refreshErr    error
	logoutErr     error
	logoutAllErr  error
	sessions      []RefreshToken
	sessionsErr   error

	calls     int
	email     string
	password  string
	ip        string
	userAgent string

	refreshCalls    int
	refreshToken    string
	logoutCalls     int
	logoutToken     string
	logoutAllCalls  int
	logoutAllUserID uuid.UUID
	sessionsCalls   int
	sessionsUserID  uuid.UUID
}

func (s *stubSvc) Login(ctx context.Context, email, password, ip, userAgent string) (*LoginResult, error) {
	s.calls++
	s.email = email
	s.password = password
	s.ip = ip
	s.userAgent = userAgent
	return s.result, s.err
}

func (s *stubSvc) Refresh(ctx context.Context, refreshToken, ip, userAgent string) (*LoginResult, error) {
	s.refreshCalls++
	s.refreshToken = refreshToken
	s.ip = ip
	s.userAgent = userAgent
	if s.refreshResult != nil || s.refreshErr != nil {
		return s.refreshResult, s.refreshErr
	}
	return s.result, s.err
}

func (s *stubSvc) Logout(ctx context.Context, refreshToken, ip, userAgent string) error {
	s.logoutCalls++
	s.logoutToken = refreshToken
	s.ip = ip
	s.userAgent = userAgent
	return s.logoutErr
}

func (s *stubSvc) LogoutAll(ctx context.Context, userID uuid.UUID, ip, userAgent string) error {
	s.logoutAllCalls++
	s.logoutAllUserID = userID
	s.ip = ip
	s.userAgent = userAgent
	return s.logoutAllErr
}

func (s *stubSvc) ListSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error) {
	s.sessionsCalls++
	s.sessionsUserID = userID
	return s.sessions, s.sessionsErr
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

func TestHandler_Refresh_Success(t *testing.T) {
	svc := &stubSvc{result: testLoginResult()}
	app := testAuthApp(svc)

	resp := postJSON(t, app, "/refresh", `{"refresh_token":"fake.refresh.token"}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	var body struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.Tokens.AccessToken != "fake.access.token" {
		t.Fatalf("tokens.access_token = %q, want fake.access.token", body.Tokens.AccessToken)
	}
	if svc.refreshCalls != 1 {
		t.Fatalf("Refresh calls = %d, want 1", svc.refreshCalls)
	}
	if svc.refreshToken != "fake.refresh.token" {
		t.Fatalf("Refresh token = %q, want fake.refresh.token", svc.refreshToken)
	}
}

func TestHandler_Refresh_InvalidCredentials(t *testing.T) {
	assertRefreshError(t, ErrInvalidCredentials, fiber.StatusUnauthorized, "invalid_credentials")
}

func TestHandler_Refresh_ReuseDetected(t *testing.T) {
	assertRefreshError(t, ErrRefreshReuse, fiber.StatusUnauthorized, "invalid_credentials")
}

func TestHandler_Refresh_BadBody(t *testing.T) {
	app := testAuthApp(&stubSvc{result: testLoginResult()})

	resp := postJSON(t, app, "/refresh", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestHandler_Logout_Success(t *testing.T) {
	svc := &stubSvc{}
	app := testAuthApp(svc)

	resp := postJSON(t, app, "/logout", `{"refresh_token":"fake.refresh.token"}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("body len = %d, want 0", len(body))
	}
	if svc.logoutCalls != 1 {
		t.Fatalf("Logout calls = %d, want 1", svc.logoutCalls)
	}
	if svc.logoutToken != "fake.refresh.token" {
		t.Fatalf("Logout token = %q, want fake.refresh.token", svc.logoutToken)
	}
}

func TestHandler_Logout_BadBody(t *testing.T) {
	app := testAuthApp(&stubSvc{})

	resp := postJSON(t, app, "/logout", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestHandler_LogoutAll_NoContext(t *testing.T) {
	app := testAuthApp(&stubSvc{})

	resp := postJSON(t, app, "/logout-all", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestHandler_LogoutAll_Success(t *testing.T) {
	svc := &stubSvc{}
	app := fiber.New()
	userID := uuid.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		return c.Next()
	})
	h := &Handler{svc: svc}
	app.Post("/logout-all", h.LogoutAll)

	resp := postJSON(t, app, "/logout-all", `{}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
	}
	if svc.logoutAllCalls != 1 {
		t.Fatalf("LogoutAll calls = %d, want 1", svc.logoutAllCalls)
	}
	if svc.logoutAllUserID != userID {
		t.Fatalf("LogoutAll userID = %s, want %s", svc.logoutAllUserID, userID)
	}
}

func TestHandler_Sessions_Success(t *testing.T) {
	userID := uuid.New()
	svc := &stubSvc{
		sessions: []RefreshToken{
			{JTI: uuid.New(), UserID: userID},
			{JTI: uuid.New(), UserID: userID},
		},
	}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		return c.Next()
	})
	h := &Handler{svc: svc}
	app.Get("/sessions", h.Sessions)

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	var body struct {
		Sessions []RefreshToken `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(body.Sessions) != 2 {
		t.Fatalf("sessions len = %d, want 2", len(body.Sessions))
	}
	if svc.sessionsCalls != 1 {
		t.Fatalf("ListSessions calls = %d, want 1", svc.sessionsCalls)
	}
	if svc.sessionsUserID != userID {
		t.Fatalf("ListSessions userID = %s, want %s", svc.sessionsUserID, userID)
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

func assertRefreshError(t *testing.T, err error, wantStatus int, wantCode string) {
	t.Helper()

	app := testAuthApp(&stubSvc{refreshErr: err})
	resp := postJSON(t, app, "/refresh", `{"refresh_token":"fake.refresh.token"}`)
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

func testAuthApp(svc *stubSvc) *fiber.App {
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Post("/login", h.Login)
	app.Post("/refresh", h.Refresh)
	app.Post("/logout", h.Logout)
	app.Post("/logout-all", h.LogoutAll)
	app.Get("/sessions", h.Sessions)
	return app
}

func postLogin(t *testing.T, app *fiber.App, body string) *http.Response {
	t.Helper()

	return postJSON(t, app, "/login", body)
}

func postJSON(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
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
