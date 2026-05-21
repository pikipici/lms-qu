package admin

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

	listSessionsFn     func(context.Context, uuid.UUID) ([]auth.RefreshToken, error)
	listSessionsCalls  int
	listSessionsUserID uuid.UUID

	listAuditLogsFn     func(context.Context, auth.AuditLogFilter, int, int) ([]auth.AuditLog, int64, error)
	listAuditLogsCalls  int
	listAuditLogsFilter auth.AuditLogFilter
	listAuditLogsLimit  int
	listAuditLogsOffset int

	listLoginAttemptsFn     func(context.Context, auth.LoginAttemptFilter, int, int) ([]auth.LoginAttempt, int64, error)
	listLoginAttemptsCalls  int
	listLoginAttemptsFilter auth.LoginAttemptFilter
	listLoginAttemptsLimit  int
	listLoginAttemptsOffset int

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

	updateUserRoleFn   func(context.Context, uuid.UUID, auth.UserRole) error
	updateUserRoleErr  error
	updateUserRoleArgs struct {
		id   uuid.UUID
		role auth.UserRole
	}
	updateUserRoleCalls int

	suspendFn    func(context.Context, uuid.UUID) error
	suspendCalls int
	suspendID    uuid.UUID

	adminResetPasswordFn    func(context.Context, uuid.UUID, string) error
	adminResetPasswordCalls int
	adminResetPasswordID    uuid.UUID
	adminResetPasswordHash  string

	unsuspendFn    func(context.Context, uuid.UUID) error
	unsuspendCalls int
	unsuspendID    uuid.UUID

	unlockFn    func(context.Context, uuid.UUID) error
	unlockCalls int
	unlockID    uuid.UUID

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

func (s *stubRepo) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]auth.RefreshToken, error) {
	s.listSessionsCalls++
	s.listSessionsUserID = userID
	if s.listSessionsFn != nil {
		return s.listSessionsFn(ctx, userID)
	}
	return nil, nil
}

func (s *stubRepo) ListAuditLogs(ctx context.Context, f auth.AuditLogFilter, limit, offset int) ([]auth.AuditLog, int64, error) {
	s.listAuditLogsCalls++
	s.listAuditLogsFilter = f
	s.listAuditLogsLimit = limit
	s.listAuditLogsOffset = offset
	if s.listAuditLogsFn != nil {
		return s.listAuditLogsFn(ctx, f, limit, offset)
	}
	return nil, 0, nil
}

func (s *stubRepo) ListLoginAttempts(ctx context.Context, f auth.LoginAttemptFilter, limit, offset int) ([]auth.LoginAttempt, int64, error) {
	s.listLoginAttemptsCalls++
	s.listLoginAttemptsFilter = f
	s.listLoginAttemptsLimit = limit
	s.listLoginAttemptsOffset = offset
	if s.listLoginAttemptsFn != nil {
		return s.listLoginAttemptsFn(ctx, f, limit, offset)
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

func (s *stubRepo) UpdateUserRole(ctx context.Context, id uuid.UUID, role auth.UserRole) error {
	s.updateUserRoleCalls++
	s.updateUserRoleArgs.id = id
	s.updateUserRoleArgs.role = role
	if s.updateUserRoleFn != nil {
		return s.updateUserRoleFn(ctx, id, role)
	}
	return s.updateUserRoleErr
}

func (s *stubRepo) SuspendUser(ctx context.Context, id uuid.UUID) error {
	s.suspendCalls++
	s.suspendID = id
	if s.suspendFn != nil {
		return s.suspendFn(ctx, id)
	}
	return nil
}

func (s *stubRepo) AdminResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	s.adminResetPasswordCalls++
	s.adminResetPasswordID = id
	s.adminResetPasswordHash = newHash
	if s.adminResetPasswordFn != nil {
		return s.adminResetPasswordFn(ctx, id, newHash)
	}
	return nil
}

func (s *stubRepo) UnsuspendUser(ctx context.Context, id uuid.UUID) error {
	s.unsuspendCalls++
	s.unsuspendID = id
	if s.unsuspendFn != nil {
		return s.unsuspendFn(ctx, id)
	}
	return nil
}

func (s *stubRepo) UnlockUser(ctx context.Context, id uuid.UUID) error {
	s.unlockCalls++
	s.unlockID = id
	if s.unlockFn != nil {
		return s.unlockFn(ctx, id)
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

func (s *stubRepo) ClearRecentFailedAttempts(ctx context.Context, email string, since time.Time) error {
	return nil
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

func TestHandler_ListTargetSessions(t *testing.T) {
	t.Run("happy 200 returns sessions array", func(t *testing.T) {
		targetID := uuid.New()
		tokenID := uuid.New()
		jti := uuid.New()
		issuedAt := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
		expiresAt := issuedAt.Add(24 * time.Hour)
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		repo.listSessionsFn = func(context.Context, uuid.UUID) ([]auth.RefreshToken, error) {
			return []auth.RefreshToken{{
				ID:        tokenID,
				JTI:       jti,
				UserID:    targetID,
				IssuedAt:  issuedAt,
				ExpiresAt: expiresAt,
			}}, nil
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/"+targetID.String()+"/sessions", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body listTargetSessionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if len(body.Sessions) != 1 || body.Sessions[0].ID != tokenID || body.Sessions[0].JTI != jti {
			t.Fatalf("sessions = %+v, want one token %s/%s", body.Sessions, tokenID, jti)
		}
		if repo.listSessionsCalls != 1 || repo.listSessionsUserID != targetID {
			t.Fatalf("ListUserSessions calls/id = %d/%s, want 1/%s", repo.listSessionsCalls, repo.listSessionsUserID, targetID)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/not-a-uuid/sessions", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
		if repo.findIDCalls != 0 || repo.listSessionsCalls != 0 {
			t.Fatalf("find/list calls = %d/%d, want 0/0", repo.findIDCalls, repo.listSessionsCalls)
		}
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/"+uuid.NewString()+"/sessions", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
		if repo.listSessionsCalls != 0 {
			t.Fatalf("ListUserSessions calls = %d, want 0", repo.listSessionsCalls)
		}
	})
}

func TestHandler_RevokeTargetSessions(t *testing.T) {
	t.Run("happy 200 returns revoked_count and audits reason", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		repo.revokeFn = func(context.Context, uuid.UUID, auth.RevokedReason) (int64, error) {
			return 3, nil
		}
		app := testAdminApp(repo, adminID)

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/revoke-sessions", `{"reason":" stale sessions "}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body revokeTargetSessionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.RevokedCount != 3 {
			t.Fatalf("revoked_count = %d, want 3", body.RevokedCount)
		}
		if repo.revokeCalls != 1 || repo.revokeUserID != targetID || repo.revokeReason != auth.AdminReset {
			t.Fatalf("revoke = calls %d user %s reason %s", repo.revokeCalls, repo.revokeUserID, repo.revokeReason)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_sessions_revoked" {
			t.Fatalf("audit = %+v", repo.audits)
		}
		if repo.audits[0].ActorID == nil || *repo.audits[0].ActorID != adminID {
			t.Fatalf("audit actor = %v, want %s", repo.audits[0].ActorID, adminID)
		}
		if repo.audits[0].TargetID == nil || *repo.audits[0].TargetID != targetID {
			t.Fatalf("audit target = %v, want %s", repo.audits[0].TargetID, targetID)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["revoked_count"] != float64(3) || meta["reason"] != "stale sessions" {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/revoke-sessions", `{"reason":"test"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
		if repo.revokeCalls != 0 {
			t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeCalls)
		}
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/revoke-sessions", `{"reason":"test"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
		if repo.revokeCalls != 0 {
			t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeCalls)
		}
	})

	t.Run("empty body OK", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		repo.revokeFn = func(context.Context, uuid.UUID, auth.RevokedReason) (int64, error) {
			return 1, nil
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/revoke-sessions", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body revokeTargetSessionsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.RevokedCount != 1 {
			t.Fatalf("revoked_count = %d, want 1", body.RevokedCount)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["reason"] != "" {
			t.Fatalf("audit reason = %q, want empty", meta["reason"])
		}
	})
}

func TestHandler_ListAuditLog(t *testing.T) {
	t.Run("happy 200 with default pagination", func(t *testing.T) {
		eventID := uuid.New()
		at := time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC)
		repo := &stubRepo{
			listAuditLogsFn: func(context.Context, auth.AuditLogFilter, int, int) ([]auth.AuditLog, int64, error) {
				return []auth.AuditLog{{
					ID:     eventID,
					Action: "admin_user_created",
					At:     at,
				}}, 1, nil
			},
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/audit-log", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body listAuditLogResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if len(body.Events) != 1 || body.Events[0].ID != eventID {
			t.Fatalf("events = %+v, want one event %s", body.Events, eventID)
		}
		if body.Page != 1 || body.PageSize != defaultPageSize || body.Total != 1 || body.TotalPages != 1 {
			t.Fatalf("pagination = page %d size %d total %d pages %d", body.Page, body.PageSize, body.Total, body.TotalPages)
		}
		if repo.listAuditLogsLimit != defaultPageSize || repo.listAuditLogsOffset != 0 {
			t.Fatalf("limit/offset = %d/%d, want %d/0", repo.listAuditLogsLimit, repo.listAuditLogsOffset, defaultPageSize)
		}
	})

	t.Run("filter by actor_id", func(t *testing.T) {
		actorID := uuid.New()
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/audit-log?actor_id="+actorID.String(), "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listAuditLogsFilter.ActorID == nil || *repo.listAuditLogsFilter.ActorID != actorID {
			t.Fatalf("actor_id filter = %v, want %s", repo.listAuditLogsFilter.ActorID, actorID)
		}
	})

	t.Run("filter by action", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/audit-log?action=admin_user_sessions_revoked", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listAuditLogsFilter.Action != "admin_user_sessions_revoked" {
			t.Fatalf("action filter = %q, want admin_user_sessions_revoked", repo.listAuditLogsFilter.Action)
		}
	})

	t.Run("invalid_actor_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/audit-log?actor_id=bad", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_actor_id")
		if repo.listAuditLogsCalls != 0 {
			t.Fatalf("ListAuditLogs calls = %d, want 0", repo.listAuditLogsCalls)
		}
	})

	t.Run("invalid_time -> 400", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/admin/audit-log?since=bad",
			"/api/v1/admin/audit-log?until=bad",
		} {
			repo := &stubRepo{}
			app := testAdminApp(repo, uuid.New())

			resp := doAdminRequest(t, app, http.MethodGet, path, "")
			defer resp.Body.Close()

			assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_time")
			if repo.listAuditLogsCalls != 0 {
				t.Fatalf("ListAuditLogs calls = %d, want 0", repo.listAuditLogsCalls)
			}
		}
	})

	t.Run("page_size clamping", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/audit-log?page_size=500", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body listAuditLogResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.PageSize != maxPageSize || repo.listAuditLogsLimit != maxPageSize {
			t.Fatalf("page_size/limit = %d/%d, want %d/%d", body.PageSize, repo.listAuditLogsLimit, maxPageSize, maxPageSize)
		}
	})
}

func TestHandler_ListLoginAttempts(t *testing.T) {
	t.Run("happy 200 with defaults", func(t *testing.T) {
		attemptID := uuid.New()
		at := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
		repo := &stubRepo{
			listLoginAttemptsFn: func(context.Context, auth.LoginAttemptFilter, int, int) ([]auth.LoginAttempt, int64, error) {
				return []auth.LoginAttempt{{
					ID:      attemptID,
					Email:   "teacher@example.com",
					Success: true,
					At:      at,
				}}, 1, nil
			},
		}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/login-attempts", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body listLoginAttemptsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if len(body.Attempts) != 1 || body.Attempts[0].ID != attemptID {
			t.Fatalf("attempts = %+v, want one attempt %s", body.Attempts, attemptID)
		}
		if body.Page != 1 || body.PageSize != defaultPageSize || body.Total != 1 || body.TotalPages != 1 {
			t.Fatalf("pagination = page %d size %d total %d pages %d", body.Page, body.PageSize, body.Total, body.TotalPages)
		}
		if repo.listLoginAttemptsLimit != defaultPageSize || repo.listLoginAttemptsOffset != 0 {
			t.Fatalf("limit/offset = %d/%d, want %d/0", repo.listLoginAttemptsLimit, repo.listLoginAttemptsOffset, defaultPageSize)
		}
	})

	t.Run("filter by email", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/login-attempts?email=%20TEACHER@example.com%20", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listLoginAttemptsFilter.Email != "TEACHER@example.com" {
			t.Fatalf("email filter = %q, want TEACHER@example.com", repo.listLoginAttemptsFilter.Email)
		}
	})

	t.Run("filter by success=true", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/login-attempts?success=true", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listLoginAttemptsFilter.Success == nil || *repo.listLoginAttemptsFilter.Success != true {
			t.Fatalf("success filter = %v, want true", repo.listLoginAttemptsFilter.Success)
		}
	})

	t.Run("filter by success=false", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/login-attempts?success=false", "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		if repo.listLoginAttemptsFilter.Success == nil || *repo.listLoginAttemptsFilter.Success != false {
			t.Fatalf("success filter = %v, want false", repo.listLoginAttemptsFilter.Success)
		}
	})

	t.Run("invalid_success -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/login-attempts?success=maybe", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_success")
		if repo.listLoginAttemptsCalls != 0 {
			t.Fatalf("ListLoginAttempts calls = %d, want 0", repo.listLoginAttemptsCalls)
		}
	})

	t.Run("invalid_time -> 400", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/admin/login-attempts?since=bad",
			"/api/v1/admin/login-attempts?until=bad",
		} {
			repo := &stubRepo{}
			app := testAdminApp(repo, uuid.New())

			resp := doAdminRequest(t, app, http.MethodGet, path, "")
			defer resp.Body.Close()

			assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_time")
			if repo.listLoginAttemptsCalls != 0 {
				t.Fatalf("ListLoginAttempts calls = %d, want 0", repo.listLoginAttemptsCalls)
			}
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

func TestChangeUserRole(t *testing.T) {
	t.Run("happy promote guru to admin -> 200", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(
			&auth.User{
				ID:           adminID,
				Name:         "Admin",
				Email:        "admin@example.com",
				PasswordHash: "hash",
				Role:         auth.Admin,
				Status:       auth.Active,
			},
			&auth.User{
				ID:     targetID,
				Name:   "Teacher",
				Email:  "teacher@example.com",
				Role:   auth.Guru,
				Status: auth.Active,
			},
		)
		var gotPlain string
		app := testAdminAppWithVerifier(repo, adminID, func(_, plain string) error {
			gotPlain = plain
			return nil
		})

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"admin","current_password":"secret"}`)
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
		if body.User.ID != targetID || body.User.Role != auth.Admin {
			t.Fatalf("user = %+v, want target role admin", body.User)
		}
		if gotPlain != "secret" {
			t.Fatalf("plain password = %q, want secret", gotPlain)
		}
		if repo.updateUserRoleCalls != 1 || repo.updateUserRoleArgs.id != targetID || repo.updateUserRoleArgs.role != auth.Admin {
			t.Fatalf("UpdateUserRole calls/id/role = %d/%s/%s", repo.updateUserRoleCalls, repo.updateUserRoleArgs.id, repo.updateUserRoleArgs.role)
		}
		if repo.revokeCalls != 1 || repo.revokeUserID != targetID || repo.revokeReason != auth.AdminReset {
			t.Fatalf("revoke = calls %d user %s reason %s", repo.revokeCalls, repo.revokeUserID, repo.revokeReason)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_role_changed" {
			t.Fatalf("audit = %+v", repo.audits)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["old_role"] != string(auth.Guru) || meta["new_role"] != string(auth.Admin) {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("happy demote admin to guru on non-self target -> 200", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(
			&auth.User{
				ID:           adminID,
				Name:         "Admin",
				Email:        "admin@example.com",
				PasswordHash: "hash",
				Role:         auth.Admin,
				Status:       auth.Active,
			},
			&auth.User{
				ID:     targetID,
				Name:   "Other Admin",
				Email:  "other-admin@example.com",
				Role:   auth.Admin,
				Status: auth.Active,
			},
		)
		repo.countAdminsFn = func(context.Context) (int64, error) { return 2, nil }
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"guru","current_password":"secret"}`)
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
		if body.User.Role != auth.Guru {
			t.Fatalf("user.role = %s, want %s", body.User.Role, auth.Guru)
		}
		if repo.updateUserRoleCalls != 1 || repo.updateUserRoleArgs.id != targetID || repo.updateUserRoleArgs.role != auth.Guru {
			t.Fatalf("UpdateUserRole calls/id/role = %d/%s/%s", repo.updateUserRoleCalls, repo.updateUserRoleArgs.id, repo.updateUserRoleArgs.role)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminAppWithVerifier(repo, uuid.New(), func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/role", `{"new_role":"admin","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("invalid_body malformed json -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminAppWithVerifier(repo, uuid.New(), func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/role", `{"new_role":`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_body")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("invalid_role -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminAppWithVerifier(repo, uuid.New(), func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/role", `{"new_role":"hacker","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_role")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("missing current_password -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminAppWithVerifier(repo, uuid.New(), func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/role", `{"new_role":"admin"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_body")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("invalid_current_password -> 401", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(
			&auth.User{
				ID:           adminID,
				PasswordHash: "hash",
				Role:         auth.Admin,
				Status:       auth.Active,
			},
			&auth.User{
				ID:     targetID,
				Role:   auth.Guru,
				Status: auth.Active,
			},
		)
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error {
			return bcrypt.ErrMismatchedHashAndPassword
		})

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"admin","current_password":"wrong"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusUnauthorized, "invalid_current_password")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
		}
	})

	t.Run("requester not found -> 401", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"admin","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusUnauthorized, "invalid_current_password")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
		}
	})

	t.Run("target not found -> 404", func(t *testing.T) {
		adminID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:           adminID,
			PasswordHash: "hash",
			Role:         auth.Admin,
			Status:       auth.Active,
		})
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/role", `{"new_role":"admin","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
		}
	})

	t.Run("same_role -> 400", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(
			&auth.User{
				ID:           adminID,
				PasswordHash: "hash",
				Role:         auth.Admin,
				Status:       auth.Active,
			},
			&auth.User{
				ID:     targetID,
				Role:   auth.Guru,
				Status: auth.Active,
			},
		)
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"guru","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "same_role")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
		}
	})

	t.Run("last_admin_protected -> 400", func(t *testing.T) {
		adminID := uuid.New()
		targetID := uuid.New()
		repo := repoWithUsers(
			&auth.User{
				ID:           adminID,
				PasswordHash: "hash",
				Role:         auth.Admin,
				Status:       auth.Active,
			},
			&auth.User{
				ID:     targetID,
				Role:   auth.Admin,
				Status: auth.Active,
			},
		)
		repo.countAdminsFn = func(context.Context) (int64, error) { return 1, nil }
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/role", `{"new_role":"guru","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "last_admin_protected")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
		}
	})

	t.Run("cannot_demote_self -> 400", func(t *testing.T) {
		adminID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:           adminID,
			PasswordHash: "hash",
			Role:         auth.Admin,
			Status:       auth.Active,
		})
		repo.countAdminsFn = func(context.Context) (int64, error) { return 2, nil }
		app := testAdminAppWithVerifier(repo, adminID, func(_, _ string) error { return nil })

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+adminID.String()+"/role", `{"new_role":"guru","current_password":"secret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "cannot_demote_self")
		if repo.updateUserRoleCalls != 0 {
			t.Fatalf("UpdateUserRole calls = %d, want 0", repo.updateUserRoleCalls)
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

func TestHandler_ResetUserPassword(t *testing.T) {
	t.Run("happy manual -> 200 user + no generated_password", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:                 targetID,
			Name:               "Teacher",
			Email:              "teacher@example.com",
			Role:               auth.Guru,
			Status:             auth.Active,
			MustChangePassword: false,
			FailedLoginCount:   3,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/reset-password", `{"password_strategy":"manual","password":"newsecret"}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body resetUserPasswordResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.GeneratedPassword != nil {
			t.Fatalf("generated_password = %q, want nil", *body.GeneratedPassword)
		}
		if body.User == nil || body.User.ID != targetID || !body.User.MustChangePassword {
			t.Fatalf("user = %+v", body.User)
		}
		if repo.adminResetPasswordCalls != 1 || repo.adminResetPasswordID != targetID {
			t.Fatalf("AdminResetUserPassword calls/id = %d/%s", repo.adminResetPasswordCalls, repo.adminResetPasswordID)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(repo.adminResetPasswordHash), []byte("newsecret")); err != nil {
			t.Fatalf("password hash did not match manual password: %v", err)
		}
		if repo.revokeCalls != 1 || repo.revokeUserID != targetID || repo.revokeReason != auth.AdminReset {
			t.Fatalf("revoke = calls %d user %s reason %s", repo.revokeCalls, repo.revokeUserID, repo.revokeReason)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_password_reset" {
			t.Fatalf("audit = %+v", repo.audits)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["password_strategy"] != "manual" || meta["must_change"] != true {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("happy generate -> 200 user + generated_password len 16", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Student",
			Email:  "student@example.com",
			Role:   auth.Siswa,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/reset-password", `{"password_strategy":"generate"}`)
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body resetUserPasswordResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.GeneratedPassword == nil || len(*body.GeneratedPassword) != generatedLength {
			t.Fatalf("generated_password len = %v, want %d", body.GeneratedPassword, generatedLength)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(repo.adminResetPasswordHash), []byte(*body.GeneratedPassword)); err != nil {
			t.Fatalf("password hash did not match generated password: %v", err)
		}
		if repo.adminResetPasswordCalls != 1 || repo.revokeCalls != 1 {
			t.Fatalf("reset/revoke calls = %d/%d, want 1/1", repo.adminResetPasswordCalls, repo.revokeCalls)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/reset-password", `{"password_strategy":"manual","password":"newsecret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
		if repo.adminResetPasswordCalls != 0 {
			t.Fatalf("AdminResetUserPassword calls = %d, want 0", repo.adminResetPasswordCalls)
		}
	})

	t.Run("invalid_strategy -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/reset-password", `{"password_strategy":"bad"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_strategy")
		if repo.findIDCalls != 0 {
			t.Fatalf("FindUserByID calls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("weak_password -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/reset-password", `{"password_strategy":"manual","password":"short"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "weak_password")
	})

	t.Run("conflicting_password -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/reset-password", `{"password_strategy":"generate","password":"newsecret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "conflicting_password")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/reset-password", `{"password_strategy":"manual","password":"newsecret"}`)
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
		if repo.adminResetPasswordCalls != 0 {
			t.Fatalf("AdminResetUserPassword calls = %d, want 0", repo.adminResetPasswordCalls)
		}
	})
}

func TestHandler_SuspendUser(t *testing.T) {
	t.Run("happy -> 200 user + previous_status in audit meta", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/suspend", `{"reason":"  policy  "}`)
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
		if body.User.Status != auth.Suspended {
			t.Fatalf("user.status = %s, want %s", body.User.Status, auth.Suspended)
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
		meta := auditMetaMap(t, repo.audits[0])
		if meta["previous_status"] != string(auth.Active) || meta["reason"] != "policy" {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/suspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/suspend", "")
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

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/suspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "last_admin_protected")
		if repo.suspendCalls != 0 {
			t.Fatalf("SuspendUser calls = %d, want 0", repo.suspendCalls)
		}
	})

	t.Run("cannot_suspend_self -> 400", func(t *testing.T) {
		adminID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     adminID,
			Name:   "Admin",
			Email:  "admin@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, adminID)

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+adminID.String()+"/suspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "cannot_suspend_self")
		if repo.suspendCalls != 0 {
			t.Fatalf("SuspendUser calls = %d, want 0", repo.suspendCalls)
		}
	})

	t.Run("already_suspended -> 400", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Suspended,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/suspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "already_suspended")
		if repo.suspendCalls != 0 {
			t.Fatalf("SuspendUser calls = %d, want 0", repo.suspendCalls)
		}
	})
}

func TestHandler_UnsuspendUser(t *testing.T) {
	t.Run("happy from suspended -> 200 user", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Suspended,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/unsuspend", "")
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
		if body.User.Status != auth.Active {
			t.Fatalf("user.status = %s, want %s", body.User.Status, auth.Active)
		}
		if repo.unsuspendCalls != 1 || repo.unsuspendID != targetID {
			t.Fatalf("UnsuspendUser calls/id = %d/%s", repo.unsuspendCalls, repo.unsuspendID)
		}
		if repo.revokeCalls != 0 {
			t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeCalls)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_unsuspended" {
			t.Fatalf("audit = %+v", repo.audits)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["previous_status"] != string(auth.Suspended) {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/unsuspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/unsuspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
	})

	t.Run("not_suspended status=active -> 400", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/unsuspend", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "not_suspended")
		if repo.unsuspendCalls != 0 {
			t.Fatalf("UnsuspendUser calls = %d, want 0", repo.unsuspendCalls)
		}
	})
}

func TestHandler_UnlockUser(t *testing.T) {
	t.Run("happy from locked -> 200 user", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:               targetID,
			Name:             "Teacher",
			Email:            "teacher@example.com",
			Role:             auth.Guru,
			Status:           auth.Locked,
			FailedLoginCount: 5,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/unlock", "")
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
		if body.User.Status != auth.Active {
			t.Fatalf("user.status = %s, want %s", body.User.Status, auth.Active)
		}
		if repo.unlockCalls != 1 || repo.unlockID != targetID {
			t.Fatalf("UnlockUser calls/id = %d/%s", repo.unlockCalls, repo.unlockID)
		}
		if repo.revokeCalls != 0 {
			t.Fatalf("RevokeAllRefreshByUser calls = %d, want 0", repo.revokeCalls)
		}
		if repo.auditCalls != 1 || repo.audits[0].Action != "admin_user_unlocked" {
			t.Fatalf("audit = %+v", repo.audits)
		}
		meta := auditMetaMap(t, repo.audits[0])
		if meta["previous_status"] != string(auth.Locked) {
			t.Fatalf("audit meta = %+v", meta)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/not-a-uuid/unlock", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+uuid.NewString()+"/unlock", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
	})

	t.Run("not_locked status=active -> 400", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodPost, "/api/v1/admin/users/"+targetID.String()+"/unlock", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "not_locked")
		if repo.unlockCalls != 0 {
			t.Fatalf("UnlockUser calls = %d, want 0", repo.unlockCalls)
		}
	})
}

func TestHandler_GetUser(t *testing.T) {
	t.Run("happy 200 returns user", func(t *testing.T) {
		targetID := uuid.New()
		repo := repoWithUsers(&auth.User{
			ID:     targetID,
			Name:   "Teacher",
			Email:  "teacher@example.com",
			Role:   auth.Guru,
			Status: auth.Active,
		})
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/"+targetID.String(), "")
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
		}
		var body struct {
			User *auth.User `json:"user"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.User == nil || body.User.ID != targetID || body.User.Email != "teacher@example.com" {
			t.Fatalf("user = %+v, want id=%s email=teacher@example.com", body.User, targetID)
		}
		if repo.findIDCalls != 1 || repo.findID != targetID {
			t.Fatalf("FindUserByID calls/id = %d/%s, want 1/%s", repo.findIDCalls, repo.findID, targetID)
		}
	})

	t.Run("invalid_id -> 400", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/not-a-uuid", "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
		if repo.findIDCalls != 0 {
			t.Fatalf("findIDCalls = %d, want 0", repo.findIDCalls)
		}
	})

	t.Run("user_not_found -> 404", func(t *testing.T) {
		repo := &stubRepo{}
		app := testAdminApp(repo, uuid.New())

		resp := doAdminRequest(t, app, http.MethodGet, "/api/v1/admin/users/"+uuid.NewString(), "")
		defer resp.Body.Close()

		assertErrorCode(t, resp, fiber.StatusNotFound, "user_not_found")
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
	repo.updateUserRoleFn = func(_ context.Context, id uuid.UUID, role auth.UserRole) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.Role = role
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
	repo.adminResetPasswordFn = func(_ context.Context, id uuid.UUID, newHash string) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.PasswordHash = newHash
		user.MustChangePassword = true
		user.FailedLoginCount = 0
		return nil
	}
	repo.unsuspendFn = func(_ context.Context, id uuid.UUID) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.Status = auth.Active
		return nil
	}
	repo.unlockFn = func(_ context.Context, id uuid.UUID) error {
		user, ok := store[id]
		if !ok {
			return gorm.ErrRecordNotFound
		}
		user.Status = auth.Active
		user.FailedLoginCount = 0
		return nil
	}
	return repo
}

func testAdminApp(repo *stubRepo, adminID uuid.UUID) *fiber.App {
	return testAdminAppWithVerifier(repo, adminID, nil)
}

func testAdminAppWithVerifier(repo *stubRepo, adminID uuid.UUID, verifier passwordVerifier) *fiber.App {
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
	if verifier != nil {
		h.verifyPassword = verifier
	}
	app.Get("/api/v1/admin/users", h.ListUsers)
	app.Post("/api/v1/admin/users", h.CreateUser)
	app.Get("/api/v1/admin/users/:id", h.GetUser)
	app.Patch("/api/v1/admin/users/:id", h.UpdateUser)
	app.Delete("/api/v1/admin/users/:id", h.DeleteUser)
	app.Post("/api/v1/admin/users/:id/reset-password", h.ResetUserPassword)
	app.Post("/api/v1/admin/users/:id/suspend", h.SuspendUser)
	app.Post("/api/v1/admin/users/:id/unsuspend", h.UnsuspendUser)
	app.Post("/api/v1/admin/users/:id/unlock", h.UnlockUser)
	app.Post("/api/v1/admin/users/:id/role", h.ChangeUserRole)
	app.Get("/api/v1/admin/users/:id/sessions", h.ListTargetSessions)
	app.Post("/api/v1/admin/users/:id/revoke-sessions", h.RevokeTargetSessions)
	app.Get("/api/v1/admin/audit-log", h.ListAuditLog)
	app.Get("/api/v1/admin/login-attempts", h.ListLoginAttempts)
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

func auditMetaMap(t *testing.T, entry *auth.AuditLog) map[string]any {
	t.Helper()

	var meta map[string]any
	if err := json.Unmarshal(entry.Meta, &meta); err != nil {
		t.Fatalf("Unmarshal audit meta error = %v", err)
	}
	return meta
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
