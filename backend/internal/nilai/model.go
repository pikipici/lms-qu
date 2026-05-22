// Package nilai computes per-siswa NilaiBab + NilaiUlanganHarian aggregates
// at-query-time (locked #90 — read-only, no persisted denormalize).
//
// Formula references:
//   - Section 6.2 NilaiBab = weighted_avg(NilaiUlanganBab, NilaiTugasBab,
//     weights = (Kelas.BobotSoalUlangan, Kelas.BobotTugas), skip NULL).
//   - NilaiUlanganBab = TotalNilai HasilSoalBab(mode=ulangan, deleted_at IS
//     NULL, status=selesai) terbaru per (BabID, SiswaID), normalize ke 0..100
//     pakai SUM SoalBab.Poin (mode=ulangan,keduanya).
//   - NilaiTugasBab = AVG NilaiSetelahPenalty / MaxNilai × 100 untuk
//     submission status=graded di tugas yang BabID=bab tsb.
//   - NilaiUlanganHarian = TotalNilai HasilUjian(deleted_at IS NULL,
//     status=selesai) terbaik per (UjianID, SiswaID).
//
// NULL handling (locked #48): komponen tanpa konten di-skip + bobot
// di-redistribute. Bab kosong total → return null (FE render "—").
package nilai

import (
	"github.com/google/uuid"
)

// BabBreakdownItem captures a single nilai component's value (0..100).
// Pct nullable: "—" / not-yet-graded.
type BabBreakdownItem struct {
	Pct    *float64 `json:"pct"`
	Weight int      `json:"w"`
}

// BabBreakdown is the per-bab nilai breakdown.
type BabBreakdown struct {
	UlanganBab BabBreakdownItem `json:"ulangan_bab"`
	Tugas      BabBreakdownItem `json:"tugas"`
}

// NilaiBabRow is one bab's nilai for a siswa.
type NilaiBabRow struct {
	BabID            uuid.UUID    `json:"bab_id"`
	Nomor            int          `json:"nomor"`
	Judul            string       `json:"judul"`
	NilaiUlanganBab  *float64     `json:"nilai_ulangan_bab"`
	NilaiTugasBab    *float64     `json:"nilai_tugas_bab"`
	Total            *float64     `json:"total"`
	Breakdown        BabBreakdown `json:"breakdown"`
	JumlahTugas      int          `json:"jumlah_tugas"`
	JumlahTugasGrade int          `json:"jumlah_tugas_dinilai"`
	JumlahSoalUlBab  int          `json:"jumlah_soal_ulangan_bab"`
	HasilUlanganID   *uuid.UUID   `json:"hasil_ulangan_id,omitempty"`
}

// NilaiUjianRow is one ulangan-harian (Ujian) attempt aggregate.
type NilaiUjianRow struct {
	UjianID       uuid.UUID  `json:"ujian_id"`
	Judul         string     `json:"judul"`
	NilaiTerbaik  *float64   `json:"nilai_terbaik"`
	NilaiTerakhir *float64   `json:"nilai_terakhir"`
	AttemptCount  int        `json:"attempt_count"`
	HasilID       *uuid.UUID `json:"hasil_id,omitempty"`
}

// KelasInfo embeds the kelas header used in nilai responses.
type KelasInfo struct {
	ID               uuid.UUID `json:"id"`
	Nama             string    `json:"nama"`
	BobotSoalUlangan int       `json:"bobot_soal_ulangan"`
	BobotTugas       int       `json:"bobot_tugas"`
}

// SiswaKelasNilaiResponse is the shape of GET /siswa/kelas/:id/nilai.
type SiswaKelasNilaiResponse struct {
	Kelas         KelasInfo       `json:"kelas"`
	Bab           []NilaiBabRow   `json:"bab"`
	UlanganHarian []NilaiUjianRow `json:"ulangan_harian"`
	TotalKelas    *float64        `json:"total_kelas"`
}

// SiswaKelasSummary is a single kelas card on cross-class siswa list.
type SiswaKelasSummary struct {
	KelasID       uuid.UUID `json:"kelas_id"`
	KelasNama     string    `json:"kelas_nama"`
	GuruNama      string    `json:"guru_nama"`
	TotalKelas    *float64  `json:"total_kelas"`
	BabCount      int       `json:"bab_count"`
	UlanganCount  int       `json:"ulangan_count"`
}

// SiswaListResponse is the shape of GET /siswa/nilai (cross-class).
type SiswaListResponse struct {
	Items []SiswaKelasSummary `json:"items"`
}

// computeWeightedTotal applies weighted average with NULL-skip
// re-normalization (locked #48 + Section 6.2). Returns nil if all
// components are nil OR total weight is zero.
func computeWeightedTotal(ulangan, tugas *float64, wUlangan, wTugas int) *float64 {
	var num float64
	var den int
	if ulangan != nil && wUlangan > 0 {
		num += *ulangan * float64(wUlangan)
		den += wUlangan
	}
	if tugas != nil && wTugas > 0 {
		num += *tugas * float64(wTugas)
		den += wTugas
	}
	if den == 0 {
		return nil
	}
	v := num / float64(den)
	return &v
}

// computeKelasTotal averages across non-null bab totals — bab kosong
// (total=nil) skipped. Returns nil if no bab has a graded component.
func computeKelasTotal(rows []NilaiBabRow) *float64 {
	var sum float64
	var n int
	for _, r := range rows {
		if r.Total != nil {
			sum += *r.Total
			n++
		}
	}
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}
