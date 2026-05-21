package tugas

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
	createFn func(ctx context.Context, t *Tugas) error
	findFn   func(ctx context.Context, id uuid.UUID) (*Tugas, error)
	listFn   func(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Tugas, error)
	updateFn func(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error
	deleteFn func(ctx context.Context, id uuid.UUID) ([]string, error)
}

func (r *stubRepo) Create(ctx context.Context, t *Tugas) error {
	return r.createFn(ctx, t)
}
func (r *stubRepo) FindByID(ctx context.Context, id uuid.UUID) (*Tugas, error) {
	return r.findFn(ctx, id)
}
func (r *stubRepo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Tugas, error) {
	return r.listFn(ctx, kelasID, f)
}
func (r *stubRepo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error {
	return r.updateFn(ctx, id, expectedVersion, fields)
}
func (r *stubRepo) Delete(ctx context.Context, id uuid.UUID) ([]string, error) {
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

func activeKelas(id, guruID uuid.UUID) *kelas.Kelas {
	return &kelas.Kelas{ID: id, GuruID: guruID, Nama: "Kelas X"}
}

func archivedKelas(id, guruID uuid.UUID) *kelas.Kelas {
	now := time.Now()
	return &kelas.Kelas{ID: id, GuruID: guruID, Nama: "Kelas X", ArchivedAt: &now}
}

func activeEnrollment(kelasID, siswaID uuid.UUID) *kelas.Enrollment {
	return &kelas.Enrollment{KelasID: kelasID, SiswaID: siswaID, Status: kelas.EnrollmentActive}
}

// ---------- Service tests ----------

func TestService_Create_HappyPath(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	repo := &stubRepo{
		createFn: func(ctx context.Context, tg *Tugas) error {
			if tg.KelasID != kelasID {
				t.Fatalf("kelas_id mismatch")
			}
			if tg.Status != StatusDraft {
				t.Fatalf("status default mismatch %q (want draft)", tg.Status)
			}
			if tg.Version != 1 {
				t.Fatalf("version mismatch %d", tg.Version)
			}
			if tg.IzinkanLate {
				t.Fatalf("izinkan_late default mismatch")
			}
			if tg.PenaltyPersen != 0 {
				t.Fatalf("penalty default mismatch %d", tg.PenaltyPersen)
			}
			tg.ID = uuid.New()
			return nil
		},
	}
	k := &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}
	audit := &stubAudit{}
	svc := NewService(repo, k, &stubBab{}, nil, audit)

	tg, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "  Tugas Pertama  ", Deskripsi: "kerjakan ya"}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if tg.Judul != "Tugas Pertama" {
		t.Fatalf("judul not trimmed: %q", tg.Judul)
	}
	if len(audit.logged) != 1 || audit.logged[0].Action != "tugas_created" {
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
		CreateInput{Judul: "   ", Deskripsi: ""}, "", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_Create_RejectDeskripsiTooLong(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	huge := strings.Repeat("x", MaxDeskripsiBytes+1)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok", Deskripsi: huge}, "", "")
	if !errors.Is(err, ErrDeskripsiTooLong) {
		t.Fatalf("expected ErrDeskripsiTooLong, got %v", err)
	}
}

func TestService_Create_RejectPenaltyOutOfRange(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	svc := NewService(&stubRepo{}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	_, err := svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok", PenaltyPersen: 150}, "", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for penalty>100, got %v", err)
	}
	_, err = svc.Create(context.Background(), kelasID, guruID, string(auth.Guru),
		CreateInput{Judul: "ok", PenaltyPersen: -1}, "", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for penalty<0, got %v", err)
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
		listFn: func(ctx context.Context, k uuid.UUID, f ListFilter) ([]Tugas, error) {
			calls++
			if f.Status == nil || *f.Status != StatusPublished {
				t.Fatalf("siswa list must be pinned to published, got %+v", f.Status)
			}
			return []Tugas{}, nil
		},
	}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, uuid.New()), nil
	}}, &stubBab{}, &stubEnroll{findFn: func(ctx context.Context, kID, sID uuid.UUID) (*kelas.Enrollment, error) {
		return activeEnrollment(kID, sID), nil
	}}, nil)

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
		listFn: func(ctx context.Context, k uuid.UUID, f ListFilter) ([]Tugas, error) {
			called = true
			if f.Status != nil {
				t.Fatalf("guru status must be nil (no filter), got %+v", *f.Status)
			}
			return []Tugas{{ID: uuid.New(), Status: StatusDraft}}, nil
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

func TestService_Get_Siswa_DraftHidden(t *testing.T) {
	siswaID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return &Tugas{ID: id, KelasID: kelasID, Status: StatusDraft}, nil
		},
	}, &stubKelas{}, &stubBab{}, nil, nil)
	_, err := svc.Get(context.Background(), id, siswaID, string(auth.Siswa))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for draft siswa view, got %v", err)
	}
}

func TestService_Get_Siswa_ArchivedHidden(t *testing.T) {
	siswaID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return &Tugas{ID: id, KelasID: kelasID, Status: StatusArchived}, nil
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
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return &Tugas{ID: id, KelasID: kelasID, Version: 5, Judul: "old", Status: StatusDraft}, nil
		},
		updateFn: func(ctx context.Context, _ uuid.UUID, expVer int, _ map[string]any) error {
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

func TestService_Update_StatusChangedAuditAction(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	updated := false
	repo := &stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			if updated {
				return &Tugas{ID: id, KelasID: kelasID, Version: 6, Status: StatusPublished, Judul: "x"}, nil
			}
			return &Tugas{ID: id, KelasID: kelasID, Version: 5, Status: StatusDraft, Judul: "x"}, nil
		},
		updateFn: func(ctx context.Context, _ uuid.UUID, _ int, _ map[string]any) error {
			updated = true
			return nil
		},
	}
	audit := &stubAudit{}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, audit)
	st := StatusPublished
	_, err := svc.Update(context.Background(), id, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion: 5, Status: &st,
	}, "", "")
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if len(audit.logged) != 1 || audit.logged[0].Action != "tugas_status_changed" {
		t.Fatalf("audit action mismatch: %+v", audit.logged)
	}
}

func TestService_Update_NoOpReturnsExisting(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	id := uuid.New()
	updateCalled := false
	repo := &stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return &Tugas{ID: id, KelasID: kelasID, Version: 5, Judul: "x", Status: StatusDraft}, nil
		},
		updateFn: func(ctx context.Context, _ uuid.UUID, _ int, _ map[string]any) error {
			updateCalled = true
			return nil
		},
	}
	svc := NewService(repo, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	// Patch with same judul as existing (no-op).
	sameJudul := "x"
	_, err := svc.Update(context.Background(), id, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion: 5, Judul: &sameJudul,
	}, "", "")
	if err != nil {
		t.Fatalf("Update err: %v", err)
	}
	if updateCalled {
		t.Fatalf("repo.UpdateBasic must not be called for no-op patch")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := NewService(&stubRepo{
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}, &stubKelas{}, &stubBab{}, nil, nil)
	_, _, err := svc.Delete(context.Background(), id, guruID, string(auth.Guru), "", "")
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
		findFn: func(ctx context.Context, _ uuid.UUID) (*Tugas, error) {
			return &Tugas{ID: id, KelasID: kelasID, Judul: "x", Status: StatusPublished}, nil
		},
		deleteFn: func(ctx context.Context, _ uuid.UUID) ([]string, error) {
			deleted = true
			return []string{"tugas/abc.pdf"}, nil
		},
	}, &stubKelas{findFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
		return activeKelas(id, guruID), nil
	}}, &stubBab{}, nil, nil)
	_, keys, err := svc.Delete(context.Background(), id, guruID, string(auth.Guru), "", "")
	if err != nil {
		t.Fatalf("Delete err: %v", err)
	}
	if !deleted {
		t.Fatalf("repo Delete not called")
	}
	if len(keys) != 1 || keys[0] != "tugas/abc.pdf" {
		t.Fatalf("keys mismatch: %+v", keys)
	}
}

// ---------- Handler tests (smoke) ----------

type stubSvc struct {
	createFn func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Tugas, error)
	listFn   func(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Tugas, error)
	getFn    func(ctx context.Context, id, callerID uuid.UUID, role string) (*Tugas, error)
	updateFn func(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Tugas, error)
	deleteFn func(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Tugas, []string, error)
}

func (s *stubSvc) Create(ctx context.Context, kelasID, callerID uuid.UUID, role string, in CreateInput, ip, ua string) (*Tugas, error) {
	return s.createFn(ctx, kelasID, callerID, role, in, ip, ua)
}
func (s *stubSvc) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, role string, in ListInput) ([]Tugas, error) {
	return s.listFn(ctx, kelasID, callerID, role, in)
}
func (s *stubSvc) Get(ctx context.Context, id, callerID uuid.UUID, role string) (*Tugas, error) {
	return s.getFn(ctx, id, callerID, role)
}
func (s *stubSvc) Update(ctx context.Context, id, callerID uuid.UUID, role string, in UpdateInput, ip, ua string) (*Tugas, error) {
	return s.updateFn(ctx, id, callerID, role, in, ip, ua)
}
func (s *stubSvc) Delete(ctx context.Context, id, callerID uuid.UUID, role, ip, ua string) (*Tugas, []string, error) {
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
	app.Post("/kelas/:id/tugas", h.Create)
	app.Get("/kelas/:id/tugas", h.ListByKelas)
	app.Get("/tugas/:id", h.Get)
	app.Patch("/tugas/:id", h.Update)
	app.Delete("/tugas/:id", h.Delete)
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
		createFn: func(ctx context.Context, k, _ uuid.UUID, role string, in CreateInput, _, _ string) (*Tugas, error) {
			if k != kelasID {
				t.Fatalf("kelasID mismatch")
			}
			if in.Judul != "Tugas 1" {
				t.Fatalf("judul mismatch %q", in.Judul)
			}
			if !in.IzinkanLate {
				t.Fatalf("izinkan_late mismatch")
			}
			if in.PenaltyPersen != 25 {
				t.Fatalf("penalty mismatch %d", in.PenaltyPersen)
			}
			return &Tugas{ID: id, KelasID: k, Judul: in.Judul, Status: StatusDraft, Version: 1,
				IzinkanLate: in.IzinkanLate, PenaltyPersen: in.PenaltyPersen}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doReq(t, app, "POST", "/kelas/"+kelasID.String()+"/tugas", map[string]any{
		"judul":          "Tugas 1",
		"deskripsi":      "kerjakan",
		"izinkan_late":   true,
		"penalty_persen": 25,
	})
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestHandler_List_InvalidStatus(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/tugas?status=funky", nil)
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_List_BabIDNullKeyword(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	called := false
	svc := &stubSvc{
		listFn: func(ctx context.Context, _, _ uuid.UUID, _ string, in ListInput) ([]Tugas, error) {
			called = true
			if in.BabID == nil || *in.BabID != uuid.Nil {
				t.Fatalf("expected BabID=&uuid.Nil for bab_id=null query, got %+v", in.BabID)
			}
			return []Tugas{}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "GET", "/kelas/"+kelasID.String()+"/tugas?bab_id=null", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !called {
		t.Fatalf("svc not called")
	}
}

func TestHandler_Update_VersionConflict(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := &stubSvc{
		updateFn: func(ctx context.Context, _, _ uuid.UUID, _ string, _ UpdateInput, _, _ string) (*Tugas, error) {
			return nil, ErrVersionConflict
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "PATCH", "/tugas/"+id.String(), map[string]any{
		"version": 5, "judul": "x",
	})
	if resp.StatusCode != fiber.StatusConflict {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestHandler_Update_BabIDExplicitNull(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	called := false
	svc := &stubSvc{
		updateFn: func(ctx context.Context, _, _ uuid.UUID, _ string, in UpdateInput, _, _ string) (*Tugas, error) {
			called = true
			if !in.BabIDExplicit {
				t.Fatalf("BabIDExplicit must be true for explicit null")
			}
			if in.BabID != nil {
				t.Fatalf("BabID must be nil for null clear, got %+v", in.BabID)
			}
			return &Tugas{ID: id, Version: 6}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	body := []byte(`{"version": 5, "bab_id": null}`)
	req := httptest.NewRequest("PATCH", "/tugas/"+id.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !called {
		t.Fatalf("svc not called")
	}
}

func TestHandler_Delete_NotFound(t *testing.T) {
	guruID := uuid.New()
	id := uuid.New()
	svc := &stubSvc{
		deleteFn: func(ctx context.Context, _, _ uuid.UUID, _, _, _ string) (*Tugas, []string, error) {
			return nil, nil, ErrNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, _ := doReq(t, app, "DELETE", "/tugas/"+id.String(), nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
