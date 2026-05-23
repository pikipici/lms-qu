package nilai

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFormatFloatCSV(t *testing.T) {
	if got := formatFloatCSV(nil); got != "" {
		t.Fatalf("formatFloatCSV(nil) = %q, want empty", got)
	}
	if got := formatFloatCSV(fptr(87.65)); got != "87.7" {
		t.Fatalf("formatFloatCSV(87.65) = %q, want 87.7", got)
	}
}

func TestSanitizeCSVHeaderID(t *testing.T) {
	got := sanitizeCSVHeaderID("ABCDEF12-3456-7890-abcd-ef1234567890")
	if got != "ABCDEF12" {
		t.Fatalf("sanitizeCSVHeaderID() = %q, want ABCDEF12", got)
	}
	if got := sanitizeCSVHeaderID("short"); got != "short" {
		t.Fatalf("sanitizeCSVHeaderID(short) = %q", got)
	}
}

func TestEncodeRekapCSV(t *testing.T) {
	babID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ujianID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	siswaID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	res := &GuruRekapResponse{
		Bab:   []RekapBabHead{{ID: babID, Nomor: 2, Judul: "Bab Dua"}},
		Ujian: []RekapUjianHead{{ID: ujianID, Judul: "UH 1"}},
		Rows: []RekapRow{{
			SiswaID:    siswaID,
			SiswaNama:  "Siswa, Pakai Koma",
			TotalKelas: fptr(88.44),
			Bab:        []RekapBabCell{{BabID: babID, Total: fptr(90), UlanganBab: nil, Tugas: fptr(80.25)}},
			Ujian:      []RekapUjianCell{{UjianID: ujianID, NilaiTerbaik: fptr(95), NilaiTerakhir: fptr(92.2), AttemptCount: 2}},
		}},
	}

	var buf bytes.Buffer
	if err := EncodeRekapCSV(&buf, res); err != nil {
		t.Fatalf("EncodeRekapCSV() error = %v", err)
	}

	records, err := csv.NewReader(strings.NewReader(buf.String())).ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error = %v\n%s", err, buf.String())
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}

	wantHeader := []string{
		"siswa_id", "siswa_nama", "total_kelas",
		"bab_2_11111111_total", "bab_2_11111111_ulangan", "bab_2_11111111_tugas",
		"ujian_22222222_terbaik", "ujian_22222222_terakhir", "ujian_22222222_attempt",
	}
	assertCSVRecord(t, records[0], wantHeader)
	assertCSVRecord(t, records[1], []string{
		siswaID.String(), "Siswa, Pakai Koma", "88.4", "90.0", "", "80.2", "95.0", "92.2", "2",
	})
}

func assertCSVRecord(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("record len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record[%d] = %q, want %q\nrecord=%#v", i, got[i], want[i], got)
		}
	}
}
