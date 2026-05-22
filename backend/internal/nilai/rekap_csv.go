// CSV encoder for guru rekap matrix (Task 7.B). Header layout:
//
//	siswa_id,siswa_nama,total_kelas,
//	bab_<id>_total,bab_<id>_ulangan,bab_<id>_tugas, ...,
//	ujian_<id>_terbaik,ujian_<id>_terakhir,ujian_<id>_attempt
//
// Float cells round to 1 decimal. NULL cells emitted as empty string.
// Filename hint: "rekap-<kelasID>.csv".
package nilai

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

func formatFloatCSV(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%.1f", *v)
}

// EncodeRekapCSV writes the matrix as RFC4180 CSV to w.
func EncodeRekapCSV(w io.Writer, r *GuruRekapResponse) error {
	cw := csv.NewWriter(w)

	header := []string{"siswa_id", "siswa_nama", "total_kelas"}
	for _, b := range r.Bab {
		base := fmt.Sprintf("bab_%d_%s", b.Nomor, sanitizeCSVHeaderID(b.ID.String()))
		header = append(header, base+"_total", base+"_ulangan", base+"_tugas")
	}
	for _, u := range r.Ujian {
		base := "ujian_" + sanitizeCSVHeaderID(u.ID.String())
		header = append(header, base+"_terbaik", base+"_terakhir", base+"_attempt")
	}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, row := range r.Rows {
		out := []string{row.SiswaID.String(), row.SiswaNama, formatFloatCSV(row.TotalKelas)}
		for _, c := range row.Bab {
			out = append(out, formatFloatCSV(c.Total), formatFloatCSV(c.UlanganBab), formatFloatCSV(c.Tugas))
		}
		for _, c := range row.Ujian {
			out = append(out, formatFloatCSV(c.NilaiTerbaik), formatFloatCSV(c.NilaiTerakhir), fmt.Sprintf("%d", c.AttemptCount))
		}
		if err := cw.Write(out); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// sanitizeCSVHeaderID keeps the UUID compact in CSV headers — strips
// hyphens, lowercases, takes first 8 chars (collision-tolerant for 1
// kelas; full UUID still in row data via siswa_id column anyway).
func sanitizeCSVHeaderID(id string) string {
	s := strings.ReplaceAll(id, "-", "")
	if len(s) > 8 {
		s = s[:8]
	}
	return s
}
