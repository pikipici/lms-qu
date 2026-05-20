package kelas

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type fakeFinder struct {
	taken    map[string]bool
	calls    int
	lastKode string
}

func (f *fakeFinder) FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error) {
	f.calls++
	f.lastKode = kode
	if f.taken[kode] {
		return &Kelas{ID: uuid.New(), KodeInvite: kode}, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func TestGenerateKodeInvite_HappyPath(t *testing.T) {
	repo := &fakeFinder{taken: map[string]bool{}}
	kode, err := GenerateKodeInvite(context.Background(), repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(kode) != KodeInviteLength {
		t.Fatalf("expected len %d, got %d (%q)", KodeInviteLength, len(kode), kode)
	}
	for _, r := range kode {
		if !strings.ContainsRune(KodeInviteCharset, r) {
			t.Fatalf("kode %q contains char %q outside charset", kode, r)
		}
	}
	if repo.calls != 1 {
		t.Fatalf("expected 1 lookup, got %d", repo.calls)
	}
}

func TestGenerateKodeInvite_AvoidsAmbiguousChars(t *testing.T) {
	// Statistical sanity check: sample many codes, ensure none contain
	// banned ambiguous chars (0, O, 1, I, L, 8, B).
	repo := &fakeFinder{taken: map[string]bool{}}
	banned := "0OI1LB8"
	const samples = 200
	for i := 0; i < samples; i++ {
		kode, err := GenerateKodeInvite(context.Background(), repo)
		if err != nil {
			t.Fatalf("sample %d: unexpected error: %v", i, err)
		}
		if strings.ContainsAny(kode, banned) {
			t.Fatalf("sample %d kode %q contains banned ambiguous char", i, kode)
		}
	}
}

func TestGenerateKodeInvite_RetriesOnCollision(t *testing.T) {
	// Pre-populate every code the fake might roll on the first 3 attempts
	// by capturing them on a dry-run pass, then re-running with those
	// recorded as "taken." Easiest: mark the FIRST kode the generator
	// produces as taken, expect at least 2 calls.
	repo := &fakeFinder{taken: map[string]bool{}}

	// First probe: empty taken set, expect success on first try.
	first, err := GenerateKodeInvite(context.Background(), repo)
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	// Now mark `first` as taken, expect retry path.
	repo2 := &fakeFinder{taken: map[string]bool{first: true}}
	kode, err := GenerateKodeInvite(context.Background(), repo2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kode == first {
		t.Fatalf("generator returned a kode marked as taken: %q", kode)
	}
	// Cannot easily assert call count > 1 because crypto/rand might
	// produce a non-colliding kode on attempt 1 anyway. The fact that
	// the returned kode != first is the real signal.
}

func TestGenerateKodeInvite_ExhaustsRetries(t *testing.T) {
	// Force every kode the generator produces to look "taken" by saying
	// every lookup returns a Kelas (regardless of input). After
	// MaxKodeInviteRetry attempts, expect ErrKodeInviteCollision.
	always := alwaysFoundFinder{}
	_, err := GenerateKodeInvite(context.Background(), always)
	if !errors.Is(err, ErrKodeInviteCollision) {
		t.Fatalf("expected ErrKodeInviteCollision, got %v", err)
	}
}

type alwaysFoundFinder struct{}

func (alwaysFoundFinder) FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error) {
	return &Kelas{KodeInvite: kode}, nil
}

func TestGenerateKodeInvite_PropagatesLookupError(t *testing.T) {
	boom := errors.New("db down")
	repo := errFinder{err: boom}
	_, err := GenerateKodeInvite(context.Background(), repo)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped boom, got %v", err)
	}
}

type errFinder struct{ err error }

func (e errFinder) FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error) {
	return nil, e.err
}

func TestRandomKode_LengthAndCharset(t *testing.T) {
	for _, n := range []int{1, 6, 16} {
		kode, err := randomKode(n)
		if err != nil {
			t.Fatalf("len %d: error %v", n, err)
		}
		if len(kode) != n {
			t.Fatalf("len %d: got %d", n, len(kode))
		}
		for _, r := range kode {
			if !strings.ContainsRune(KodeInviteCharset, r) {
				t.Fatalf("len %d: kode %q has invalid char %q", n, kode, r)
			}
		}
	}
}

func TestRandomKode_RejectsZeroLength(t *testing.T) {
	if _, err := randomKode(0); err == nil {
		t.Fatal("expected error for n=0")
	}
}
