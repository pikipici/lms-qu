package kelas

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
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

type stubSvc struct {
	createFn      func(ctx context.Context, guruID uuid.UUID, in CreateInput, ip, ua string) (*Kelas, error)
	listFn        func(ctx context.Context, guruID uuid.UUID, in ListInput) (*ListResult, error)
	listAllFn     func(ctx context.Context, in ListInput) (*ListResult, error)
	getFn         func(ctx context.Context, id, viewerID uuid.UUID, role string) (*Kelas, error)
	updateFn      func(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Kelas, error)
	archiveFn     func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Kelas, error)
	duplicateFn   func(ctx context.Context, id, callerID uuid.UUID, role string, in DuplicateInput, ip, ua string) (*Kelas, error)
	joinFn        func(ctx context.Context, siswaID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error)
	listMyKelasFn func(ctx context.Context, siswaID uuid.UUID, in ListInput) (*MyKelasResult, error)
}

func (s *stubSvc) Create(ctx context.Context, guruID uuid.UUID, in CreateInput, ip, ua string) (*Kelas, error) {
	return s.createFn(ctx, guruID, in, ip, ua)
}

func (s *stubSvc) ListForGuru(ctx context.Context, guruID uuid.UUID, in ListInput) (*ListResult, error) {
	return s.listFn(ctx, guruID, in)
}

func (s *stubSvc) ListAllAdmin(ctx context.Context, in ListInput) (*ListResult, error) {
	return s.listAllFn(ctx, in)
}

func (s *stubSvc) Get(ctx context.Context, id, viewerID uuid.UUID, role string) (*Kelas, error) {
	return s.getFn(ctx, id, viewerID, role)
}

func (s *stubSvc) Update(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Kelas, error) {
	return s.updateFn(ctx, id, callerID, role, in, ip, ua)
}

func (s *stubSvc) Archive(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Kelas, error) {
	return s.archiveFn(ctx, id, callerID, role, ip, ua)
}

func (s *stubSvc) Duplicate(ctx context.Context, id, callerID uuid.UUID, role string, in DuplicateInput, ip, ua string) (*Kelas, error) {
	return s.duplicateFn(ctx, id, callerID, role, in, ip, ua)
}

func (s *stubSvc) JoinByKode(ctx context.Context, siswaID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
	return s.joinFn(ctx, siswaID, in, ip, ua)
}

func (s *stubSvc) ListMyKelas(ctx context.Context, siswaID uuid.UUID, in ListInput) (*MyKelasResult, error) {
	if s.listMyKelasFn == nil {
		return &MyKelasResult{Items: []MyKelasItem{}, Total: 0}, nil
	}
	return s.listMyKelasFn(ctx, siswaID, in)
}

// newApp builds a Fiber app with locals injected (mimicking BearerAuth output).
func newApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Get("/kelas", h.List)
	app.Post("/kelas", h.Create)
	app.Get("/kelas/:id", h.Get)
	app.Patch("/kelas/:id", h.Update)
	app.Post("/kelas/:id/archive", h.Archive)
	app.Post("/kelas/:id/duplicate", h.Duplicate)
	app.Post("/siswa/kelas/join", h.JoinByKode)
	app.Get("/siswa/kelas", h.ListMyKelas)
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
	id := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, gID uuid.UUID, in CreateInput, ip, ua string) (*Kelas, error) {
			if gID != guruID {
				t.Fatalf("guruID mismatch: %s != %s", gID, guruID)
			}
			if in.Nama != "Mat 7A" {
				t.Fatalf("nama mismatch: %q", in.Nama)
			}
			return &Kelas{ID: id, Nama: in.Nama, KodeInvite: "ABC234", GuruID: gID, Version: 1}, nil
		},
	}
	app := newApp(t, NewHandler(nil), string(auth.Guru), guruID) // svc nil placeholder
	app = newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)

	resp, body := doReq(t, app, "POST", "/kelas", map[string]any{
		"nama":               "Mat 7A",
		"bobot_soal_ulangan": 50,
		"bobot_tugas":        50,
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "ABC234") {
		t.Fatalf("missing kode invite in response: %s", body)
	}
}

func TestHandler_Create_PassesValidationErrToBody(t *testing.T) {
	guruID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, gID uuid.UUID, in CreateInput, ip, ua string) (*Kelas, error) {
			return nil, ErrBobotInvalid
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)

	resp, body := doReq(t, app, "POST", "/kelas", map[string]any{
		"nama":               "X",
		"bobot_soal_ulangan": 70,
		"bobot_tugas":        40,
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_bobot") {
		t.Fatalf("missing invalid_bobot code: %s", body)
	}
}

func TestHandler_Create_BadJSON(t *testing.T) {
	guruID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	req := httptest.NewRequest("POST", "/kelas", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, -1)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHandler_List_GuruOnlySeesOwn(t *testing.T) {
	guruID := uuid.New()
	called := false
	svc := &stubSvc{
		listFn: func(ctx context.Context, gID uuid.UUID, in ListInput) (*ListResult, error) {
			called = true
			if gID != guruID {
				t.Fatalf("guruID leak: %s != %s", gID, guruID)
			}
			return &ListResult{Items: []Kelas{{ID: uuid.New(), Nama: "A", GuruID: gID}}, Total: 1}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/kelas?page=1&page_size=10", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !called {
		t.Fatal("ListForGuru not called")
	}
}

func TestHandler_List_AdminUsesListAll(t *testing.T) {
	adminID := uuid.New()
	called := false
	svc := &stubSvc{
		listAllFn: func(ctx context.Context, in ListInput) (*ListResult, error) {
			called = true
			return &ListResult{Items: []Kelas{}, Total: 0}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Admin), adminID)
	resp, _ := doReq(t, app, "GET", "/kelas", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !called {
		t.Fatal("ListAllAdmin not called")
	}
}

func TestHandler_List_ForbiddenForSiswa(t *testing.T) {
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Siswa), uuid.New())
	resp, body := doReq(t, app, "GET", "/kelas", nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Update_VersionConflictReturns409(t *testing.T) {
	id := uuid.New()
	guruID := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Kelas, error) {
			return nil, ErrVersionConflict
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "PATCH", "/kelas/"+id.String(), map[string]any{
		"version":            1,
		"nama":               "Y",
		"bobot_soal_ulangan": 50,
		"bobot_tugas":        50,
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "version_conflict") {
		t.Fatalf("missing version_conflict: %s", body)
	}
}

func TestHandler_Update_ForbiddenReturns403(t *testing.T) {
	id := uuid.New()
	guruID := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Kelas, error) {
			return nil, ErrForbidden
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "PATCH", "/kelas/"+id.String(), map[string]any{
		"version":            1,
		"nama":               "Y",
		"bobot_soal_ulangan": 50,
		"bobot_tugas":        50,
	})
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestHandler_Update_RejectsZeroVersion(t *testing.T) {
	id := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "PATCH", "/kelas/"+id.String(), map[string]any{
		"nama":               "Y",
		"bobot_soal_ulangan": 50,
		"bobot_tugas":        50,
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_version") {
		t.Fatalf("missing invalid_version: %s", body)
	}
}

func TestHandler_Archive_PassThrough(t *testing.T) {
	id := uuid.New()
	guruID := uuid.New()
	svc := &stubSvc{
		archiveFn: func(ctx context.Context, kID, cID uuid.UUID, role, ip, ua string) (*Kelas, error) {
			now := time.Now()
			return &Kelas{ID: kID, ArchivedAt: &now}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+id.String()+"/archive", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Archive_AlreadyArchived(t *testing.T) {
	id := uuid.New()
	svc := &stubSvc{
		archiveFn: func(ctx context.Context, kID, cID uuid.UUID, role, ip, ua string) (*Kelas, error) {
			return nil, ErrAlreadyArchived
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "POST", "/kelas/"+id.String()+"/archive", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "already_archived") {
		t.Fatalf("missing already_archived: %s", body)
	}
}

func TestHandler_Duplicate_HappyPath(t *testing.T) {
	id := uuid.New()
	guruID := uuid.New()
	svc := &stubSvc{
		duplicateFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in DuplicateInput, ip, ua string) (*Kelas, error) {
			if in.NewNama != "" {
				t.Fatalf("expected empty NewNama default, got %q", in.NewNama)
			}
			return &Kelas{ID: uuid.New(), Nama: "X (Salinan)", KodeInvite: "DEF456", GuruID: cID, Version: 1}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+id.String()+"/duplicate", nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Duplicate_AcceptsCustomNama(t *testing.T) {
	id := uuid.New()
	guruID := uuid.New()
	svc := &stubSvc{
		duplicateFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in DuplicateInput, ip, ua string) (*Kelas, error) {
			if in.NewNama != "IPA 8B" {
				t.Fatalf("got newNama %q", in.NewNama)
			}
			return &Kelas{ID: uuid.New(), Nama: in.NewNama, KodeInvite: "EFG567", GuruID: cID, Version: 1}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "POST", "/kelas/"+id.String()+"/duplicate", map[string]any{"new_nama": "IPA 8B"})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_Get_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &stubSvc{
		getFn: func(ctx context.Context, kID, vID uuid.UUID, role string) (*Kelas, error) {
			return nil, ErrNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "GET", "/kelas/"+id.String(), nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_Get_InvalidUUID(t *testing.T) {
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "GET", "/kelas/not-a-uuid", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "invalid_id") {
		t.Fatalf("missing invalid_id: %s", body)
	}
}

func TestHandler_FriendlyMessage_StripsLayeredPrefix(t *testing.T) {
	got := friendlyMessage(errors.New("kelas: invalid input: nama is required"), "fallback")
	if got != "nama is required" {
		t.Fatalf("got %q", got)
	}
}
