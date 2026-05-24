package nilai

import "testing"

func fptr(v float64) *float64 { return &v }

func TestComputeWeightedTotal(t *testing.T) {
	tests := []struct {
		name     string
		ulangan  *float64
		tugas    *float64
		wUlangan int
		wTugas   int
		want     *float64
	}{
		{name: "legacy class weights no longer used by callers", ulangan: fptr(80), tugas: fptr(100), wUlangan: 1, wTugas: 1, want: fptr(90)},
		{name: "weighted helper still supports explicit weights", ulangan: fptr(80), tugas: fptr(100), wUlangan: 60, wTugas: 40, want: fptr(88)},
		{name: "skip nil ulangan and renormalize", ulangan: nil, tugas: fptr(75), wUlangan: 60, wTugas: 40, want: fptr(75)},
		{name: "skip nil tugas and renormalize", ulangan: fptr(90), tugas: nil, wUlangan: 60, wTugas: 40, want: fptr(90)},
		{name: "zero weight component skipped", ulangan: fptr(20), tugas: fptr(90), wUlangan: 0, wTugas: 100, want: fptr(90)},
		{name: "all nil returns nil", ulangan: nil, tugas: nil, wUlangan: 60, wTugas: 40, want: nil},
		{name: "all zero weights returns nil", ulangan: fptr(80), tugas: fptr(90), wUlangan: 0, wTugas: 0, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeWeightedTotal(tt.ulangan, tt.tugas, tt.wUlangan, tt.wTugas)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("computeWeightedTotal() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("computeWeightedTotal() = nil, want %.2f", *tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("computeWeightedTotal() = %.2f, want %.2f", *got, *tt.want)
			}
		})
	}
}

func TestComputeKelasTotal(t *testing.T) {
	tests := []struct {
		name string
		rows []NilaiBabRow
		want *float64
	}{
		{name: "averages non nil totals", rows: []NilaiBabRow{{Total: fptr(80)}, {Total: nil}, {Total: fptr(100)}}, want: fptr(90)},
		{name: "all nil returns nil", rows: []NilaiBabRow{{Total: nil}, {Total: nil}}, want: nil},
		{name: "empty rows returns nil", rows: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeKelasTotal(tt.rows)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("computeKelasTotal() = %v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("computeKelasTotal() = nil, want %.2f", *tt.want)
			}
			if *got != *tt.want {
				t.Fatalf("computeKelasTotal() = %.2f, want %.2f", *got, *tt.want)
			}
		})
	}
}
