package kelas

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
)

func TestHandler_JoinByKode_HappyPath201(t *testing.T) {
	siswa := uuid.New()
	kelasID := uuid.New()
	svc := &stubSvc{
		joinFn: func(ctx context.Context, sID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
			if sID != siswa {
				t.Fatalf("siswa id mismatch")
			}
			if strings.TrimSpace(in.KodeInvite) == "" {
				t.Fatal("handler should not pass empty kode")
			}
			return &JoinByKodeResult{
				Kelas:    &Kelas{ID: kelasID, Nama: "Matematika 7A", KodeInvite: "ABC234", Version: 1},
				Inserted: true,
			}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), siswa)

	resp, body := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": "abc234"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", resp.StatusCode, body)
	}
	var parsed joinByKodeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !parsed.Inserted {
		t.Fatal("expected inserted=true")
	}
	if parsed.Kelas == nil || parsed.Kelas.ID != kelasID {
		t.Fatal("kelas not echoed back")
	}
}

func TestHandler_JoinByKode_Idempotent200(t *testing.T) {
	siswa := uuid.New()
	svc := &stubSvc{
		joinFn: func(ctx context.Context, sID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
			return &JoinByKodeResult{
				Kelas:    &Kelas{ID: uuid.New(), Nama: "X", KodeInvite: "WXYZ23"},
				Inserted: false,
			}, nil
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), siswa)

	resp, _ := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": "WXYZ23"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 idempotent, got %d", resp.StatusCode)
	}
}

func TestHandler_JoinByKode_EmptyBody400(t *testing.T) {
	app := newApp(t, &Handler{svc: &stubSvc{}}, string(auth.Siswa), uuid.New())

	resp, body := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "kode_invite_required") {
		t.Fatalf("expected kode_invite_required error code, got %s", body)
	}
}

func TestHandler_JoinByKode_NotFound404(t *testing.T) {
	svc := &stubSvc{
		joinFn: func(ctx context.Context, sID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
			return nil, ErrKodeInviteNotFound
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), uuid.New())

	resp, body := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": "BADCOD"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "kode_invite_not_found") {
		t.Fatalf("expected kode_invite_not_found code, got %s", body)
	}
}

func TestHandler_JoinByKode_Archived409(t *testing.T) {
	svc := &stubSvc{
		joinFn: func(ctx context.Context, sID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
			return nil, ErrKelasArchived
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), uuid.New())

	resp, body := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": "ABC234"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "kelas_archived") {
		t.Fatalf("expected kelas_archived code, got %s", body)
	}
}

func TestHandler_JoinByKode_Removed409(t *testing.T) {
	svc := &stubSvc{
		joinFn: func(ctx context.Context, sID uuid.UUID, in JoinByKodeInput, ip, ua string) (*JoinByKodeResult, error) {
			return nil, ErrEnrollmentRemoved
		},
	}
	app := newApp(t, &Handler{svc: svc}, string(auth.Siswa), uuid.New())

	resp, body := doReq(t, app, http.MethodPost, "/siswa/kelas/join", map[string]any{"kode_invite": "ABC234"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "enrollment_removed") {
		t.Fatalf("expected enrollment_removed code, got %s", body)
	}
}
