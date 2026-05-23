package submission

import (
	"errors"
	"math"
	"testing"

	"gorm.io/gorm"
)

func TestStatusValid(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusSubmitted, true},
		{StatusGraded, true},
		{StatusReturned, true},
		{Status("draft"), false},
		{Status(""), false},
	}
	for _, tt := range tests {
		if got := tt.status.Valid(); got != tt.want {
			t.Fatalf("Status(%q).Valid() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestRoundNilai(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"keeps integer", 90, 90},
		{"rounds half up", 89.995, 90},
		{"rounds down", 72.334, 72.33},
		{"rounds negative", -1.235, -1.24},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoundNilai(tt.in); math.Abs(got-tt.want) > 0.00001 {
				t.Fatalf("RoundNilai(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTableNames(t *testing.T) {
	if got := (Submission{}).TableName(); got != "submission" {
		t.Fatalf("Submission.TableName() = %q", got)
	}
	if got := (Attachment{}).TableName(); got != "submission_attachment" {
		t.Fatalf("Attachment.TableName() = %q", got)
	}
}

func TestSmallHelpers(t *testing.T) {
	if ptrString("") != nil {
		t.Fatal("ptrString empty should be nil")
	}
	if got := ptrString("client"); got == nil || *got != "client" {
		t.Fatalf("ptrString non-empty = %v", got)
	}

	if got := marshalMeta(nil); got != nil {
		t.Fatalf("marshalMeta(nil) = %s, want nil", string(got))
	}
	meta := marshalMeta(map[string]any{"status": "submitted", "count": 2})
	if len(meta) == 0 {
		t.Fatalf("marshalMeta returned empty json")
	}

	if !isNoRowsErr(gorm.ErrRecordNotFound) {
		t.Fatal("isNoRowsErr should accept gorm.ErrRecordNotFound")
	}
	if isNoRowsErr(errors.New("db down")) {
		t.Fatal("isNoRowsErr should reject non no-rows errors")
	}
}

func TestSanitizeAttachmentFilename(t *testing.T) {
	long := ""
	for i := 0; i < 210; i++ {
		long += "a"
	}
	tests := []struct {
		name string
		raw  string
		ext  string
		want string
	}{
		{"empty fallback", " ", "pdf", "submission.pdf"},
		{"base name only", "../report.pdf", "pdf", "report.pdf"},
		{"windows slash stripped", `folder\\report.docx`, "docx", `folderreport.docx`},
		{"dot fallback", ".", "zip", "submission.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeAttachmentFilename(tt.raw, tt.ext); got != tt.want {
				t.Fatalf("sanitizeAttachmentFilename(%q, %q) = %q, want %q", tt.raw, tt.ext, got, tt.want)
			}
		})
	}
	if got := sanitizeAttachmentFilename(long, "pdf"); len(got) != 200 {
		t.Fatalf("long filename len = %d, want 200", len(got))
	}
}
