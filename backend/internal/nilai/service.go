// Service: nilai aggregator orchestration.
//
// All entry points are caller-role-aware (siswa vs guru) but Fase 7.A
// only ships siswa-facing endpoints. Guru rekap matrix (7.B) reuses
// the same Repo + helpers via a separate service entry.
package nilai

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors. Handler maps to HTTP status codes.
var (
	// ErrForbidden — caller role/ownership/enrollment failed.
	ErrForbidden = errors.New("forbidden")
	// ErrNotFound — kelas tidak ada (collapsed to forbidden for siswa
	// to avoid existence probing).
	ErrNotFound = errors.New("not_found")
)

// kelasLookup hydrates kelas (no ownership check).
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// enrollmentLookup verifies the siswa is actively enrolled.
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// Service wraps nilai-aggregator queries with auth + enrollment checks.
type Service struct {
	repo   *Repo
	kelas  kelasLookup
	enroll enrollmentLookup
}

// NewService wires the nilai service.
func NewService(r *Repo, k kelasLookup, e enrollmentLookup) *Service {
	return &Service{repo: r, kelas: k, enroll: e}
}

// SiswaKelasNilai returns the per-kelas nilai bundle for a siswa.
//
// Authorization: callerRole must be siswa, and siswaID must be actively
// enrolled. Failures collapse to ErrForbidden (no probing).
func (s *Service) SiswaKelasNilai(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) (*SiswaKelasNilaiResponse, error) {
	if callerRole != string(auth.Siswa) {
		return nil, ErrForbidden
	}

	// Fetch kelas. Missing → forbidden (collapse).
	k, err := s.kelas.FindByID(ctx, kelasID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
		return nil, fmt.Errorf("nilai siswa kelas find: %w", err)
	}

	// Verify enrollment.
	enr, err := s.enroll.FindEnrollment(ctx, kelasID, siswaID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
		return nil, fmt.Errorf("nilai siswa enrollment: %w", err)
	}
	if enr == nil || enr.Status != kelas.EnrollmentActive {
		return nil, ErrForbidden
	}

	// Bab + ujian metadata.
	babs, err := s.repo.ListPublishedBabByKelas(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("nilai siswa list bab: %w", err)
	}
	ujians, err := s.repo.ListPublishedUjianByKelas(ctx, kelasID)
	if err != nil {
		return nil, fmt.Errorf("nilai siswa list ujian: %w", err)
	}

	babIDs := make([]uuid.UUID, len(babs))
	for i, b := range babs {
		babIDs[i] = b.ID
	}
	poin, soalCount, err := s.repo.soalUlanganBabPoinByBab(ctx, babIDs)
	if err != nil {
		return nil, fmt.Errorf("nilai siswa soal poin: %w", err)
	}
	hasilUlangan, err := s.repo.hasilUlanganBabBySiswa(ctx, siswaID, babIDs)
	if err != nil {
		return nil, fmt.Errorf("nilai siswa hasil ulangan: %w", err)
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
		// Breakdown surfaces both even when nil so FE renders placeholders.
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

// SiswaList returns one-card-per-kelas summary for the cross-class
// /siswa/nilai page.
func (s *Service) SiswaList(ctx context.Context, siswaID uuid.UUID, callerRole string) (*SiswaListResponse, error) {
	if callerRole != string(auth.Siswa) {
		return nil, ErrForbidden
	}
	rows, err := s.repo.ListEnrolledKelas(ctx, siswaID)
	if err != nil {
		return nil, fmt.Errorf("nilai siswa list kelas: %w", err)
	}
	out := make([]SiswaKelasSummary, 0, len(rows))
	for _, r := range rows {
		// Reuse SiswaKelasNilai for total — small N kelas typical (<10),
		// per-kelas aggregator < 50ms each. If perf becomes an issue,
		// swap to a single batched query.
		full, err := s.SiswaKelasNilai(ctx, r.KelasID, siswaID, callerRole)
		if err != nil {
			// Forbidden mid-list shouldn't happen (we just listed
			// active enrollments). Skip silently to keep UX intact.
			if errors.Is(err, ErrForbidden) {
				continue
			}
			return nil, err
		}
		out = append(out, SiswaKelasSummary{
			KelasID:      r.KelasID,
			KelasNama:    r.KelasNama,
			GuruNama:     r.GuruNama,
			TotalKelas:   full.TotalKelas,
			BabCount:     len(full.Bab),
			UlanganCount: len(full.UlanganHarian),
		})
	}
	return &SiswaListResponse{Items: out}, nil
}
