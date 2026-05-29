package chat

import (
	"strings"
	"testing"
)

func TestNormalizeBody(t *testing.T) {
	got, err := normalizeBody("  halo pak  ")
	if err != nil {
		t.Fatalf("normalizeBody() error = %v", err)
	}
	if got != "halo pak" {
		t.Fatalf("normalizeBody() = %q", got)
	}

	if _, err := normalizeBody("   "); err != ErrInvalidBody {
		t.Fatalf("empty body error = %v, want ErrInvalidBody", err)
	}

	if _, err := normalizeBody(strings.Repeat("a", messageMaxLen+1)); err != ErrInvalidBody {
		t.Fatalf("too long body error = %v, want ErrInvalidBody", err)
	}
}

func TestMakePreview(t *testing.T) {
	body := strings.Repeat("x", previewMaxLen+5)
	got := makePreview(body)
	if len([]rune(got)) != previewMaxLen {
		t.Fatalf("preview length = %d, want %d", len([]rune(got)), previewMaxLen)
	}
}

func TestClampLimit(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{name: "default", in: 0, want: 20},
		{name: "value", in: 30, want: 30},
		{name: "max", in: 200, want: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampLimit(tc.in, 20, 100); got != tc.want {
				t.Fatalf("clampLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}
