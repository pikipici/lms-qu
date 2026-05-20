package kelas

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func mustCreateKelas(t *testing.T, svc *Service, repo *mockRepo, guruID uuid.UUID) *Kelas {
	t.Helper()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "Mat 7A"}, "", "")
	if err != nil {
		t.Fatalf("seed kelas: %v", err)
	}
	if k == nil {
		t.Fatal("seed kelas: nil result")
	}
	if _, ok := repo.rows[k.ID]; !ok {
		t.Fatal("seed kelas: not stored in mock repo")
	}
	return k
}

func TestService_JoinByKode_HappyPath(t *testing.T) {
	svc, repo, audit := newSvc(t)
	k := mustCreateKelas(t, svc, repo, uuid.New())
	siswa := uuid.New()

	res, err := svc.JoinByKode(context.Background(), siswa, JoinByKodeInput{
		KodeInvite: strings.ToLower(k.KodeInvite), // tahan input lowercase
	}, "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !res.Inserted {
		t.Fatal("expected Inserted=true on first join")
	}
	if res.Kelas.ID != k.ID {
		t.Fatalf("kelas id mismatch: %s != %s", res.Kelas.ID, k.ID)
	}

	// Audit recorded
	var found bool
	for _, e := range audit.entries {
		if e.Action == "siswa_joined_kelas" {
			found = true
			if e.TargetKelasID == nil || *e.TargetKelasID != k.ID {
				t.Fatal("target_kelas_id missing/wrong on audit row")
			}
			if e.ActorRole == nil || *e.ActorRole != "siswa" {
				t.Fatal("actor_role should be siswa")
			}
		}
	}
	if !found {
		t.Fatal("expected siswa_joined_kelas audit entry")
	}
}

func TestService_JoinByKode_Idempotent(t *testing.T) {
	svc, repo, _ := newSvc(t)
	k := mustCreateKelas(t, svc, repo, uuid.New())
	siswa := uuid.New()

	if _, err := svc.JoinByKode(context.Background(), siswa, JoinByKodeInput{KodeInvite: k.KodeInvite}, "", ""); err != nil {
		t.Fatal(err)
	}
	res, err := svc.JoinByKode(context.Background(), siswa, JoinByKodeInput{KodeInvite: k.KodeInvite}, "", "")
	if err != nil {
		t.Fatalf("idempotent join should not error: %v", err)
	}
	if res.Inserted {
		t.Fatal("second join must report Inserted=false")
	}
}

func TestService_JoinByKode_RejectsEmpty(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.JoinByKode(context.Background(), uuid.New(), JoinByKodeInput{KodeInvite: "   "}, "", "")
	if !errors.Is(err, ErrKodeInviteEmpty) {
		t.Fatalf("expected ErrKodeInviteEmpty, got %v", err)
	}
}

func TestService_JoinByKode_NotFound(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.JoinByKode(context.Background(), uuid.New(), JoinByKodeInput{KodeInvite: "ZZZZZZ"}, "", "")
	if !errors.Is(err, ErrKodeInviteNotFound) {
		t.Fatalf("expected ErrKodeInviteNotFound, got %v", err)
	}
}

func TestService_JoinByKode_RejectsArchivedKelas(t *testing.T) {
	svc, repo, _ := newSvc(t)
	k := mustCreateKelas(t, svc, repo, uuid.New())
	now := time.Now()
	repo.rows[k.ID].ArchivedAt = &now

	_, err := svc.JoinByKode(context.Background(), uuid.New(), JoinByKodeInput{KodeInvite: k.KodeInvite}, "", "")
	if !errors.Is(err, ErrKelasArchived) {
		t.Fatalf("expected ErrKelasArchived, got %v", err)
	}
}

func TestService_JoinByKode_RejectsRemovedSiswa(t *testing.T) {
	svc, repo, _ := newSvc(t)
	k := mustCreateKelas(t, svc, repo, uuid.New())
	siswa := uuid.New()
	repo.enrollments[enrollKey(k.ID, siswa)] = &Enrollment{
		KelasID: k.ID, SiswaID: siswa, Status: EnrollmentRemoved, JoinedVia: JoinedViaKode,
	}

	_, err := svc.JoinByKode(context.Background(), siswa, JoinByKodeInput{KodeInvite: k.KodeInvite}, "", "")
	if !errors.Is(err, ErrEnrollmentRemoved) {
		t.Fatalf("expected ErrEnrollmentRemoved, got %v", err)
	}
}
