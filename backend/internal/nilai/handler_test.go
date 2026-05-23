package nilai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

type stubNilaiSvc struct {
	siswaKelasResult *SiswaKelasNilaiResponse
	siswaListResult  *SiswaListResponse
	guruRekapResult  *GuruRekapResponse
	err              error

	kelasID    uuid.UUID
	callerID   uuid.UUID
	callerRole string
}

func (s *stubNilaiSvc) SiswaKelasNilai(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) (*SiswaKelasNilaiResponse, error) {
	s.kelasID = kelasID
	s.callerID = siswaID
	s.callerRole = callerRole
	return s.siswaKelasResult, s.err
}

func (s *stubNilaiSvc) SiswaList(ctx context.Context, siswaID uuid.UUID, callerRole string) (*SiswaListResponse, error) {
	s.callerID = siswaID
	s.callerRole = callerRole
	return s.siswaListResult, s.err
}

func (s *stubNilaiSvc) GuruKelasRekap(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, enrollLookup rekapEnrollmentLookup, userLookup rekapUserLookup) (*GuruRekapResponse, error) {
	s.kelasID = kelasID
	s.callerID = callerID
	s.callerRole = callerRole
	return s.guruRekapResult, s.err
}

func TestHandlerSiswaKelasNilai(t *testing.T) {
	kelasID := uuid.New()
	siswaID := uuid.New()
	svc := &stubNilaiSvc{siswaKelasResult: &SiswaKelasNilaiResponse{Kelas: KelasInfo{ID: kelasID, Nama: "VII A"}}}
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Get("/siswa/kelas/:id/nilai", withNilaiUser(siswaID, string(auth.Siswa)), h.SiswaKelasNilai)

	resp, err := app.Test(httptest.NewRequest("GET", "/siswa/kelas/"+kelasID.String()+"/nilai", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if svc.kelasID != kelasID || svc.callerID != siswaID || svc.callerRole != string(auth.Siswa) {
		t.Fatalf("service args mismatch")
	}
	var body SiswaKelasNilaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Kelas.Nama != "VII A" {
		t.Fatalf("kelas nama = %q", body.Kelas.Nama)
	}
}

func TestHandlerSiswaKelasNilaiInvalidID(t *testing.T) {
	app := fiber.New()
	h := &Handler{svc: &stubNilaiSvc{}}
	app.Get("/siswa/kelas/:id/nilai", withNilaiUser(uuid.New(), string(auth.Siswa)), h.SiswaKelasNilai)

	resp, err := app.Test(httptest.NewRequest("GET", "/siswa/kelas/not-a-uuid/nilai", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertNilaiErrorCode(t, resp, fiber.StatusBadRequest, "invalid_id")
}

func TestHandlerSiswaList(t *testing.T) {
	siswaID := uuid.New()
	svc := &stubNilaiSvc{siswaListResult: &SiswaListResponse{Items: []SiswaKelasSummary{{KelasNama: "VII A"}}}}
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Get("/siswa/nilai", withNilaiUser(siswaID, string(auth.Siswa)), h.SiswaList)

	resp, err := app.Test(httptest.NewRequest("GET", "/siswa/nilai", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if svc.callerID != siswaID || svc.callerRole != string(auth.Siswa) {
		t.Fatalf("service args mismatch")
	}
}

func TestHandlerGuruKelasRekapJSONAndCSV(t *testing.T) {
	kelasID := uuid.New()
	guruID := uuid.New()
	res := &GuruRekapResponse{
		Kelas: KelasInfo{ID: kelasID, Nama: "VII A"},
		Rows:  []RekapRow{{SiswaID: uuid.New(), SiswaNama: "Budi"}},
	}
	svc := &stubNilaiSvc{guruRekapResult: res}
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Get("/kelas/:id/rekap", withNilaiUser(guruID, string(auth.Guru)), h.GuruKelasRekap)

	resp, err := app.Test(httptest.NewRequest("GET", "/kelas/"+kelasID.String()+"/rekap", nil))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("json status = %d", resp.StatusCode)
	}
	if svc.kelasID != kelasID || svc.callerID != guruID || svc.callerRole != string(auth.Guru) {
		t.Fatalf("service args mismatch")
	}

	resp, err = app.Test(httptest.NewRequest("GET", "/kelas/"+kelasID.String()+"/rekap?format=csv", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("csv status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestHandlerGuruKelasRekapRejectsSiswaBeforeService(t *testing.T) {
	kelasID := uuid.New()
	svc := &stubNilaiSvc{guruRekapResult: &GuruRekapResponse{}}
	app := fiber.New()
	h := &Handler{svc: svc}
	app.Get("/kelas/:id/rekap", withNilaiUser(uuid.New(), string(auth.Siswa)), h.GuruKelasRekap)

	resp, err := app.Test(httptest.NewRequest("GET", "/kelas/"+kelasID.String()+"/rekap", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	assertNilaiErrorCode(t, resp, fiber.StatusForbidden, "forbidden")
	if svc.kelasID != uuid.Nil {
		t.Fatal("service should not be called")
	}
}

func TestMapErr(t *testing.T) {
	app := fiber.New()
	app.Get("/forbidden", func(c *fiber.Ctx) error { return mapErr(c, ErrForbidden) })
	app.Get("/missing", func(c *fiber.Ctx) error { return mapErr(c, ErrNotFound) })
	app.Get("/boom", func(c *fiber.Ctx) error { return mapErr(c, errors.New("boom")) })

	cases := []struct {
		path   string
		status int
		code   string
	}{
		{"/forbidden", fiber.StatusForbidden, "forbidden"},
		{"/missing", fiber.StatusNotFound, "not_found"},
		{"/boom", fiber.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		assertNilaiErrorCode(t, resp, tc.status, tc.code)
		resp.Body.Close()
	}
}

func withNilaiUser(userID uuid.UUID, role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	}
}

func assertNilaiErrorCode(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status = %d, want %d", resp.StatusCode, status)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Code != code {
		t.Fatalf("code = %q, want %q", body.Code, code)
	}
}
