package materi

import (
	"bytes"
	"context"
	"encoding/json"
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
	createFn  func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error)
	listFn    func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Materi, error)
	getFn     func(ctx context.Context, id, callerID uuid.UUID, role string) (*Materi, error)
	updateFn  func(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Materi, error)
	deleteFn  func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Materi, *string, error)
	uploadFn  func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in UploadInput, ip, ua string) (*Materi, error)
	presignFn func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*FileURLResult, error)
}

func (s *stubSvc) Create(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error) {
	return s.createFn(ctx, kelasID, callerID, role, in, ip, ua)
}
func (s *stubSvc) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Materi, error) {
	return s.listFn(ctx, kelasID, callerID, role, in)
}
func (s *stubSvc) Get(ctx context.Context, id, callerID uuid.UUID, role string) (*Materi, error) {
	return s.getFn(ctx, id, callerID, role)
}
func (s *stubSvc) Update(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Materi, error) {
	return s.updateFn(ctx, id, callerID, role, in, ip, ua)
}
func (s *stubSvc) Delete(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Materi, *string, error) {
	return s.deleteFn(ctx, id, callerID, role, ip, ua)
}
func (s *stubSvc) Upload(ctx context.Context, kelasID, callerID uuid.UUID, role string, in UploadInput, ip, ua string) (*Materi, error) {
	return s.uploadFn(ctx, kelasID, callerID, role, in, ip, ua)
}
func (s *stubSvc) PresignFileURL(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*FileURLResult, error) {
	return s.presignFn(ctx, id, callerID, role, ip, ua)
}

func newApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Get("/kelas/:id/materi", h.ListByKelas)
	app.Post("/kelas/:id/materi", h.Create)
	app.Get("/materi/:id", h.Get)
	app.Patch("/materi/:id", h.Update)
	app.Delete("/materi/:id", h.Delete)
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

func TestHandler_Create_YouTube_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	materiID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error) {
			if kID != kelasID {
				t.Fatalf("kelasID mismatch")
			}
			if in.Tipe != TipeYouTube {
				t.Fatalf("tipe mismatch %q", in.Tipe)
			}
			// Service is responsible for parsing — handler passes raw konten.
			if in.Konten != "https://youtu.be/dQw4w9WgXcQ" {
				t.Fatalf("konten mismatch %q", in.Konten)
			}
			return &Materi{
				ID: materiID, KelasID: kID, Judul: in.Judul, Tipe: TipeYouTube,
				Konten: "dQw4w9WgXcQ", Urutan: 1, Version: 1,
			}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+kelasID.String()+"/materi", map[string]any{
		"judul":  "Video Pengantar",
		"tipe":   "youtube",
		"konten": "https://youtu.be/dQw4w9WgXcQ",
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	var out struct {
		Materi Materi `json:"materi"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if out.Materi.ID != materiID {
		t.Fatalf("ID mismatch")
	}
	if out.Materi.Konten != "dQw4w9WgXcQ" {
		t.Fatalf("Konten mismatch %q", out.Materi.Konten)
	}
}

func TestHandler_Create_PDF_RejectedAsTipeUnsupported(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error) {
			return nil, ErrTipeUnsupported
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+kelasID.String()+"/materi", map[string]any{
		"judul":  "x",
		"tipe":   "pdf",
		"konten": "",
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"tipe_unsupported"`) {
		t.Fatalf("expected code=tipe_unsupported, got %s", body)
	}
}

func TestHandler_Create_KelasArchived_409(t *testing.T) {
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error) {
			return nil, ErrKelasArchived
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "POST", "/kelas/"+uuid.NewString()+"/materi", map[string]any{
		"judul": "x", "tipe": "markdown", "konten": "hi",
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"kelas_archived"`) {
		t.Fatalf("expected code=kelas_archived, got %s", body)
	}
}

func TestHandler_Create_BabNotInKelas_400(t *testing.T) {
	svc := &stubSvc{
		createFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in CreateInput, ip, ua string) (*Materi, error) {
			return nil, ErrBabNotInKelas
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	bid := uuid.New()
	resp, body := doReq(t, app, "POST", "/kelas/"+uuid.NewString()+"/materi", map[string]any{
		"bab_id": bid.String(), "judul": "x", "tipe": "markdown", "konten": "hi",
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"bab_not_in_kelas"`) {
		t.Fatalf("expected code=bab_not_in_kelas, got %s", body)
	}
}

func TestHandler_List_BabIDNullQuery(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	called := false
	svc := &stubSvc{
		listFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in ListInput) ([]Materi, error) {
			called = true
			if in.BabID == nil {
				t.Fatalf("expected BabID non-nil for ?bab_id=null")
			}
			if *in.BabID != uuid.Nil {
				t.Fatalf("expected zero UUID for null filter, got %v", *in.BabID)
			}
			return []Materi{}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/materi?bab_id=null", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !called {
		t.Fatalf("svc.ListByKelas not invoked")
	}
}

func TestHandler_List_BabIDInvalidQuery_400(t *testing.T) {
	svc := &stubSvc{
		listFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in ListInput) ([]Materi, error) {
			t.Fatalf("svc should not be called on invalid bab_id")
			return nil, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, _ := doReq(t, app, "GET", "/kelas/"+uuid.NewString()+"/materi?bab_id=not-a-uuid", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHandler_List_BabIDEqQuery(t *testing.T) {
	bid := uuid.New()
	called := false
	svc := &stubSvc{
		listFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in ListInput) ([]Materi, error) {
			called = true
			if in.BabID == nil || *in.BabID != bid {
				t.Fatalf("BabID mismatch: got %v want %v", in.BabID, bid)
			}
			return []Materi{}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, _ := doReq(t, app, "GET", "/kelas/"+uuid.NewString()+"/materi?bab_id="+bid.String(), nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !called {
		t.Fatalf("svc not called")
	}
}

func TestHandler_Update_VersionConflict_409(t *testing.T) {
	svc := &stubSvc{
		updateFn: func(ctx context.Context, id, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Materi, error) {
			return nil, ErrVersionConflict
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "PATCH", "/materi/"+uuid.NewString(), map[string]any{
		"version": 1, "judul": "new",
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"version_conflict"`) {
		t.Fatalf("expected code=version_conflict, got %s", body)
	}
}

func TestHandler_Update_VersionMissing_400(t *testing.T) {
	svc := &stubSvc{}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "PATCH", "/materi/"+uuid.NewString(), map[string]any{
		"judul": "new", // no version
	})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"invalid_version"`) {
		t.Fatalf("expected code=invalid_version, got %s", body)
	}
}

func TestHandler_Update_TipeImmutable_409(t *testing.T) {
	svc := &stubSvc{
		updateFn: func(ctx context.Context, id, cID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Materi, error) {
			return nil, ErrTipeImmutable
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "PATCH", "/materi/"+uuid.NewString(), map[string]any{
		"version": 1, "konten": "new content",
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"tipe_immutable"`) {
		t.Fatalf("expected code=tipe_immutable, got %s", body)
	}
}

func TestHandler_Get_NotFound_404(t *testing.T) {
	svc := &stubSvc{
		getFn: func(ctx context.Context, id, cID uuid.UUID, role string) (*Materi, error) {
			return nil, ErrNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, _ := doReq(t, app, "GET", "/materi/"+uuid.NewString(), nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHandler_Get_Forbidden_403(t *testing.T) {
	svc := &stubSvc{
		getFn: func(ctx context.Context, id, cID uuid.UUID, role string) (*Materi, error) {
			return nil, ErrForbidden
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, _ := doReq(t, app, "GET", "/materi/"+uuid.NewString(), nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestHandler_Delete_HappyPath_NoObjectKey(t *testing.T) {
	mid := uuid.New()
	svc := &stubSvc{
		deleteFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*Materi, *string, error) {
			if id != mid {
				t.Fatalf("id mismatch")
			}
			return &Materi{ID: id, Tipe: TipeMarkdown}, nil, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "DELETE", "/materi/"+mid.String(), nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "object_key") {
		t.Fatalf("expected no object_key for markdown, got %s", body)
	}
}

func TestHandler_Delete_PDF_ReturnsObjectKey(t *testing.T) {
	mid := uuid.New()
	objectKey := "materi/abc123.pdf"
	svc := &stubSvc{
		deleteFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*Materi, *string, error) {
			return &Materi{ID: id, Tipe: TipePDF}, &objectKey, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, "DELETE", "/materi/"+mid.String(), nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"object_key":"materi/abc123.pdf"`) {
		t.Fatalf("expected object_key in body, got %s", body)
	}
	if !strings.Contains(string(body), `"pending_r2_cleanup":true`) {
		t.Fatalf("expected pending_r2_cleanup flag, got %s", body)
	}
}

func TestHandler_InvalidUUID_400(t *testing.T) {
	svc := &stubSvc{}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, _ := doReq(t, app, "GET", "/materi/not-a-uuid", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
