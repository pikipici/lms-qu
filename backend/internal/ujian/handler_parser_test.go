package ujian

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func TestParseSource(t *testing.T) {
	id := uuid.New()
	manual, err := parseSource(&sourceRequest{Mode: string(SourceManual), SoalIDs: []string{" " + id.String() + " "}})
	if err != nil {
		t.Fatal(err)
	}
	if manual == nil || manual.Manual == nil || len(manual.Manual.SoalIDs) != 1 || manual.Manual.SoalIDs[0] != id {
		t.Fatalf("manual source mismatch: %+v", manual)
	}

	random, err := parseSource(&sourceRequest{Mode: string(SourceRandom), Filter: &filterDTO{Mapel: "IPA", Tingkat: "7", Topik: "Bab 1"}, JumlahSoal: 12})
	if err != nil {
		t.Fatal(err)
	}
	if random == nil || random.Random == nil || random.Random.Filter.Mapel != "IPA" || random.Random.JumlahSoal != 12 {
		t.Fatalf("random source mismatch: %+v", random)
	}

	nilSource, err := parseSource(nil)
	if err != nil || nilSource != nil {
		t.Fatalf("nil source = %+v err=%v", nilSource, err)
	}
}

func TestParseSourceErrors(t *testing.T) {
	tests := []struct {
		name string
		req  sourceRequest
		want error
	}{
		{name: "manual invalid uuid", req: sourceRequest{Mode: string(SourceManual), SoalIDs: []string{"bad"}}, want: errInvalidSourceSoalID},
		{name: "missing mode", req: sourceRequest{}, want: errSourceMissingMode},
		{name: "invalid mode", req: sourceRequest{Mode: "bank"}, want: errSourceInvalidMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSource(&tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v want %v", err, tt.want)
			}
		})
	}
}

func TestParseOptionalRFC3339(t *testing.T) {
	if got, err := parseOptionalRFC3339(nil); err != nil || got != nil {
		t.Fatalf("nil parse got=%v err=%v", got, err)
	}
	empty := "  "
	if got, err := parseOptionalRFC3339(&empty); err != nil || got != nil {
		t.Fatalf("empty parse got=%v err=%v", got, err)
	}
	raw := "2026-05-23T10:00:00.123+07:00"
	got, err := parseOptionalRFC3339(&raw)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse(time.RFC3339Nano, raw)
	if got == nil || !got.Equal(want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
	bad := "23-05-2026"
	if _, err := parseOptionalRFC3339(&bad); err == nil {
		t.Fatal("invalid timestamp should error")
	}
}

func TestMapServiceErr(t *testing.T) {
	app := fiber.New()
	cases := []struct {
		path   string
		err    error
		status int
		code   string
	}{
		{"/bank", ErrSoalNotInBank, fiber.StatusBadRequest, "soal_not_in_bank"},
		{"/empty", ErrSoalEmpty, fiber.StatusBadRequest, "source_pool_empty"},
		{"/missing", ErrSourceMissing, fiber.StatusBadRequest, "source_missing"},
		{"/invalid", ErrInvalidInput, fiber.StatusBadRequest, "invalid_body"},
		{"/attempts", ErrAttemptsExist, fiber.StatusConflict, "attempts_exist"},
		{"/active", ErrActiveAttemptsBlock, fiber.StatusConflict, "active_attempts_block"},
		{"/archived", ErrKelasArchived, fiber.StatusConflict, "kelas_archived"},
		{"/not-found", ErrNotFound, fiber.StatusNotFound, "not_found"},
		{"/forbidden", ErrForbidden, fiber.StatusForbidden, "forbidden"},
		{"/version", ErrVersionConflict, fiber.StatusConflict, "version_conflict"},
		{"/internal", errors.New("boom"), fiber.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		errToMap := tc.err
		app.Get(tc.path, func(c *fiber.Ctx) error { return mapServiceErr(c, errToMap) })
	}
	for _, tc := range cases {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, tc.path, nil))
		if err != nil {
			t.Fatal(err)
		}
		assertUjianErrorCode(t, resp, tc.status, tc.code)
		resp.Body.Close()
	}
}

func TestFriendlyMessage(t *testing.T) {
	if friendlyMessage(nil, "fallback") != "fallback" {
		t.Fatal("nil should use fallback")
	}
	if got := friendlyMessage(errors.New("wrap: invalid input: judul required"), "fallback"); got != "judul required" {
		t.Fatalf("got %q", got)
	}
	if got := friendlyMessage(errors.New("plain"), "fallback"); got != "fallback" {
		t.Fatalf("got %q", got)
	}
}

func assertUjianErrorCode(t *testing.T, resp *http.Response, status int, code string) {
	t.Helper()
	if resp.StatusCode != status {
		t.Fatalf("status = %d want %d", resp.StatusCode, status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"code":"`+code+`"`) {
		t.Fatalf("body %s missing code %s", string(body), code)
	}
}
