package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

type fakeAuditService struct {
	res  *ListResponse
	err  error
	seen struct {
		kelasID  uuid.UUID
		callerID uuid.UUID
		role     string
		action   string
		limit    int
		offset   int
	}
}

func (f *fakeAuditService) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole, action string, limit, offset int) (*ListResponse, error) {
	f.seen.kelasID = kelasID
	f.seen.callerID = callerID
	f.seen.role = callerRole
	f.seen.action = action
	f.seen.limit = limit
	f.seen.offset = offset
	return f.res, f.err
}

func newAuditTestApp(svc auditService, userID uuid.UUID, role string) *fiber.App {
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Use(func(c *fiber.Ctx) error {
		if userID != uuid.Nil {
			c.Locals(middleware.LocalsUserID, userID)
		}
		if role != "" {
			c.Locals(middleware.LocalsUserRole, role)
		}
		return c.Next()
	})
	app.Get("/kelas/:id/audit", (&Handler{svc: svc}).ListByKelas)
	app.Get("/audit-actions", (&Handler{svc: svc}).ListActions)
	return app
}

func TestHandlerListByKelasSuccess(t *testing.T) {
	kelasID := uuid.New()
	callerID := uuid.New()
	svc := &fakeAuditService{res: &ListResponse{Events: []Entry{{ID: uuid.New(), Action: "kelas_created"}}, Total: 1, Limit: 7, Offset: 2}}
	app := newAuditTestApp(svc, callerID, string(auth.Guru))

	resp, err := app.Test(httptest.NewRequest("GET", "/kelas/"+kelasID.String()+"/audit?action=kelas_created&limit=7&offset=2", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	if svc.seen.kelasID != kelasID || svc.seen.callerID != callerID || svc.seen.role != string(auth.Guru) || svc.seen.action != "kelas_created" || svc.seen.limit != 7 || svc.seen.offset != 2 {
		t.Fatalf("service args = %+v", svc.seen)
	}

	var body ListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Total != 1 || len(body.Events) != 1 || body.Events[0].Action != "kelas_created" {
		t.Fatalf("body = %+v", body)
	}
}

func TestHandlerListByKelasRequestValidation(t *testing.T) {
	validID := uuid.New().String()
	tests := []struct {
		name   string
		path   string
		status int
		code   string
		userID uuid.UUID
	}{
		{"bad id", "/kelas/not-uuid/audit", fiber.StatusBadRequest, "invalid_id", uuid.New()},
		{"missing user", "/kelas/" + validID + "/audit", fiber.StatusUnauthorized, "unauthorized", uuid.Nil},
		{"bad limit", "/kelas/" + validID + "/audit?limit=0", fiber.StatusBadRequest, "invalid_limit", uuid.New()},
		{"bad offset", "/kelas/" + validID + "/audit?offset=-1", fiber.StatusBadRequest, "invalid_offset", uuid.New()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAuditTestApp(&fakeAuditService{}, tt.userID, string(auth.Guru))
			resp, err := app.Test(httptest.NewRequest("GET", tt.path, nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["code"] != tt.code || body["request_id"] == "" {
				t.Fatalf("error body = %+v", body)
			}
		})
	}
}

func TestHandlerListByKelasMapsServiceErrors(t *testing.T) {
	kelasID := uuid.New().String()
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"forbidden", ErrForbidden, fiber.StatusForbidden, "forbidden"},
		{"not found", ErrNotFound, fiber.StatusNotFound, "kelas_not_found"},
		{"invalid action", ErrInvalidAction, fiber.StatusBadRequest, "invalid_action"},
		{"invalid paginate", ErrInvalidPaginate, fiber.StatusBadRequest, "invalid_offset"},
		{"internal", errors.New("repo down"), fiber.StatusInternalServerError, "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newAuditTestApp(&fakeAuditService{err: tt.err}, uuid.New(), string(auth.Guru))
			resp, err := app.Test(httptest.NewRequest("GET", "/kelas/"+kelasID+"/audit", nil))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.status)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["code"] != tt.code || body["request_id"] == "" {
				t.Fatalf("error body = %+v", body)
			}
		})
	}
}

func TestHandlerListActionsRoleGuard(t *testing.T) {
	app := newAuditTestApp(&fakeAuditService{}, uuid.New(), string(auth.Guru))
	resp, err := app.Test(httptest.NewRequest("GET", "/audit-actions", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	var body ActionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Actions) != len(AllowedActions) {
		t.Fatalf("actions len = %d, want %d", len(body.Actions), len(AllowedActions))
	}

	app = newAuditTestApp(&fakeAuditService{}, uuid.New(), string(auth.Siswa))
	resp, err = app.Test(httptest.NewRequest("GET", "/audit-actions", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusForbidden)
	}
}
