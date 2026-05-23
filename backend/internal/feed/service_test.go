package feed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
)

func TestCursorRoundTrip(t *testing.T) {
	at := time.Date(2026, 5, 23, 10, 11, 12, 123456000, time.FixedZone("WIB", 7*60*60))
	id := "event-42"

	cursor := encodeCursor(at, id)
	gotAt, gotID, err := decodeCursor(cursor)
	if err != nil {
		t.Fatalf("decodeCursor() error = %v", err)
	}
	if !gotAt.Equal(at.UTC()) || gotID != id {
		t.Fatalf("decodeCursor() = (%s, %q), want (%s, %q)", gotAt, gotID, at.UTC(), id)
	}
}

func TestDecodeCursorAcceptsStandardBase64(t *testing.T) {
	at := time.UnixMicro(123456789).UTC()
	cursor := "MTIzNDU2Nzg5OnN0ZC1pZA=="

	gotAt, gotID, err := decodeCursor(cursor)
	if err != nil {
		t.Fatalf("decodeCursor() error = %v", err)
	}
	if !gotAt.Equal(at) || gotID != "std-id" {
		t.Fatalf("decodeCursor() = (%s, %q), want (%s, std-id)", gotAt, gotID, at)
	}
}

func TestDecodeCursorRejectsMalformedValues(t *testing.T) {
	tests := []string{
		"not-base64$$$",
		"bm8tc2VwYXJhdG9y",
		"bm90LW51bWJlcjppZA",
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			_, _, err := decodeCursor(tc)
			if err == nil {
				t.Fatalf("decodeCursor(%q) error = nil, want error", tc)
			}
		})
	}
}

func TestServiceListRejectsNonGuruAdminRoles(t *testing.T) {
	svc := NewService(nil)

	_, err := svc.List(context.Background(), uuid.New(), string(auth.Siswa), "", 20)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("List() error = %v, want ErrForbidden", err)
	}
}

func TestServiceListRejectsInvalidCursorBeforeRepoQuery(t *testing.T) {
	svc := NewService(NewRepo(nil))

	_, err := svc.List(context.Background(), uuid.New(), string(auth.Guru), "invalid-cursor", 20)
	if !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("List() error = %v, want ErrInvalidCursor", err)
	}
}
