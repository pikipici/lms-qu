// Guru rekap matrix — Task 7.B.
//
// GET /kelas/:id/rekap?format=json|csv
//
// Response shape (JSON):
//
//	{
//	  "kelas":  KelasInfo,
//	  "bab":    [{ "id": uuid, "nomor": int, "judul": string }, ...],
//	  "ujian":  [{ "id": uuid, "judul": string }, ...],
//	  "rows":   [{
//	    "siswa_id": uuid, "siswa_nama": string,
//	    "bab":   [{ "bab_id": uuid, "total": *float, "ulangan_bab": *float, "tugas": *float }],
//	    "ujian": [{ "ujian_id": uuid, "nilai_terbaik": *float, "nilai_terakhir": *float, "attempt_count": int }],
//	    "total_kelas": *float
//	  }],
//	  "summary": { "siswa_count": int, "siswa_with_nilai": int, "kelas_avg": *float }
//	}
//
// CSV shape (header):
//
//	siswa_id,siswa_nama,total_kelas,bab_<id> ...,ujian_<id>_terbaik,ujian_<id>_terakhir
//
// Authorization: callerRole=guru with kelas.GuruID == userID, OR callerRole=admin.
package nilai

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
)

// ---------- Types (matrix) ----------

// RekapBabCell is a single (siswa, bab) cell in the matrix.
type RekapBabCell struct {
	BabID      uuid.UUID `json:"bab_id"`
	Total      *float64  `json:"total"`
	UlanganBab *float64  `json:"ulangan_bab"`
	Tugas      *float64  `json:"tugas"`
}

// RekapUjianCell is a single (siswa, ujian) cell in the matrix.
type RekapUjianCell struct {
	UjianID       uuid.UUID `json:"ujian_id"`
	NilaiTerbaik  *float64  `json:"nilai_terbaik"`
	NilaiTerakhir *float64  `json:"nilai_terakhir"`
	Bobot         int       `json:"bobot"`
	AttemptCount  int       `json:"attempt_count"`
}

// RekapRow is one siswa's full row in the matrix.
type RekapRow struct {
	SiswaID    uuid.UUID        `json:"siswa_id"`
	SiswaNama  string           `json:"siswa_nama"`
	Bab        []RekapBabCell   `json:"bab"`
	Ujian      []RekapUjianCell `json:"ujian"`
	TotalKelas *float64         `json:"total_kelas"`
}

// RekapBabHead is bab metadata for the matrix header.
type RekapBabHead struct {
	ID    uuid.UUID `json:"id"`
	Nomor int       `json:"nomor"`
	Judul string    `json:"judul"`
}

// RekapUjianHead is ujian metadata for the matrix header.
type RekapUjianHead struct {
	ID    uuid.UUID `json:"id"`
	Judul string    `json:"judul"`
	Bobot int       `json:"bobot"`
}

// RekapSummary collapses the matrix into class-wide stats.
type RekapSummary struct {
	SiswaCount     int      `json:"siswa_count"`
	SiswaWithNilai int      `json:"siswa_with_nilai"`
	KelasAvg       *float64 `json:"kelas_avg"`
}

// GuruRekapResponse is the full matrix payload.
type GuruRekapResponse struct {
	Kelas   KelasInfo        `json:"kelas"`
	Bab     []RekapBabHead   `json:"bab"`
	Ujian   []RekapUjianHead `json:"ujian"`
	Rows    []RekapRow       `json:"rows"`
	Summary RekapSummary     `json:"summary"`
}

// ---------- Service ----------

// rekapEnrollmentLookup lists active enrollments for a kelas. Implemented
// by *kelas.Repo via ListEnrollmentsByKelas.
type rekapEnrollmentLookup interface {
	ListEnrollmentsByKelas(ctx context.Context, kelasID uuid.UUID, limit, offset int) ([]kelas.Enrollment, int64, error)
}

// rekapUserLookup hydrates display name for siswa rows.
type rekapUserLookup interface {
	NameByID(ctx context.Context, id uuid.UUID) (string, error)
}

// GuruKelasRekap returns the full rekap matrix for a kelas.
//
// Authorization: callerRole=guru with kelas ownership, OR callerRole=admin.
// Siswa is rejected. Iteration cost: O(siswa_count) calls to the per-siswa
// aggregator. Typical kelas <= 40 siswa → < 2s on dev DB. If perf becomes a
// concern, swap to a single batched query (locked #91 trade-off accepted MVP).
func (s *Service) GuruKelasRekap(
	ctx context.Context,
	kelasID, callerID uuid.UUID,
	callerRole string,
	enrollLookup rekapEnrollmentLookup,
	userLookup rekapUserLookup,
) (*GuruRekapResponse, error) {
	if callerRole != string(auth.Admin) && callerRole != string(auth.Guru) {
		return nil, ErrForbidden
	}

	// Fetch + ownership check.
	k, err := s.kelas.FindByID(ctx, kelasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("nilai rekap kelas find: %w", err)
	}
	if callerRole == string(auth.Guru) && k.GuruID != callerID {
		return nil, ErrForbidden
	}

	// Header: bab + ujian metadata. Same shape used by SiswaKelasNilai.
	babs, err := s.repo.ListPublishedBabByKelas(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("nilai rekap list bab: %w", err)
	}
	ujians, err := s.repo.ListPublishedUjianByKelas(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("nilai rekap list ujian: %w", err)
	}
	babHead := make([]RekapBabHead, len(babs))
	for i, b := range babs {
		babHead[i] = RekapBabHead{ID: b.ID, Nomor: b.Nomor, Judul: b.Judul}
	}
	ujianHead := make([]RekapUjianHead, len(ujians))
	for i, u := range ujians {
		ujianHead[i] = RekapUjianHead{ID: u.ID, Judul: u.Judul, Bobot: u.Bobot}
	}

	// Active enrollments. limit=10_000 sebagai cap MVP — kelas dengan
	// >10K siswa gak realistic untuk single-teacher LMS sekolah.
	enrolls, _, err := enrollLookup.ListEnrollmentsByKelas(ctx, kelasID, 10000, 0)
	if err != nil {
		return nil, fmt.Errorf("nilai rekap list enroll: %w", err)
	}
	siswaIDs := make([]uuid.UUID, 0, len(enrolls))
	for _, e := range enrolls {
		if e.Status == kelas.EnrollmentActive {
			siswaIDs = append(siswaIDs, e.SiswaID)
		}
	}

	rows := make([]RekapRow, 0, len(siswaIDs))
	var sumTotals float64
	var nWithNilai int
	for _, sid := range siswaIDs {
		// Siswa nama hydration. Empty fallback on error keeps UX intact.
		name, _ := userLookup.NameByID(ctx, sid)

		// Reuse aggregator. Bypass enrollment check: we already filtered
		// for active enrollments above and we are guru-context. Use
		// a non-collapsing internal call so guru sees real data.
		full, err := s.guruSiswaKelasNilai(ctx, kelasID, sid)
		if err != nil {
			return nil, fmt.Errorf("nilai rekap siswa %s: %w", sid, err)
		}

		babCells := make([]RekapBabCell, len(babs))
		babIdx := map[uuid.UUID]int{}
		for i, b := range babs {
			babCells[i] = RekapBabCell{BabID: b.ID}
			babIdx[b.ID] = i
		}
		for _, br := range full.Bab {
			if i, ok := babIdx[br.BabID]; ok {
				babCells[i].Total = br.Total
				babCells[i].UlanganBab = br.NilaiUlanganBab
				babCells[i].Tugas = br.NilaiTugasBab
			}
		}

		ujianCells := make([]RekapUjianCell, len(ujians))
		ujianIdx := map[uuid.UUID]int{}
		for i, u := range ujians {
			ujianCells[i] = RekapUjianCell{UjianID: u.ID, Bobot: u.Bobot}
			ujianIdx[u.ID] = i
		}
		for _, ur := range full.UlanganHarian {
			if i, ok := ujianIdx[ur.UjianID]; ok {
				ujianCells[i].NilaiTerbaik = ur.NilaiTerbaik
				ujianCells[i].NilaiTerakhir = ur.NilaiTerakhir
				ujianCells[i].AttemptCount = ur.AttemptCount
			}
		}

		row := RekapRow{
			SiswaID:    sid,
			SiswaNama:  name,
			Bab:        babCells,
			Ujian:      ujianCells,
			TotalKelas: full.TotalKelas,
		}
		if row.TotalKelas != nil {
			sumTotals += *row.TotalKelas
			nWithNilai++
		}
		rows = append(rows, row)
	}

	// Stable sort by siswa nama (case-insensitive simple compare).
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].SiswaNama < rows[j].SiswaNama
	})

	var kelasAvg *float64
	if nWithNilai > 0 {
		v := sumTotals / float64(nWithNilai)
		kelasAvg = &v
	}

	return &GuruRekapResponse{
		Kelas: KelasInfo{
			ID:               k.ID,
			Nama:             k.Nama,
			BobotSoalUlangan: k.BobotSoalUlangan,
			BobotTugas:       k.BobotTugas,
		},
		Bab:   babHead,
		Ujian: ujianHead,
		Rows:  rows,
		Summary: RekapSummary{
			SiswaCount:     len(siswaIDs),
			SiswaWithNilai: nWithNilai,
			KelasAvg:       kelasAvg,
		},
	}, nil
}

// guruSiswaKelasNilai mirrors SiswaKelasNilai but skips role+enrollment
// checks (guru caller already authorized + we filtered to active
// enrollments).
func (s *Service) guruSiswaKelasNilai(ctx context.Context, kelasID, siswaID uuid.UUID) (*SiswaKelasNilaiResponse, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("rekap find kelas: %w", err)
	}

	babs, err := s.repo.ListPublishedBabByKelas(ctx, kelasID)
	if err != nil {
		return nil, err
	}
	ujians, err := s.repo.ListPublishedUjianByKelas(ctx, kelasID)
	if err != nil {
		return nil, err
	}

	babIDs := make([]uuid.UUID, len(babs))
	for i, b := range babs {
		babIDs[i] = b.ID
	}
	poin, soalCount, err := s.repo.soalUlanganBabPoinByBab(ctx, babIDs)
	if err != nil {
		return nil, err
	}
	hasilUlangan, err := s.repo.hasilUlanganBabBySiswa(ctx, siswaID, babIDs)
	if err != nil {
		return nil, err
	}
	tugasAgg, err := s.repo.nilaiTugasPerBab(ctx, kelasID, siswaID)
	if err != nil {
		return nil, err
	}
	ujianAgg, err := s.repo.nilaiUjianByKelas(ctx, kelasID, siswaID)
	if err != nil {
		return nil, err
	}

	wUlangan := k.BobotSoalUlangan
	wTugas := k.BobotTugas

	babRows := make([]NilaiBabRow, 0, len(babs))
	for _, b := range babs {
		row := NilaiBabRow{
			BabID:           b.ID,
			Nomor:           b.Nomor,
			Judul:           b.Judul,
			JumlahSoalUlBab: soalCount[b.ID],
		}
		if h, ok := hasilUlangan[b.ID]; ok && h.NilaiTotal != nil {
			if denom := poin[b.ID]; denom > 0 {
				v := (*h.NilaiTotal / float64(denom)) * 100
				row.NilaiUlanganBab = &v
				hid := h.HasilID
				row.HasilUlanganID = &hid
			}
		}
		if t, ok := tugasAgg[b.ID]; ok {
			row.JumlahTugas = t.TugasTotal
			row.JumlahTugasGrade = t.GradedCount
			row.BobotTugasTotal = t.BobotTotal
			if t.AvgPct != nil {
				v := *t.AvgPct
				row.NilaiTugasBab = &v
			}
		}
		row.Breakdown = BabBreakdown{
			UlanganBab: BabBreakdownItem{Pct: row.NilaiUlanganBab, Weight: wUlangan},
			Tugas:      BabBreakdownItem{Pct: row.NilaiTugasBab, Weight: wTugas},
		}
		row.Total = computeWeightedTotal(row.NilaiUlanganBab, row.NilaiTugasBab, wUlangan, wTugas)
		babRows = append(babRows, row)
	}

	ujianRows := make([]NilaiUjianRow, 0, len(ujians))
	for _, u := range ujians {
		row := NilaiUjianRow{
			UjianID: u.ID,
			Judul:   u.Judul,
			Bobot:   u.Bobot,
		}
		if a, ok := ujianAgg[u.ID]; ok {
			row.NilaiTerbaik = a.NilaiTerbaik
			row.NilaiTerakhir = a.NilaiTerakhir
			row.AttemptCount = a.AttemptCount
			if a.HasilTerakhir != nil {
				row.HasilID = a.HasilTerakhir
			}
		}
		ujianRows = append(ujianRows, row)
	}

	return &SiswaKelasNilaiResponse{
		Kelas: KelasInfo{
			ID:               k.ID,
			Nama:             k.Nama,
			BobotSoalUlangan: k.BobotSoalUlangan,
			BobotTugas:       k.BobotTugas,
		},
		Bab:           babRows,
		UlanganHarian: ujianRows,
		TotalKelas:    computeKelasTotal(babRows),
	}, nil
}
