package bab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

type stubSvc struct {
	createFn  func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Bab, error)
	listFn    func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Bab, error)
	getFn     func(ctx context.Context, id, callerID uuid.UUID, role string) (*Bab, error)
	updateFn  func(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Bab, error)
	archiveFn func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Bab, error)
}

func (s *stubSvc) Create(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Bab, error) {
	return s.createFn(ctx, kelasID, callerID, role, in, ip, ua)
}

func (s *stubSvc) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Bab, error) {
	return s.listFn(ctx, kelasID, callerID, role, in)
}

func (s *stubSvc) Get(ctx context.Context, id, callerID uuid.UUID, role string) (*Bab, error) {
	return s.getFn(ctx, id, callerID, role)
}

func (s *stubSvc) Update(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Bab, error) {
	return s.updateFn(ctx, id, callerID, role, in, ip, ua)
}

func (s *stubSvc) Archive(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Bab, error) {
	return s.archiveFn(ctx, id, callerID, role, ip, ua)
}

func newApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Get("/kelas/:id/bab", h.ListByKelas)
	app.Post("/kelas/:id/bab", h.Create)
	app.Get("/bab/:id", h.Get)
	app.Patch("/bab/:id", h.Update)
	app.Post("/bab/:id/archive", h.Archive)
	return app
}

func doReq(t *testing.T, app *fiber.App, method, path string, body any) (*http.Response, []byte) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, respBody
}

func TestHandler_Create_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	babID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Bab, error) {
			if kID != kelasID {
				t.Fatalf("kelasID mismatch")
			}
			if cID != guruID {
				t.Fatalf("callerID mismatch")
			}
			if in.Judul != "Bab 1: Bilangan" {
				t.Fatalf("judul mismatch %q", in.Judul)
			}
			return &Bab{ID: babID, KelasID: kID, Nomor: in.Nomor, Judul: in.Judul, Status: StatusDraft, Version: 1, Urutan: 1}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+kelasID.String()+"/bab", map[string]any{
		"nomor":     1,
		"judul":     "Bab 1: Bilangan",
		"deskripsi": "Pengantar bilangan",
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), babID.String()) {
		t.Fatalf("missing bab id in response: %s", body)
	}
}

func TestHandler_Create_InvalidKelasID(t *testing.T) {
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "POST", "/kelas/not-a-uuid/bab", map[string]any{"judul": "x", "nomor": 1})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_id") {
		t.Fatalf("missing invalid_id code: %s", body)
	}
}

func TestHandler_Create_PassesValidationErr(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Bab, error) {
			return nil, ErrInvalidInput
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+uuid.NewString()+"/bab", map[string]any{"nomor": 1, "judul": ""})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_body") {
		t.Fatalf("missing invalid_body code: %s", body)
	}
}

func TestHandler_Create_KelasArchived(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Bab, error) {
			return nil, ErrKelasArchived
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+uuid.NewString()+"/bab", map[string]any{"nomor": 1, "judul": "x"})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "kelas_archived") {
		t.Fatalf("missing kelas_archived code: %s", body)
	}
}

func TestHandler_Create_BadJSON(t *testing.T) {
	guruID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	req := httptest.NewRequest("POST", "/kelas/"+uuid.NewString()+"/bab", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_List_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := &stubSvc{
		listFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in ListInput) ([]Bab, error) {
			if in.IncludeArchived {
				t.Fatalf("expected default include_archived=false")
			}
			return []Bab{
				{ID: uuid.New(), KelasID: kID, Nomor: 1, Judul: "Bab 1", Urutan: 1, Status: StatusPublished, Version: 1},
				{ID: uuid.New(), KelasID: kID, Nomor: 2, Judul: "Bab 2", Urutan: 2, Status: StatusDraft, Version: 1},
			}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/bab", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"total":2`) {
		t.Fatalf("missing total=2: %s", body)
	}
}

func TestHandler_List_StatusFilter(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := &stubSvc{
		listFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in ListInput) ([]Bab, error) {
			if in.Status == nil || *in.Status != StatusPublished {
				t.Fatalf("expected status filter published; got %+v", in.Status)
			}
			return []Bab{}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/bab?status=published", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_List_InvalidStatusFilter(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/bab?status=zzz", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_status") {
		t.Fatalf("missing invalid_status code: %s", body)
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		getFn: func(ctx context.Context, id, cID uuid.UUID, role string) (*Bab, error) {
			return nil, ErrNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/bab/"+uuid.NewString(), nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Get_Forbidden(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		getFn: func(ctx context.Context, id, cID uuid.UUID, role string) (*Bab, error) {
			return nil, ErrForbidden
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/bab/"+uuid.NewString(), nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Update_VersionConflict(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, id, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Bab, error) {
			return nil, ErrVersionConflict
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "PATCH", "/bab/"+uuid.NewString(), map[string]any{"version": 1, "judul": "X"})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "version_conflict") {
		t.Fatalf("missing version_conflict code: %s", body)
	}
}

func TestHandler_Update_RequiresVersion(t *testing.T) {
	guruID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "PATCH", "/bab/"+uuid.NewString(), map[string]any{"judul": "X"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_version") {
		t.Fatalf("missing invalid_version code: %s", body)
	}
}

func TestHandler_Update_StatusOnly(t *testing.T) {
	guruID := uuid.New()
	babID := uuid.New()
	wantStatus := StatusPublished
	svc := &stubSvc{
		updateFn: func(ctx context.Context, id, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Bab, error) {
			if in.Status == nil || *in.Status != wantStatus {
				t.Fatalf("status not propagated: %+v", in.Status)
			}
			return &Bab{ID: id, Status: *in.Status, Version: 2}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "PATCH", "/bab/"+babID.String(), map[string]any{"version": 1, "status": "published"})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Archive_AlreadyArchived(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		archiveFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*Bab, error) {
			return nil, ErrAlreadyArchived
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/bab/"+uuid.NewString()+"/archive", nil)
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "already_archived") {
		t.Fatalf("missing already_archived: %s", body)
	}
}

func TestHandler_Archive_HappyPath(t *testing.T) {
	guruID := uuid.New()
	babID := uuid.New()
	svc := &stubSvc{
		archiveFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*Bab, error) {
			return &Bab{ID: id, Status: StatusArchived, Version: 2}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/bab/"+babID.String()+"/archive", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"status":"archived"`) {
		t.Fatalf("missing archived status: %s", body)
	}
}

func TestHandler_Update_GenericInternalErr(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, id, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Bab, error) {
			return nil, errors.New("db blew up")
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "PATCH", "/bab/"+uuid.NewString(), map[string]any{"version": 1, "judul": "X"})
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
