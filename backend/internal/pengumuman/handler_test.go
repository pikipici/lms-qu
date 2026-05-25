package pengumuman

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
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
)

// ---------- Stubs untuk service deps ----------

type stubRepo struct {
	createFn func(ctx context.Context, p *Pengumuman) error
	findFn   func(ctx context.Context, id uuid.UUID) (*Pengumuman, error)
	listFn   func(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Pengumuman, error)
	updateFn func(ctx context.Context, id uuid.UUID, expectedVersion int, judul, isi string, status Status) error
	deleteFn func(ctx context.Context, id uuid.UUID) error
}

func (r *stubRepo) Create(ctx context.Context, p *Pengumuman) error {
	return r.createFn(ctx, p)
}
func (r *stubRepo) FindByID(ctx context.Context, id uuid.UUID) (*Pengumuman, error) {
	return r.findFn(ctx, id)
}
func (r *stubRepo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Pengumuman, error) {
	return r.listFn(ctx, kelasID, f)
}
func (r *stubRepo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, isi string, status Status) error {
	return r.updateFn(ctx, id, expectedVersion, judul, isi, status)
}
func (r *stubRepo) AddAttachment(ctx context.Context, a *Attachment) error { return nil }
func (r *stubRepo) CountAttachmentsByPengumuman(ctx context.Context, pengumumanID uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *stubRepo) FindAttachmentByID(ctx context.Context, pengumumanID, attachmentID uuid.UUID) (*Attachment, error) {
	return &Attachment{ID: attachmentID, PengumumanID: pengumumanID, ObjectKey: "pengumuman/test.pdf", OriginalFilename: "test.pdf", MimeType: "application/pdf", SizeBytes: 1}, nil
}
func (r *stubRepo) DeleteAttachment(ctx context.Context, pengumumanID, attachmentID uuid.UUID) (string, error) {
	return "pengumuman/test.pdf", nil
}
func (r *stubRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.deleteFn(ctx, id)
}

type stubKelas struct {
	findFn func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

func (k *stubKelas) FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
	return k.findFn(ctx, id)
}

type stubBab struct {
	findFn func(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
}

func (b *stubBab) FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
	return b.findFn(ctx, id)
}

type stubEnroll struct {
	findFn func(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

func (e *stubEnroll) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error) {
	return e.findFn(ctx, kelasID, siswaID)
}

type stubAudit struct {
	logged []*auth.AuditLog
}

func (a *stubAudit) LogAudit(ctx context.Context, entry *auth.AuditLog) error {
	a.logged = append(a.logged, entry)
	return nil
}

// activeKelas returns a non-archived kelas owned by guruID.
func activeKelas(id, guruID uuid.UUID) *kelas.Kelas {
	return &kelas.Kelas{ID: id, GuruID: guruID, Nama: "Kelas X"}
}

// archivedKelas returns an archived kelas.
func archivedKelas(id, guruID uuid.UUID) *kelas.Kelas {
	now := time.Now()
	return &kelas.Kelas{ID: id, GuruID: guruID, Nama: "Kelas X", ArchivedAt: &now}
}

// activeEnrollment returns a healthy enrollment row.
func activeEnrollment(kelasID, siswaID uuid.UUID) *kelas.Enrollment {
	return &kelas.Enrollment{KelasID: kelasID, SiswaID: siswaID, Status: kelas.EnrollmentActive}
}

// ---------- Service tests ----------

func TestService_Create_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	repo := &stubRepo{
		createFn: func(ctx context.Context, p *Pengumuman) error {
			if p.KelasID != kelasID {
				t.Fatalf("kelas_id mismatch")
			}
			if p.Status != StatusPublished {
				t.Fatalf("status default mismatch %q", p.Status)
			}
			if p.Version != 1 {
				t.Fatalf("version mismatch %d", p.Version)
			}
			p.ID = uuid.New()
			return nil
		},
	}
	k := &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}
	audit := &stubAudit{}
	svc := NewService(repo, k, &stubBab{}, nil, audit)

	p, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "  Pengumuman Penting  ", Isi: "Halo siswa"}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if p.Judul != "Pengumuman Penting" {
		t.Fatalf("judul not trimmed: %q", p.Judul)
	}
	if len(audit.logged) != 1 || audit.logged[0].Action != "pengumuman_created" {
		t.Fatalf("audit log mismatch: %+v", audit.logged)
	}
}

func TestService_Create_RejectEmptyJudul(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "   ", Isi: ""}, "", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_Create_RejectIsiTooLong(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	huge := strings.Repeat("x", MaxIsiBytes+1)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok", Isi: huge}, "", "")
	if !errors.Is(err, ErrIsiTooLong) {
		t.Fatalf("expected ErrIsiTooLong, got %v", err)
	}
}

func TestService_Create_KelasArchived(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return archivedKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok"}, "", "")
	if !errors.Is(err, ErrKelasArchived) {
		t.Fatalf("expected ErrKelasArchived, got %v", err)
	}
}

func TestService_Create_NotOwner(t *testing.T) {
	guruID := uuid.New()
	otherGuru := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, otherGuru), nil
	}}, &stubBab{}, nil, nil)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok"}, "", "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_Create_BabNotInKelas(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	otherKelasID := uuid.New()
	babID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{findFn: func(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
		return &bab.Bab{ID: id, KelasID: otherKelasID}, nil
	}}, nil, nil)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok", BabID: &babID}, "", "")
	if !errors.Is(err, ErrBabNotInKelas) {
		t.Fatalf("expected ErrBabNotInKelas, got %v", err)
	}
}

func TestService_List_Siswa_ForcePublishedOnly(t *testing.T) {
	siswaID := uuid.New()
	kelasID := uuid.New()
	calls := 0
	repo := &stubRepo{
		listFn: func(ctx context.Context, k uuid.UUID, f ListFilter) ([]Pengumuman, error) {
			calls++
			if f.Status == nil || *f.Status != StatusPublished {
				t.Fatalf("siswa list must be pinned to published, got %+v", f.Status)
			}
			return []Pengumuman{}, nil
		},
	}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, uuid.New()), nil
	}}, &stubBab{}, &stubEnroll{findFn: func(ctx context.Context, kID, sID uuid.UUID) (*kelas.Enrollment, error) {
		return activeEnrollment(kID, sID), nil
	}}, nil)

	// Even if siswa passes status=archived, service forces it back to published.
	st := StatusArchived
	_, err := svc.ListByKelas(context.Background(), kelasID, siswaID, string(auth.Siswa), ListInput{Status: &st})
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("repo not called")
	}
}

func TestService_List_Siswa_NotEnrolled(t *testing.T) {
	siswaID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, uuid.New()), nil
	}}, &stubBab{}, &stubEnroll{findFn: func(ctx context.Context, kID, sID uuid.UUID) (*kelas.Enrollment, error) {
		return nil, gorm.ErrRecordNotFound
	}}, nil)
	_, err := svc.ListByKelas(context.Background(), kelasID, siswaID, string(auth.Siswa), ListInput{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestService_List_Guru_FullVisibility(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	called := false
	repo := &stubRepo{
		listFn: func(ctx context.Context, k uuid.UUID, f ListFilter) ([]Pengumuman, error) {
			called = true
			if f.Status != nil {
				t.Fatalf("guru status must be nil (no filter), got %+v", *f.Status)
			}
			return []Pengumuman{{ID: uuid.New(), Status: StatusArchived}}, nil
		},
	}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	rows, err := svc.ListByKelas(context.Background(), kelasID, guruID, string(auth.Guru), ListInput{})
	if err != nil {
		t.Fatalf("List err: %v", err)
	}
	if !called {
		t.Fatalf("repo not called")
	}
	if len(rows) != 1 {
		t.Fatalf("rows mismatch: %d", len(rows))
	}
}

func TestService_Get_Siswa_ArchivedHidden(t *testing.T) {
	siswaID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Pengumuman, error) {
			return &Pengumuman{ID: id, KelasID: kelasID, Status: StatusArchived}, nil
		},
	}, &stubKelas{}, &stubBab{}, nil, nil)
	_, err := svc.Get(context.Background(), id, siswaID, string(auth.Siswa))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for archived siswa view, got %v", err)
	}
}

func TestService_Update_VersionConflict(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Pengumuman, error) {
			return &Pengumuman{ID: id, KelasID: kelasID, Version: 5, Judul: "old", Status: StatusPublished}, nil
		},
		updateFn: func(ctx context.Context, _ uuid.UUID, expVer int, _, _ string, _ Status) error {
			return ErrVersionConflict
		},
	}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	newJudul := "new"
	_, err := svc.Update(context.Background(), id, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion: 5, Judul: &newJudul,
	}, "", "")
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}

func TestService_Update_ArchiveAuditAction(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	updated := false
	repo := &stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Pengumuman, error) {
			if updated {
				return &Pengumuman{ID: id, KelasID: kelasID, Version: 6, Status: StatusArchived, Judul: "x"}, nil
			}
			return &Pengumuman{ID: id, KelasID: kelasID, Version: 5, Status: StatusPublished, Judul: "x"}, nil
		},
		updateFn: func(ctx context.Context, _ uuid.UUID, _ int, _, _ string, _ Status) error {
			updated = true
			return nil
		},
	}
	audit := &stubAudit{}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, audit)
	st := StatusArchived
	_, err := svc.Update(context.Background(), id, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion: 5, Status: &st,
	}, "", "")
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if len(audit.logged) != 1 || audit.logged[0].Action != "pengumuman_archived" {
		t.Fatalf("audit action mismatch: %+v", audit.logged)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Pengumuman, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}, &stubKelas{}, &stubBab{}, nil, nil)
	_, err := svc.Delete(context.Background(), id, guruID, string(auth.Guru), "", "")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	deleted := false
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Pengumuman, error) {
			return &Pengumuman{ID: id, KelasID: kelasID, Judul: "x", Status: StatusPublished}, nil
		},
		deleteFn: func(ctx context.Context, _ uuid.UUID) error {
			deleted = true
			return nil
		},
	}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	_, err := svc.Delete(context.Background(), id, guruID, string(auth.Guru), "", "")
	if err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if !deleted {
		t.Fatalf("repo Delete not called")
	}
}

// ---------- Handler tests (smoke) ----------

type stubSvc struct {
	createFn func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Pengumuman, error)
	listFn   func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Pengumuman, error)
	getFn    func(ctx context.Context, id, callerID uuid.UUID, role string) (*Pengumuman, error)
	updateFn func(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Pengumuman, error)
	deleteFn func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Pengumuman, error)
}

func (s *stubSvc) Create(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Pengumuman, error) {
	return s.createFn(ctx, kelasID, callerID, role, in, ip, ua)
}
func (s *stubSvc) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Pengumuman, error) {
	return s.listFn(ctx, kelasID, callerID, role, in)
}
func (s *stubSvc) Get(ctx context.Context, id, callerID uuid.UUID, role string) (*Pengumuman, error) {
	return s.getFn(ctx, id, callerID, role)
}
func (s *stubSvc) Update(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Pengumuman, error) {
	return s.updateFn(ctx, id, callerID, role, in, ip, ua)
}
func (s *stubSvc) UploadAttachment(ctx context.Context, id, callerID uuid.UUID, role string, in AttachmentUploadInput, ip, ua string) (*Pengumuman, error) {
	return s.getFn(ctx, id, callerID, role)
}
func (s *stubSvc) DeleteAttachment(ctx context.Context, id, attachmentID, callerID uuid.UUID, role string, ip, ua string) (*Pengumuman, error) {
	return s.getFn(ctx, id, callerID, role)
}
func (s *stubSvc) PresignAttachmentURL(ctx context.Context, id, attachmentID, callerID uuid.UUID, role string) (*AttachmentURLResult, error) {
	return &AttachmentURLResult{URL: "http://example.test/file", ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (s *stubSvc) Delete(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Pengumuman, error) {
	return s.deleteFn(ctx, id, callerID, role, ip, ua)
}

func newApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Post("/kelas/:id/pengumuman", h.Create)
	app.Get("/kelas/:id/pengumuman", h.ListByKelas)
	app.Get("/pengumuman/:id", h.Get)
	app.Patch("/pengumuman/:id", h.Update)
	app.Delete("/pengumuman/:id", h.Delete)
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
	id := uuid.New()
	svc := &stubSvc{
		createFn: func(ctx context.Context, k, _ uuid.UUID, role string, in CreateInput, _, _ string) (*Pengumuman, error) {
			if k != kelasID {
				t.Fatalf("kelasID mismatch")
			}
			if in.Judul != "Halo" {
				t.Fatalf("judul mismatch %q", in.Judul)
			}
			return &Pengumuman{ID: id, KelasID: k, Judul: in.Judul, Status: StatusPublished, Version: 1}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+kelasID.String()+"/pengumuman", map[string]any{
		"judul": "Halo", "isi": "isi pengumuman",
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_List_InvalidStatus(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/pengumuman?status=funky", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_Update_VersionConflict(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, _, _ uuid.UUID, _ string, _ UpdateInput, _, _ string) (*Pengumuman, error) {
			return nil, ErrVersionConflict
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "PATCH", "/pengumuman/"+id.String(), map[string]any{
		"version": 5, "judul": "x",
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := &stubSvc{
		deleteFn: func(ctx context.Context, _, _ uuid.UUID, _, _, _ string) (*Pengumuman, error) {
			return nil, ErrNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "DELETE", "/pengumuman/"+id.String(), nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
