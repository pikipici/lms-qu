package materi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
)

// newSvcWithEnroll wires a service with a real enrollment lookup stub.
func newSvcWithEnroll(repo *fakeRepo, k *kelas.Kelas, enroll *fakeEnroll) *Service {
	audit := &fakeAudit{}
	kl := &fakeKelas{rec: k}
	if k == nil {
		kl.err = gorm.ErrRecordNotFound
	}
	return NewService(repo, kl, &fakeBab{}, audit, nil, enroll)
}

// ---------- Service.MarkRead ----------

func TestService_MarkRead_Happy(t *testing.T) {
	guruID := uuid.New()
	siswaID := uuid.New()
	k := ownedKelas(guruID)
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID, Tipe: TipeMarkdown}, nil
		},
	}
	enroll := &fakeEnroll{rec: &kelas.Enrollment{
		KelasID: k.ID, SiswaID: siswaID, Status: kelas.EnrollmentActive,
		JoinedAt: time.Now(),
	}}
	svc := newSvcWithEnroll(repo, k, enroll)

	res, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if !res.WasNew {
		t.Fatalf("expected was_new=true on first call")
	}
	if res.MateriID != materiID {
		t.Fatalf("materi_id mismatch")
	}
	if res.ReadAt == "" {
		t.Fatalf("read_at empty")
	}
}

func TestService_MarkRead_Idempotent(t *testing.T) {
	siswaID := uuid.New()
	k := ownedKelas(uuid.New())
	materiID := uuid.New()
	calls := 0
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID, Tipe: TipeMarkdown}, nil
		},
		markReadFn: func(ctx context.Context, mID, sID uuid.UUID) (*Read, bool, error) {
			calls++
			wasNew := calls == 1
			return &Read{MateriID: mID, SiswaID: sID, ReadAt: time.Now()}, wasNew, nil
		},
	}
	enroll := &fakeEnroll{rec: &kelas.Enrollment{
		KelasID: k.ID, SiswaID: siswaID, Status: kelas.EnrollmentActive,
	}}
	svc := newSvcWithEnroll(repo, k, enroll)

	first, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if err != nil || !first.WasNew {
		t.Fatalf("first call: err=%v wasNew=%v", err, first.WasNew)
	}
	second, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if second.WasNew {
		t.Fatalf("expected was_new=false on idempotent re-call")
	}
}

func TestService_MarkRead_GuruRejected(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID}, nil
		},
	}
	svc := newSvcWithEnroll(repo, k, &fakeEnroll{})

	_, err := svc.MarkRead(context.Background(), materiID, guruID, string(auth.Guru))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for guru, got %v", err)
	}
}

func TestService_MarkRead_AdminRejected(t *testing.T) {
	adminID := uuid.New()
	k := ownedKelas(uuid.New())
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID}, nil
		},
	}
	svc := newSvcWithEnroll(repo, k, &fakeEnroll{})

	_, err := svc.MarkRead(context.Background(), materiID, adminID, string(auth.Admin))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for admin, got %v", err)
	}
}

func TestService_MarkRead_NotFound(t *testing.T) {
	siswaID := uuid.New()
	k := ownedKelas(uuid.New())
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := newSvcWithEnroll(repo, k, &fakeEnroll{rec: &kelas.Enrollment{Status: kelas.EnrollmentActive}})

	_, err := svc.MarkRead(context.Background(), uuid.New(), siswaID, string(auth.Siswa))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_MarkRead_NotEnrolled(t *testing.T) {
	siswaID := uuid.New()
	k := ownedKelas(uuid.New())
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID}, nil
		},
	}
	// no enrollment row
	svc := newSvcWithEnroll(repo, k, &fakeEnroll{err: gorm.ErrRecordNotFound})

	_, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for non-enrolled siswa, got %v", err)
	}
}

func TestService_MarkRead_RemovedEnrollmentRejected(t *testing.T) {
	siswaID := uuid.New()
	k := ownedKelas(uuid.New())
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID}, nil
		},
	}
	svc := newSvcWithEnroll(repo, k, &fakeEnroll{rec: &kelas.Enrollment{
		KelasID: k.ID, SiswaID: siswaID, Status: kelas.EnrollmentRemoved,
	}})

	_, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden for removed enrollment, got %v", err)
	}
}

func TestService_MarkRead_EnrollNilDisabled(t *testing.T) {
	siswaID := uuid.New()
	k := ownedKelas(uuid.New())
	materiID := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: materiID, KelasID: k.ID}, nil
		},
	}
	// enroll=nil → disabled wiring; service should refuse loudly
	svc := NewService(repo, &fakeKelas{rec: k}, &fakeBab{}, &fakeAudit{}, nil, nil)

	_, err := svc.MarkRead(context.Background(), materiID, siswaID, string(auth.Siswa))
	if err == nil {
		t.Fatalf("expected error when enroll lookup is nil")
	}
}

// ---------- Handler.MarkRead ----------

func newReadApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Post("/materi/:id/read", h.MarkRead)
	return app
}

func TestHandler_MarkRead_Happy(t *testing.T) {
	siswaID := uuid.New()
	materiID := uuid.New()
	stub := &stubSvc{
		markReadFn: func(ctx context.Context, mID, sID uuid.UUID, role string) (*MarkReadResult, error) {
			if mID != materiID || sID != siswaID || role != string(auth.Siswa) {
				t.Fatalf("unexpected args: m=%s s=%s r=%s", mID, sID, role)
			}
			return &MarkReadResult{
				MateriID: mID,
				ReadAt:   "2026-05-21T09:00:00.000Z",
				WasNew:   true,
			}, nil
		},
	}
	h := &Handler{svc: stub}
	app := newReadApp(t, h, string(auth.Siswa), siswaID)

	req := httptest.NewRequest(http.MethodPost, "/materi/"+materiID.String()+"/read", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
	var body struct {
		MateriID uuid.UUID `json:"materi_id"`
		ReadAt   string    `json:"read_at"`
		WasNew   bool      `json:"was_new"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.MateriID != materiID || !body.WasNew || body.ReadAt == "" {
		t.Fatalf("body mismatch: %+v", body)
	}
}

func TestHandler_MarkRead_ForwardsForbidden(t *testing.T) {
	siswaID := uuid.New()
	materiID := uuid.New()
	stub := &stubSvc{
		markReadFn: func(ctx context.Context, mID, sID uuid.UUID, role string) (*MarkReadResult, error) {
			return nil, ErrForbidden
		},
	}
	h := &Handler{svc: stub}
	app := newReadApp(t, h, string(auth.Siswa), siswaID)

	req := httptest.NewRequest(http.MethodPost, "/materi/"+materiID.String()+"/read", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status=%d want 403", resp.StatusCode)
	}
}

func TestHandler_MarkRead_InvalidID(t *testing.T) {
	stub := &stubSvc{}
	h := &Handler{svc: stub}
	app := newReadApp(t, h, string(auth.Siswa), uuid.New())

	req := httptest.NewRequest(http.MethodPost, "/materi/not-a-uuid/read", bytes.NewReader(nil))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", resp.StatusCode)
	}
}
