package siswabab

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/materi"
	"github.com/pikip/lms/backend/internal/middleware"
)

// ---------- Stubs ----------

type stubBab struct {
	listByKelasFn func(ctx context.Context, kelasID uuid.UUID, f bab.ListFilter) ([]bab.Bab, error)
	findByIDFn    func(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
}

func (s *stubBab) FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
	if s.findByIDFn != nil {
		return s.findByIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}
func (s *stubBab) ListByKelas(ctx context.Context, kelasID uuid.UUID, f bab.ListFilter) ([]bab.Bab, error) {
	if s.listByKelasFn != nil {
		return s.listByKelasFn(ctx, kelasID, f)
	}
	return nil, nil
}

type stubKelas struct {
	findByIDFn func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

func (s *stubKelas) FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
	if s.findByIDFn != nil {
		return s.findByIDFn(ctx, id)
	}
	return nil, gorm.ErrRecordNotFound
}

type stubEnroll struct {
	findFn func(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

func (s *stubEnroll) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error) {
	if s.findFn != nil {
		return s.findFn(ctx, kelasID, siswaID)
	}
	return nil, gorm.ErrRecordNotFound
}

type stubMateri struct {
	listByBabFn        func(ctx context.Context, babID uuid.UUID) ([]materi.Materi, error)
	countByBatchFn     func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error)
	countReadByBatchFn func(ctx context.Context, ids []uuid.UUID, siswaID uuid.UUID) (map[uuid.UUID]int64, error)
	listReadIDsFn      func(ctx context.Context, babID, siswaID uuid.UUID) ([]uuid.UUID, error)
}

func (s *stubMateri) ListByBab(ctx context.Context, babID uuid.UUID) ([]materi.Materi, error) {
	if s.listByBabFn != nil {
		return s.listByBabFn(ctx, babID)
	}
	return nil, nil
}
func (s *stubMateri) CountByBabBatch(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	if s.countByBatchFn != nil {
		return s.countByBatchFn(ctx, ids)
	}
	out := make(map[uuid.UUID]int64, len(ids))
	for _, id := range ids {
		out[id] = 0
	}
	return out, nil
}
func (s *stubMateri) CountReadByBabBatch(ctx context.Context, ids []uuid.UUID, siswaID uuid.UUID) (map[uuid.UUID]int64, error) {
	if s.countReadByBatchFn != nil {
		return s.countReadByBatchFn(ctx, ids, siswaID)
	}
	out := make(map[uuid.UUID]int64, len(ids))
	for _, id := range ids {
		out[id] = 0
	}
	return out, nil
}
func (s *stubMateri) ListReadIDsByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID) ([]uuid.UUID, error) {
	if s.listReadIDsFn != nil {
		return s.listReadIDsFn(ctx, babID, siswaID)
	}
	return nil, nil
}

// ---------- Service tests ----------

func TestService_ListSiswa_HappyPath(t *testing.T) {
	kelasID := uuid.New()
	siswaID := uuid.New()
	bab1ID := uuid.New()
	bab2ID := uuid.New()

	bRepo := &stubBab{
		listByKelasFn: func(ctx context.Context, kid uuid.UUID, f bab.ListFilter) ([]bab.Bab, error) {
			if f.Status == nil || *f.Status != bab.StatusPublished {
				t.Fatalf("expected Status filter pinned to published, got %#v", f)
			}
			return []bab.Bab{
				{ID: bab1ID, KelasID: kid, Nomor: 1, Judul: "Bab Pertama", Urutan: 1, Status: bab.StatusPublished, Version: 1},
				{ID: bab2ID, KelasID: kid, Nomor: 2, Judul: "Bab Kedua (kosong)", Urutan: 2, Status: bab.StatusPublished, Version: 1},
			}, nil
		},
	}
	kRepo := &stubKelas{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
			return &kelas.Kelas{ID: id}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return &kelas.Enrollment{KelasID: kid, SiswaID: sid, Status: kelas.EnrollmentActive}, nil
		},
	}
	mRepo := &stubMateri{
		countByBatchFn: func(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
			return map[uuid.UUID]int64{bab1ID: 4, bab2ID: 0}, nil
		},
		countReadByBatchFn: func(ctx context.Context, ids []uuid.UUID, sid uuid.UUID) (map[uuid.UUID]int64, error) {
			return map[uuid.UUID]int64{bab1ID: 1, bab2ID: 0}, nil
		},
	}

	svc := NewService(bRepo, kRepo, eRepo, mRepo)
	rows, err := svc.ListSiswa(context.Background(), kelasID, siswaID, string(auth.Siswa))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Progress.MateriRead != 1 || rows[0].Progress.MateriAll != 4 {
		t.Errorf("bab1 progress: got read=%d total=%d", rows[0].Progress.MateriRead, rows[0].Progress.MateriAll)
	}
	if rows[0].Progress.Persen != 25.0 {
		t.Errorf("bab1 persen: want 25.0, got %v", rows[0].Progress.Persen)
	}
	if rows[0].Progress.BabKosong {
		t.Errorf("bab1 should not be kosong")
	}
	if !rows[1].Progress.BabKosong || rows[1].Progress.Persen != 0 {
		t.Errorf("bab2 should be kosong + 0%%, got %#v", rows[1].Progress)
	}
}

func TestService_ListSiswa_RejectsNonSiswa(t *testing.T) {
	svc := NewService(&stubBab{}, &stubKelas{}, &stubEnroll{}, &stubMateri{})
	_, err := svc.ListSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Guru))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("guru should hit ErrForbidden, got %v", err)
	}
	_, err = svc.ListSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Admin))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("admin should hit ErrForbidden, got %v", err)
	}
}

func TestService_ListSiswa_RejectsNonEnrolled(t *testing.T) {
	kRepo := &stubKelas{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
			return &kelas.Kelas{ID: id}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewService(&stubBab{}, kRepo, eRepo, &stubMateri{})
	_, err := svc.ListSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("non-enrolled should hit ErrForbidden, got %v", err)
	}
}

func TestService_ListSiswa_RejectsRemovedEnrollment(t *testing.T) {
	kRepo := &stubKelas{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
			return &kelas.Kelas{ID: id}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return &kelas.Enrollment{Status: kelas.EnrollmentRemoved}, nil
		},
	}
	svc := NewService(&stubBab{}, kRepo, eRepo, &stubMateri{})
	_, err := svc.ListSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("removed enrollment should hit ErrForbidden, got %v", err)
	}
}

func TestService_ListSiswa_KelasMissingMapsToForbidden(t *testing.T) {
	kRepo := &stubKelas{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewService(&stubBab{}, kRepo, &stubEnroll{}, &stubMateri{})
	_, err := svc.ListSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("missing kelas should map to ErrForbidden (no info leak), got %v", err)
	}
}

func TestService_GetSiswa_HappyPath(t *testing.T) {
	babID := uuid.New()
	kelasID := uuid.New()
	siswaID := uuid.New()
	mat1 := uuid.New()
	mat2 := uuid.New()

	bRepo := &stubBab{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
			return &bab.Bab{ID: id, KelasID: kelasID, Nomor: 1, Judul: "B1", Status: bab.StatusPublished, Version: 1, Urutan: 1}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return &kelas.Enrollment{Status: kelas.EnrollmentActive}, nil
		},
	}
	mRepo := &stubMateri{
		listByBabFn: func(ctx context.Context, bid uuid.UUID) ([]materi.Materi, error) {
			return []materi.Materi{
				{ID: mat1, KelasID: kelasID, BabID: &babID, Judul: "M1", Tipe: materi.TipeMarkdown, Konten: "## hi", Urutan: 1, Version: 1},
				{ID: mat2, KelasID: kelasID, BabID: &babID, Judul: "M2", Tipe: materi.TipeYouTube, Konten: "abcdefghijk", Urutan: 2, Version: 1},
			}, nil
		},
		listReadIDsFn: func(ctx context.Context, bid, sid uuid.UUID) ([]uuid.UUID, error) {
			return []uuid.UUID{mat1}, nil
		},
	}

	svc := NewService(bRepo, &stubKelas{}, eRepo, mRepo)
	got, err := svc.GetSiswa(context.Background(), babID, siswaID, string(auth.Siswa))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Materi) != 2 {
		t.Fatalf("expected 2 materi, got %d", len(got.Materi))
	}
	if !got.Materi[0].SudahDibaca {
		t.Errorf("mat1 should be sudah_dibaca=true")
	}
	if got.Materi[1].SudahDibaca {
		t.Errorf("mat2 should be sudah_dibaca=false")
	}
	if got.Bab.Progress.MateriRead != 1 || got.Bab.Progress.MateriAll != 2 {
		t.Errorf("bab progress: got read=%d total=%d, want 1/2", got.Bab.Progress.MateriRead, got.Bab.Progress.MateriAll)
	}
	if got.Bab.Progress.Persen != 50.0 {
		t.Errorf("bab persen: want 50.0, got %v", got.Bab.Progress.Persen)
	}
}

func TestService_GetSiswa_HidesDraftAsNotFound(t *testing.T) {
	bRepo := &stubBab{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
			return &bab.Bab{ID: id, KelasID: uuid.New(), Status: bab.StatusDraft, Version: 1}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return &kelas.Enrollment{Status: kelas.EnrollmentActive}, nil
		},
	}
	svc := NewService(bRepo, &stubKelas{}, eRepo, &stubMateri{})
	_, err := svc.GetSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrNotFound) {
		t.Fatalf("draft bab should map to ErrNotFound for siswa, got %v", err)
	}
}

func TestService_GetSiswa_HidesArchivedAsNotFound(t *testing.T) {
	bRepo := &stubBab{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
			return &bab.Bab{ID: id, KelasID: uuid.New(), Status: bab.StatusArchived, Version: 1}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return &kelas.Enrollment{Status: kelas.EnrollmentActive}, nil
		},
	}
	svc := NewService(bRepo, &stubKelas{}, eRepo, &stubMateri{})
	_, err := svc.GetSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrNotFound) {
		t.Fatalf("archived bab should map to ErrNotFound for siswa, got %v", err)
	}
}

func TestService_GetSiswa_RejectsNonEnrolled(t *testing.T) {
	bRepo := &stubBab{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
			return &bab.Bab{ID: id, KelasID: uuid.New(), Status: bab.StatusPublished, Version: 1}, nil
		},
	}
	eRepo := &stubEnroll{
		findFn: func(ctx context.Context, kid, sid uuid.UUID) (*kelas.Enrollment, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewService(bRepo, &stubKelas{}, eRepo, &stubMateri{})
	_, err := svc.GetSiswa(context.Background(), uuid.New(), uuid.New(), string(auth.Siswa))
	if !errors.Is(err, bab.ErrForbidden) {
		t.Fatalf("non-enrolled siswa GetSiswa should hit ErrForbidden, got %v", err)
	}
}

func TestComputeProgress_BoundaryCases(t *testing.T) {
	cases := []struct {
		name     string
		total    int64
		read     int64
		wantPct  float64
		wantKoso bool
	}{
		{"kosong", 0, 0, 0, true},
		{"all read", 4, 4, 100.0, false},
		{"none read", 4, 0, 0.0, false},
		{"third", 3, 1, 33.33, false},
		{"two thirds", 3, 2, 66.67, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := computeProgress(c.total, c.read)
			if p.Persen != c.wantPct {
				t.Errorf("persen: got %v, want %v", p.Persen, c.wantPct)
			}
			if p.BabKosong != c.wantKoso {
				t.Errorf("bab_kosong: got %v, want %v", p.BabKosong, c.wantKoso)
			}
		})
	}
}

// ---------- Handler tests ----------

type stubSvc struct {
	listFn func(ctx context.Context, kelasID, siswaID uuid.UUID, role string) ([]SiswaBabItem, error)
	getFn  func(ctx context.Context, babID, siswaID uuid.UUID, role string) (*SiswaBabDetail, error)
}

func (s *stubSvc) ListSiswa(ctx context.Context, kelasID, siswaID uuid.UUID, role string) ([]SiswaBabItem, error) {
	return s.listFn(ctx, kelasID, siswaID, role)
}
func (s *stubSvc) GetSiswa(ctx context.Context, babID, siswaID uuid.UUID, role string) (*SiswaBabDetail, error) {
	return s.getFn(ctx, babID, siswaID, role)
}

func newApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Get("/siswa/kelas/:id/bab", h.ListSiswa)
	app.Get("/siswa/bab/:id", h.GetSiswa)
	return app
}

func doReq(t *testing.T, app *fiber.App, method, path string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 0)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	_ = resp.Body.Close()
	return resp, body
}

func TestHandler_ListSiswa_HappyPath(t *testing.T) {
	kelasID := uuid.New()
	siswaID := uuid.New()
	babID := uuid.New()

	stub := &stubSvc{
		listFn: func(ctx context.Context, kid, sid uuid.UUID, role string) ([]SiswaBabItem, error) {
			if kid != kelasID || sid != siswaID || role != string(auth.Siswa) {
				t.Fatalf("unexpected args: %v %v %s", kid, sid, role)
			}
			pct := 25.0
			return []SiswaBabItem{{
				ID:    babID,
				Nomor: 1, Judul: "B1", Urutan: 1, Status: bab.StatusPublished,
				Progress: Progress{
					Persen: pct, MateriRead: 1, MateriAll: 4,
					Breakdown: map[string]ProgressBreakdownItem{"materi": {Pct: &pct, Weight: 1.0}},
				},
			}}, nil
		},
	}
	h := &Handler{svc: stub}
	app := newApp(t, h, string(auth.Siswa), siswaID)
	resp, body := doReq(t, app, http.MethodGet, "/siswa/kelas/"+kelasID.String()+"/bab")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, body)
	}
	var got SiswaListResponse
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("payload: %#v", got)
	}
	if got.Items[0].Progress.Persen != 25.0 {
		t.Errorf("persen: got %v, want 25", got.Items[0].Progress.Persen)
	}
}

func TestHandler_ListSiswa_ForbiddenForGuru(t *testing.T) {
	stub := &stubSvc{
		listFn: func(ctx context.Context, kid, sid uuid.UUID, role string) ([]SiswaBabItem, error) {
			return nil, bab.ErrForbidden
		},
	}
	h := &Handler{svc: stub}
	app := newApp(t, h, string(auth.Guru), uuid.New())
	resp, body := doReq(t, app, http.MethodGet, "/siswa/kelas/"+uuid.New().String()+"/bab")
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403; body=%s", resp.StatusCode, body)
	}
}

func TestHandler_ListSiswa_InvalidKelasID(t *testing.T) {
	h := &Handler{svc: &stubSvc{}}
	app := newApp(t, h, string(auth.Siswa), uuid.New())
	resp, _ := doReq(t, app, http.MethodGet, "/siswa/kelas/not-a-uuid/bab")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestHandler_GetSiswa_NotFoundForDraft(t *testing.T) {
	stub := &stubSvc{
		getFn: func(ctx context.Context, bid, sid uuid.UUID, role string) (*SiswaBabDetail, error) {
			return nil, bab.ErrNotFound
		},
	}
	h := &Handler{svc: stub}
	app := newApp(t, h, string(auth.Siswa), uuid.New())
	resp, body := doReq(t, app, http.MethodGet, "/siswa/bab/"+uuid.New().String())
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404; body=%s", resp.StatusCode, body)
	}
}

func TestHandler_GetSiswa_HappyPath(t *testing.T) {
	babID := uuid.New()
	pct := 50.0
	stub := &stubSvc{
		getFn: func(ctx context.Context, bid, sid uuid.UUID, role string) (*SiswaBabDetail, error) {
			if bid != babID {
				t.Fatalf("got bid %v, want %v", bid, babID)
			}
			return &SiswaBabDetail{
				Bab: SiswaBabItem{
					ID: bid, Nomor: 1, Judul: "B1", Status: bab.StatusPublished, Urutan: 1,
					Progress: Progress{
						Persen: pct, MateriRead: 1, MateriAll: 2,
						Breakdown: map[string]ProgressBreakdownItem{"materi": {Pct: &pct, Weight: 1.0}},
					},
				},
				Materi: []SiswaMateriCard{
					{ID: uuid.New(), BabID: &bid, Judul: "M1", Tipe: "markdown", Konten: "## hi", Urutan: 1, SudahDibaca: true},
				},
			}, nil
		},
	}
	h := &Handler{svc: stub}
	app := newApp(t, h, string(auth.Siswa), uuid.New())
	resp, body := doReq(t, app, http.MethodGet, "/siswa/bab/"+babID.String())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", resp.StatusCode, body)
	}
	var got SiswaBabDetail
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Bab.ID != babID {
		t.Errorf("bab id mismatch: got %v, want %v", got.Bab.ID, babID)
	}
	if !got.Materi[0].SudahDibaca {
		t.Errorf("expected M1 sudah_dibaca=true")
	}
}

func TestHandler_ListSiswa_UnexpectedErrorMaps500(t *testing.T) {
	stub := &stubSvc{
		listFn: func(ctx context.Context, kid, sid uuid.UUID, role string) ([]SiswaBabItem, error) {
			return nil, errors.New("boom")
		},
	}
	h := &Handler{svc: stub}
	app := newApp(t, h, string(auth.Siswa), uuid.New())
	resp, _ := doReq(t, app, http.MethodGet, "/siswa/kelas/"+uuid.New().String()+"/bab")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want 500", resp.StatusCode)
	}
}
