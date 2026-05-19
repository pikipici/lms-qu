package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/middleware"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type stubRepo struct {
	listUsersFn func(context.Context, auth.UserListFilter, int, int) ([]auth.User, int64, error)
	listCalls   int
	listFilter  auth.UserListFilter
	listLimit   int
	listOffset  int

	findEmailFn    func(context.Context, string) (*auth.User, error)
	findEmailCalls int
	findEmail      string

	findIDFn    func(context.Context, uuid.UUID) (*auth.User, error)
	findIDCalls int
	findID      uuid.UUID

	createUserFn func(context.Context, *auth.User) error
	createCalls  int
	createdUser  *auth.User

	updateNameFn func(context.Context, uuid.UUID, string) error
	updateCalls  int
	updateID     uuid.UUID
	updateName   string

	suspendFn    func(context.Context, uuid.UUID) error
	suspendCalls int
	suspendID    uuid.UUID

	revokeFn     func(context.Context, uuid.UUID, auth.RevokedReason) (int64, error)
	revokeCalls  int
	revokeUserID uuid.UUID
	revokeReason auth.RevokedReason

	countAdminsFn    func(context.Context) (int64, error)
	countAdminsCalls int

	logAuditFn func(context.Context, *auth.AuditLog) error
	auditCalls int
	audits     []*auth.AuditLog
}

func (s *stubRepo) ListUsers(ctx context.Context, f auth.UserListFilter, limit, offset int) ([]auth.User, int64, error) {
	s.listCalls++
	s.listFilter = f
	s.listLimit = limit
	s.listOffset = offset
	if s.listUsersFn != nil {
		return s.listUsersFn(ctx, f, limit, offset)
	}
	return nil, 0, nil
}

func (s *stubRepo) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	s.findEmailCalls++
	s.findEmail = email
	if s.findEmailFn != nil {
		return s.findEmailFn(ctx, email)
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	s.findIDCalls++
	s.findID = id
	if s.findIDFn != nil {
		return s.findIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubRepo) CreateUser(ctx context.Context, u *auth.User) error {
	s.createCalls++
	cp := *u
	s.createdUser = &cp
	if s.createUserFn != nil {
		return s.createUserFn(ctx, u)
	}
	return nil
}

func (s *stubRepo) UpdateUserName(ctx context.Context, id uuid.UUID, name string) error {
	s.updateCalls++
	s.updateID = id
	s.updateName = name
	if s.updateNameFn != nil {
		return s.updateNameFn(ctx, id, name)
	}
	return nil
}

func (s *stubRepo) SuspendUser(ctx context.Context, id uuid.UUID) error {
	s.suspendCalls++
	s.suspendID = id
	if s.suspendFn != nil {
		return s.suspendFn(ctx, id)
	}
	return nil
}

func (s *stubRepo) RevokeAllRefreshByUser(ctx context.Context, userID uuid.UUID, reason auth.RevokedReason) (int64, error) {
	s.revokeCalls++
	s.revokeUserID = userID
	s.revokeReason = reason
	if s.revokeFn != nil {
		return s.revokeFn(ctx, userID, reason)
	}
	return 0, nil
}

func (s *stubRepo) CountAdmins(ctx context.Context) (int64, error) {
	s.countAdminsCalls++
	if s.countAdminsFn != nil {
		return s.countAdminsFn(ctx)
	}
	return 0, nil
}

func (s *stubRepo) LogAudit(ctx context.Context, entry *auth.AuditLog) error {
	s.auditCalls++
	cp := *entry
	s.audits = append(s.audits, &cp)
	if s.logAuditFn != nil {
		return s.logAuditFn(ctx, entry)
	}
	return nil
}

func TestHandler_ListUsers(t *testing.T) {
	t.Run("happy path 200", func(t *testing.T) {
		userID := uuid.New()
		repo := &stubRepo{
			listUsersFn: func(context.Context, auth.UserListFilter, int, int) ([]auth.User, int64, error) {
				return []auth.User{{
					ID:     userID,
					Name:   "Ada",
					Email:  "ada@example.com",
					Role:   auth.Guru,
					Status: auth.Active,
				}}, 1, nil
			},
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users?role=guru&status=active&q=ada&page=2&page_size=10", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body listUsersResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if len(body.Users) != 1 || body.Users[0].ID != userID {
			t.Fatalf("users = %+v, want one user %s", body.Users, userID)
		}
		if body.Page != 2 || body.PageSize != 10 || body.Total != 1 || body.TotalPages != 1 {
			t.Fatalf("pagination = page %d size %d total %d pages %d", body.Page, body.PageSize, body.Total, body.TotalPages)
		}
		if repo.listFilter.Role != "guru" || repo.listFilter.Status != "active" {
			t.Fatalf("filter role/status = %q/%q", repo.listFilter.Role, repo.listFilter.Status)
		}
		if repo.listFilter.SearchEmail != "ada" || repo.listFilter.SearchName != "ada" {
			t.Fatalf("filter q = %q/%q, want ada/ada", repo.listFilter.SearchEmail, repo.listFilter.SearchName)
		}
		if repo.listLimit != 10 || repo.listOffset != 10 {
			t.Fatalf("limit/offset = %d/%d, want 10/10", repo.listLimit, repo.listOffset)
		}
	})

	t.Run("invalid_role -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users?role=bad", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_role")
		if repo.listCalls != 0 {
			t.Fatalf("ListUsers calls = %d, want 0", repo.listCalls)
		}
	})

	t.Run("invalid_status -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users?status=bad", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_status")
		if repo.listCalls != 0 {
			t.Fatalf("ListUsers calls = %d, want 0", repo.listCalls)
		}
	})

	t.Run("default pagination defaults", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listLimit != defaultPageSize || repo.listOffset != 0 {
			t.Fatalf("limit/offset = %d/%d, want %d/0", repo.listLimit, repo.listOffset, defaultPageSize)
		}
		var body listUsersResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.Page != 1 || body.PageSize != defaultPageSize {
			t.Fatalf("page/page_size = %d/%d, want 1/%d", body.Page, body.PageSize, defaultPageSize)
		}
	})

	t.Run("page_size > 100 clamped to 100", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users?page_size=500", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listLimit != maxPageSize {
			t.Fatalf("limit = %d, want %d", repo.listLimit, maxPageSize)
		}
	})
}

func TestHandler_CreateUser(t *testing.T) {
	t.Run("happy manual -> 201 user", func(t *testing.T) {
		adminID := uuid.New()
		repo := &stubRepo{}
		app := testAdminApp(repo, adminID)

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":" Teacher ","email":"TEACHER@example.com","role":"guru","password_strategy":"manual","password":"secret123"}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
		}
		var body createUserResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.GeneratedPassword != nil {
			t.Fatalf("generated_password = %q, want nil", *body.GeneratedPassword)
		}
		if body.User.Email != "teacher@example.com" || body.User.Name != "Teacher" || body.User.Role != auth.Guru {
			t.Fatalf("user = %+v", body.User)
		}
		if repo.createCalls != 1 {
			t.Fatalf("CreateUser calls = %d, want 1", repo.createCalls)
		}
		if repo.createdUser.CreatedByID == nil || *repo.createdUser.CreatedByID != adminID {
			t.Fatalf("CreatedByID = %v, want %s", repo.createdUser.CreatedByID, adminID)
		}
		if !repo.createdUser.MustChangePassword || repo.createdUser.Status != auth.Active {
			t.Fatalf("created status/must_change = %s/%v", repo.createdUser.Status, repo.createdUser.MustChangePassword)
		}
		if repo.createdUser.PasswordHash == "" || repo.createdUser.PasswordHash == "secret123" {
			t.Fatal("password was not hashed")
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_created" {
			t.Fatalf("audit = %+v", repo.audits)
		}
	})

	t.Run("happy generate -> 201 + generated_password non-empty len 16", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Student","email":"student@example.com","role":"siswa","password_strategy":"generate"}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusCreated)
		}
		var body createUserResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.GeneratedPassword == nil || len(*body.GeneratedPassword) != generatedLength {
			t.Fatalf("generated_password len = %v, want %d", body.GeneratedPassword, generatedLength)
		}
		if repo.createCalls != 1 {
			t.Fatalf("CreateUser calls = %d, want 1", repo.createCalls)
		}
	})

	t.Run("weak_password -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Teacher","email":"teacher@example.com","role":"guru","password_strategy":"manual","password":"short"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "weak_password")
	})

	t.Run("conflicting_password -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Teacher","email":"teacher@example.com","role":"guru","password_strategy":"generate","password":"secret123"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "conflicting_password")
	})

	t.Run("invalid_role -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Teacher","email":"teacher@example.com","role":"student","password_strategy":"manual","password":"secret123"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_role")
	})

	t.Run("email_already_exists -> 409", func(t *testing.T) {
		repo := &stubRepo{
			findEmailFn: func(context.Context, string) (*auth.User, error) {
				return &auth.User{ID: uuid.New(), Email: "teacher@example.com"}, nil
			},
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Teacher","email":"teacher@example.com","role":"guru","password_strategy":"manual","password":"secret123"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusConflict, "email_already_exists")
	})

	t.Run("missing email -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users", `{"name":"Teacher","role":"guru","password_strategy":"manual","password":"secret123"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_body")
	})
}

func TestHandler_UpdateUser(t *testing.T) {
	t.Run("happy 200", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Old Name",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPatch, "/api/v1/admin/users/"+targetID.String(), `{"name":" New Name "}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body struct {
			User auth.User `json:"user"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.User.Name != "New Name" {
			t.Fatalf("user.name = %q, want New Name", body.User.Name)
		}
		if repo.updateCalls != 1 || repo.updateName != "New Name" {
			t.Fatalf("update calls/name = %d/%q", repo.updateCalls, repo.updateName)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_name_updated" {
			t.Fatalf("audit = %+v", repo.audits)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPatch, "/api/v1/admin/users/not-a-uuid", `{"name":"New Name"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPatch, "/api/v1/admin/users/"+uuid.NewString(), `{"name":"New Name"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
	})

	t.Run("empty name -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPatch, "/api/v1/admin/users/"+uuid.NewString(), `{"name":"   "}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_body")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})
}

func TestHandler_DeleteUser(t *testing.T) {
	t.Run("happy 204", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodDelete, "/api/v1/admin/users/"+targetID.String(), "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusNoContent {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusNoContent)
		}
		if repo.suspendCalls != 1 || repo.suspendID != targetID {
			t.Fatalf("SuspendUser calls/id = %d/%s", repo.suspendCalls, repo.suspendID)
		}
		if repo.revokeCalls != 1 || repo.revokeUserID != targetID || repo.revokeReason != auth.AdminReset {
			t.Fatalf("revoke = calls %d user %s reason %s", repo.revokeCalls, repo.revokeUserID, repo.revokeReason)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_suspended" {
			t.Fatalf("audit = %+v", repo.audits)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodDelete, "/api/v1/admin/users/not-a-uuid", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodDelete, "/api/v1/admin/users/"+uuid.NewString(), "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
	})

	t.Run("last_admin_protected -> 400", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Admin",
			Email:  "admin@example.com",
			Role:   auth.Admin,
			Status: auth.Active,
		})
		repo.countAdminsFn = func(context.Context) (int64, error) { return 1, nil }
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodDelete, "/api/v1/admin/users/"+targetID.String(), "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "last_admin_protected")
		if repo.suspendCalls != 0 {
			t.Fatalf("SuspendUser calls = %d, want 0", repo.suspendCalls)
		}
	})

	t.Run("cannot_delete_self -> 400", func(t *testing.T) {
		adminID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     adminID,
			Name:   "Admin",
			Email:  "admin@example.com",
			Role:   auth.Admin,
			Status: auth.Active,
		})
		repo.countAdminsFn = func(context.Context) (int64, error) { return 2, nil }
		app := testAdminApp(repo, adminID)

		resp := doAdminRequest(t, app, http.MethodDelete, "/api/v1/admin/users/"+adminID.String(), "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "cannot_delete_self")
		if repo.suspendCalls != 0 {
			t.Fatalf("SuspendUser calls = %d, want 0", repo.suspendCalls)
		}
	})
}

func repoWithUsers(users ...*auth.User) *stubRepo {
	store := make(map[uuid.UUID]*auth.User, len(users))
	for _, user := range users {
		cp := *user
		store[cp.ID] = &cp
	}

	repo := &stubRepo{}
	repo.findIDFn = func(_ context.Context, id uuid.UUID) (*auth.User, error) {
		user, ok := store[id]
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		cp := *user
		return &cp, nil
	}
	repo.updateNameFn = func(_ context.Context, id uuid.UUID, name string) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.Name = name
		return nil
	}
	repo.suspendFn = func(_ context.Context, id uuid.UUID) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.Status = auth.Suspended
		return nil
	}
	return repo
}

func testAdminApp(repo *stubRepo, adminID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, adminID)
		c.Locals(middleware.LocalsUserRole, string(auth.Admin))
		return c.Next()
	})

	h := NewHandler(repo, &config.Config{
		JWT: config.JWTConfig{BcryptCost: bcrypt.MinCost},
	})
	app.Get("/api/v1/admin/users", h.ListUsers)
	app.Post("/api/v1/admin/users", h.CreateUser)
	app.Patch("/api/v1/admin/users/:id", h.UpdateUser)
	app.Delete("/api/v1/admin/users/:id", h.DeleteUser)
	return app
}

func doAdminRequest(t *testing.T, app *fiber.App, method, path, body string) *http.Response {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "admin-handler-test")
	req.RemoteAddr = "203.0.113.20:1234"
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	return resp
}

func assertErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
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
