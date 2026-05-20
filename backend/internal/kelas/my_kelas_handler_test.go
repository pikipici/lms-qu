package kelas

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
)

func TestHandler_ListMyKelas_HappyPath(t *testing.T) {
	siswa := uuid.New()
	k1 := Kelas{ID: uuid.New(), Nama: "Mat", GuruID: uuid.New(), Version: 1}
	k2 := Kelas{ID: uuid.New(), Nama: "IPA", GuruID: uuid.New(), Version: 1}
	svc := &stubSvc{
		listMyKelasFn: func(ctx context.Context, sID uuid.UUID, in ListInput) (*MyKelasResult, error) {
			if sID != siswa {
				t.Fatalf("siswa id mismatch")
			}
			return &MyKelasResult{
				Items: []MyKelasItem{
					{Kelas: k1, JoinedVia: JoinedViaKode},
					{Kelas: k2, JoinedVia: JoinedViaAdmin},
				},
				Total: 2,
			}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), siswa)
	resp, body := doReq(t, app, "GET", "/siswa/kelas?page=1&page_size=10", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}
	var out myKelasResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 2 || len(out.Items) != 2 {
		t.Fatalf("unexpected items %+v", out)
	}
	if out.Page != 1 || out.PageSize != 10 || out.TotalPages != 1 {
		t.Fatalf("pagination mismatch: %+v", out)
	}
}

func TestHandler_ListMyKelas_DefaultsAndCap(t *testing.T) {
	siswa := uuid.New()
	captured := ListInput{}
	svc := &stubSvc{
		listMyKelasFn: func(ctx context.Context, sID uuid.UUID, in ListInput) (*MyKelasResult, error) {
			captured = in
			return &MyKelasResult{Items: []MyKelasItem{}, Total: 0}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), siswa)

	// page_size 0 -> 1, oversize -> 100
	doReq(t, app, "GET", "/siswa/kelas?page=0&page_size=999", nil)
	if captured.Limit != 100 {
		t.Fatalf("page_size cap failed: limit=%d", captured.Limit)
	}
	doReq(t, app, "GET", "/siswa/kelas?page=2&page_size=5", nil)
	if captured.Offset != 5 {
		t.Fatalf("page offset wrong: offset=%d want 5", captured.Offset)
	}
}
