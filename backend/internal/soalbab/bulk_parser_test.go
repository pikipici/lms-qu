package soalbab

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSplitBulkLines(t *testing.T) {
	lines := splitBulkLines("one\r\ntwo\nthree\r")
	if len(lines) != 3 {
		t.Fatalf("len = %d", len(lines))
	}
	if lines[0].Number != 1 || lines[0].Raw != "one" {
		t.Fatalf("line 1 = %+v", lines[0])
	}
	if lines[1].Number != 2 || lines[1].Raw != "two" {
		t.Fatalf("line 2 = %+v", lines[1])
	}
	if lines[2].Number != 3 || lines[2].Raw != "three" {
		t.Fatalf("line 3 = %+v", lines[2])
	}
	if splitBulkLines("") != nil {
		t.Fatal("empty body should return nil")
	}
}

func TestSplitEscapedPipe(t *testing.T) {
	got := splitEscapedPipe(`a\|b|c\\d|e\x`)
	want := []string{"a|b", `c\d`, `e\x`}
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("col %d = %q want %q", i, got[i], want[i])
		}
	}
}

func TestParseBulkLine(t *testing.T) {
	babID := uuid.New()
	kelasID := uuid.New()
	callerID := uuid.New()

	soal, reason := parseBulkLine(`Apa ibu kota Indonesia?|Jakarta|Bandung|Surabaya|Medan|Makassar|a|10|ulangan`, ModeKeduanya, babID, kelasID, callerID)
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
	if soal.BabID != babID || soal.KelasID != kelasID || soal.CreatedByID != callerID {
		t.Fatalf("ids mismatch: %+v", soal)
	}
	if soal.Pertanyaan != "Apa ibu kota Indonesia?" || soal.Jawaban != JawabanA || soal.Poin != 10 || soal.Mode != ModeUlangan || soal.Version != 1 {
		t.Fatalf("soal mismatch: %+v", soal)
	}

	soal, reason = parseBulkLine(`Q|A|B|C|D|E|b||`, ModeLatihan, babID, kelasID, callerID)
	if reason != "" || soal.Poin != 1 || soal.Mode != ModeLatihan {
		t.Fatalf("default parse soal=%+v reason=%q", soal, reason)
	}
}

func TestParseBulkLineReasons(t *testing.T) {
	babID := uuid.New()
	kelasID := uuid.New()
	callerID := uuid.New()
	longQuestion := strings.Repeat("x", MaxPertanyaanBytes+1)
	longOption := strings.Repeat("x", MaxOpsiBytes+1)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "invalid columns", raw: `too|few`, want: ReasonInvalidColumns},
		{name: "empty pertanyaan", raw: ` |A|B|C|D|E|a|1|latihan`, want: ReasonEmptyPertanyaan},
		{name: "pertanyaan too long", raw: longQuestion + `|A|B|C|D|E|a|1|latihan`, want: ReasonPertanyaanTooLong},
		{name: "opsi too long", raw: `Q|` + longOption + `|B|C|D|E|a|1|latihan`, want: ReasonOpsiTooLong},
		{name: "invalid jawaban", raw: `Q|A|B|C|D|E|z|1|latihan`, want: ReasonInvalidJawaban},
		{name: "invalid poin", raw: `Q|A|B|C|D|E|a|101|latihan`, want: ReasonInvalidPoin},
		{name: "invalid mode", raw: `Q|A|B|C|D|E|a|1|quiz`, want: ReasonInvalidMode},
		{name: "empty selected option", raw: `Q|A|B|C|D| |e|1|latihan`, want: ReasonEmptyJawabanOption},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, reason := parseBulkLine(tt.raw, ModeKeduanya, babID, kelasID, callerID)
			if reason != tt.want {
				t.Fatalf("reason = %q want %q", reason, tt.want)
			}
		})
	}
}
