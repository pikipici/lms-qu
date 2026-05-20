package importjob

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestParse_HappyPath(t *testing.T) {
	in := `nama,email,kode_kelas
Andi Pratama,andi@contoh.id,KLS-001
Budi Santoso,budi@contoh.id,
Citra Lestari,citra@contoh.id,KLS-002
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Total != 3 {
		t.Fatalf("Total = %d, want 3", got.Stats.Total)
	}
	if got.Stats.Valid != 3 {
		t.Fatalf("Valid = %d, want 3", got.Stats.Valid)
	}
	if got.Stats.Invalid != 0 || got.Stats.Duplicates != 0 {
		t.Fatalf("unexpected invalid/dup: %+v", got.Stats)
	}
	want0 := Row{LineNo: 2, Nama: "Andi Pratama", Email: "andi@contoh.id", KodeKelas: "KLS-001", Status: RowValid}
	if got.Rows[0] != want0 {
		t.Fatalf("row[0] = %+v, want %+v", got.Rows[0], want0)
	}
	// Empty kode normalized to "".
	if got.Rows[1].KodeKelas != "" {
		t.Fatalf("row[1].KodeKelas = %q, want empty", got.Rows[1].KodeKelas)
	}
}

func TestParse_HeaderAliases(t *testing.T) {
	cases := []string{
		"name,email\nAndi,andi@a.id\n",
		"Nama,E-Mail\nAndi,andi@a.id\n",
		"nama_lengkap,alamat_email\nAndi,andi@a.id\n",
		"FullName,EMAIL\nAndi,andi@a.id\n",
	}
	for i, in := range cases {
		got, err := Parse(strings.NewReader(in))
		if err != nil {
			t.Errorf("case %d: Parse: %v", i, err)
			continue
		}
		if got.Stats.Valid != 1 {
			t.Errorf("case %d: Valid=%d, want 1", i, got.Stats.Valid)
		}
	}
}

func TestParse_KodeAliases(t *testing.T) {
	cases := []string{
		"nama,email,kode\nAndi,a@b.id,KLS-1\n",
		"nama,email,kode_invite\nAndi,a@b.id,KLS-1\n",
		"nama,email,invite_code\nAndi,a@b.id,KLS-1\n",
	}
	for i, in := range cases {
		got, err := Parse(strings.NewReader(in))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if got.Rows[0].KodeKelas != "KLS-1" {
			t.Errorf("case %d: KodeKelas = %q, want KLS-1", i, got.Rows[0].KodeKelas)
		}
	}
}

func TestParse_NormalizesEmailAndKode(t *testing.T) {
	in := `nama,email,kode
Andi,  ANDI@Example.ID  ,kls-abc
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	r := got.Rows[0]
	if r.Email != "andi@example.id" {
		t.Errorf("Email = %q, want lowercased+trimmed andi@example.id", r.Email)
	}
	if r.KodeKelas != "KLS-ABC" {
		t.Errorf("KodeKelas = %q, want uppercased KLS-ABC", r.KodeKelas)
	}
	if r.Status != RowValid {
		t.Errorf("Status = %s, want valid", r.Status)
	}
}

func TestParse_TolerateBOM(t *testing.T) {
	in := "\xEF\xBB\xBFnama,email\nAndi,andi@a.id\n"
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Valid != 1 {
		t.Fatalf("Valid = %d, want 1", got.Stats.Valid)
	}
}

func TestParse_SemicolonDelimiter(t *testing.T) {
	in := `nama;email;kode_kelas
Andi;andi@a.id;KLS-1
Budi;budi@a.id;KLS-2
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Valid != 2 {
		t.Fatalf("Valid = %d, want 2", got.Stats.Valid)
	}
	if got.Rows[0].Email != "andi@a.id" {
		t.Errorf("delimiter detection failed: %+v", got.Rows[0])
	}
}

func TestParse_DedupeByEmail(t *testing.T) {
	in := `nama,email
Andi,andi@a.id
Andi Lain,ANDI@a.id
Budi,budi@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Total != 3 {
		t.Fatalf("Total = %d, want 3", got.Stats.Total)
	}
	if got.Stats.Valid != 2 {
		t.Fatalf("Valid = %d, want 2 (first occurrence + budi)", got.Stats.Valid)
	}
	if got.Stats.Duplicates != 1 {
		t.Fatalf("Duplicates = %d, want 1", got.Stats.Duplicates)
	}
	if got.Rows[1].Status != RowDuplicate {
		t.Fatalf("row[1].Status = %s, want duplicate", got.Rows[1].Status)
	}
	if !strings.Contains(strings.Join(got.Rows[1].Errors, " "), "duplikat dengan baris 2") {
		t.Errorf("dup err msg should reference original line: %v", got.Rows[1].Errors)
	}
}

func TestParse_InvalidEmail(t *testing.T) {
	in := `nama,email
Andi,not-an-email
Budi,budi@a.id
,empty@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Valid != 1 || got.Stats.Invalid != 2 {
		t.Fatalf("counts = %+v, want valid=1 invalid=2", got.Stats)
	}
	// Row 0 invalid (bad email).
	if got.Rows[0].Status != RowInvalid {
		t.Errorf("row[0].Status = %s, want invalid", got.Rows[0].Status)
	}
	// Row 2 invalid (empty nama).
	if got.Rows[2].Status != RowInvalid {
		t.Errorf("row[2].Status = %s, want invalid", got.Rows[2].Status)
	}
}

func TestParse_BlankRowsTolerated(t *testing.T) {
	in := `nama,email
Andi,andi@a.id
,,
Budi,budi@a.id

`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Total != 2 || got.Stats.Valid != 2 {
		t.Fatalf("counts = %+v, want total=2 valid=2", got.Stats)
	}
}

func TestParse_LongFields(t *testing.T) {
	longNama := strings.Repeat("a", MaxNamaLen+1)
	longKode := strings.Repeat("X", MaxKodeKelasLen+1)
	in := fmt.Sprintf("nama,email,kode\n%s,a@b.id,KLS-1\nAndi,a2@b.id,%s\n", longNama, longKode)
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Valid != 0 {
		t.Errorf("Valid = %d, want 0", got.Stats.Valid)
	}
	if got.Stats.Invalid != 2 {
		t.Errorf("Invalid = %d, want 2", got.Stats.Invalid)
	}
}

func TestParse_LineNoIncludesHeader(t *testing.T) {
	in := `nama,email
Andi,andi@a.id
Budi,budi@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Rows[0].LineNo != 2 {
		t.Errorf("first data row LineNo = %d, want 2", got.Rows[0].LineNo)
	}
	if got.Rows[1].LineNo != 3 {
		t.Errorf("second data row LineNo = %d, want 3", got.Rows[1].LineNo)
	}
}

func TestParse_MissingNamaColumn(t *testing.T) {
	in := `email
a@b.id
`
	_, err := Parse(strings.NewReader(in))
	if !errors.Is(err, ErrMissingNamaColumn) {
		t.Fatalf("err = %v, want wraps ErrMissingNamaColumn", err)
	}
}

func TestParse_MissingEmailColumn(t *testing.T) {
	in := `nama
Andi
`
	_, err := Parse(strings.NewReader(in))
	if !errors.Is(err, ErrMissingEmailColumn) {
		t.Fatalf("err = %v, want wraps ErrMissingEmailColumn", err)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\n\n"} {
		_, err := Parse(strings.NewReader(in))
		if !errors.Is(err, ErrEmptyCSV) {
			t.Errorf("Parse(%q) err = %v, want wraps ErrEmptyCSV", in, err)
		}
	}
}

func TestParse_HeaderOnly(t *testing.T) {
	in := `nama,email
`
	_, err := Parse(strings.NewReader(in))
	if !errors.Is(err, ErrEmptyCSV) {
		t.Fatalf("Parse(header only) err = %v, want wraps ErrEmptyCSV", err)
	}
}

func TestParse_TooLarge(t *testing.T) {
	// Build a CSV that exceeds MaxCSVBytes by 1.
	header := "nama,email\n"
	row := "Andi,andi@a.id\n"
	var buf bytes.Buffer
	buf.WriteString(header)
	for buf.Len() < MaxCSVBytes+1 {
		buf.WriteString(row)
	}
	_, err := Parse(&buf)
	if !errors.Is(err, ErrCSVTooLarge) {
		t.Fatalf("err = %v, want wraps ErrCSVTooLarge", err)
	}
}

func TestParse_TooManyRows(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("nama,email\n")
	for i := 0; i < MaxCSVRows+1; i++ {
		fmt.Fprintf(&buf, "Andi%d,andi%d@a.id\n", i, i)
	}
	_, err := Parse(&buf)
	if !errors.Is(err, ErrTooManyRows) {
		t.Fatalf("err = %v, want wraps ErrTooManyRows", err)
	}
}

func TestParse_InvalidUTF8(t *testing.T) {
	// Latin-1 encoded "Café" — invalid UTF-8.
	in := []byte("nama,email\nCaf\xe9,a@b.id\n")
	_, err := Parse(bytes.NewReader(in))
	if !errors.Is(err, ErrInvalidUTF8) {
		t.Fatalf("err = %v, want wraps ErrInvalidUTF8", err)
	}
}

func TestParse_RaggedRowKeptAsInvalid(t *testing.T) {
	// LazyQuotes + FieldsPerRecord=-1 means a quote-mid-field row should
	// still parse; a missing column simply yields empty values.
	in := `nama,email,kode
Andi,andi@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// 1 row, valid (kode optional).
	if got.Stats.Total != 1 || got.Stats.Valid != 1 {
		t.Fatalf("counts = %+v", got.Stats)
	}
}

func TestParse_OptionalKodeAbsentColumn(t *testing.T) {
	in := `nama,email
Andi,andi@a.id
Budi,budi@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Stats.Valid != 2 {
		t.Fatalf("Valid = %d, want 2", got.Stats.Valid)
	}
	for _, r := range got.Rows {
		if r.KodeKelas != "" {
			t.Errorf("KodeKelas = %q, want empty when column absent", r.KodeKelas)
		}
	}
}

func TestParse_StableLineNoOnInvalidThenDup(t *testing.T) {
	// First "andi" row is INVALID (missing nama). Second "andi" row should
	// then be VALID (not Duplicate), because invalids don't claim emails.
	in := `nama,email
,andi@a.id
Andi,andi@a.id
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Rows[0].Status != RowInvalid {
		t.Errorf("row[0].Status = %s, want invalid", got.Rows[0].Status)
	}
	if got.Rows[1].Status != RowValid {
		t.Errorf("row[1].Status = %s, want valid (invalid does not claim email)", got.Rows[1].Status)
	}
}

// Sanity: confirm the package keeps compiling with the io.Reader contract.
var _ io.Reader = (*bytes.Reader)(nil)
