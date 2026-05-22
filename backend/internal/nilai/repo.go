// Repo: nilai aggregator queries (read-only, locked #90).
//
// All queries scoped per (kelas_id, siswa_id) — caller MUST verify
// enrollment before calling. Single round-trip per shape, no N+1.
package nilai

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Repo provides read-only nilai aggregations.
type Repo struct {
	db *gorm.DB
}

// NewRepo wires the GORM DB into the nilai repo.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// babMeta holds the static bab info needed for the per-kelas matrix.
type babMeta struct {
	ID    uuid.UUID `gorm:"column:id"`
	Nomor int       `gorm:"column:nomor"`
	Judul string    `gorm:"column:judul"`
}

// ListPublishedBabByKelas returns published bab metadata for nilai
// rendering, sorted by urutan ASC, nomor ASC.
func (r *Repo) ListPublishedBabByKelas(ctx context.Context, kelasID uuid.UUID) ([]babMeta, error) {
	var rows []babMeta
	err := r.db.WithContext(ctx).
		Table("bab").
		Select("id, nomor, judul").
		Where("kelas_id = ? AND status = 'published'", kelasID).
		Order("urutan ASC, nomor ASC").
		Scan(&rows).Error
	return rows, err
}

// ujianMeta holds Ujian (Ulangan Harian) info per kelas.
type ujianMeta struct {
	ID    uuid.UUID `gorm:"column:id"`
	Judul string    `gorm:"column:judul"`
}

// ListPublishedUjianByKelas returns Ujian (Ulangan Harian) metadata
// status=published only, sorted by created_at DESC.
func (r *Repo) ListPublishedUjianByKelas(ctx context.Context, kelasID uuid.UUID) ([]ujianMeta, error) {
	var rows []ujianMeta
	err := r.db.WithContext(ctx).
		Table("ujian").
		Select("id, judul").
		Where("kelas_id = ? AND status = 'published'", kelasID).
		Order("created_at DESC").
		Scan(&rows).Error
	return rows, err
}

// soalUlanganBabPoinByBab maps bab_id → SUM(poin) of soal eligible
// for ulangan flow (mode IN ('ulangan','keduanya')). Used as the
// denominator for nilai_ulangan_bab normalize-to-100.
func (r *Repo) soalUlanganBabPoinByBab(ctx context.Context, babIDs []uuid.UUID) (map[uuid.UUID]int, map[uuid.UUID]int, error) {
	if len(babIDs) == 0 {
		return map[uuid.UUID]int{}, map[uuid.UUID]int{}, nil
	}
	type row struct {
		BabID    uuid.UUID `gorm:"column:bab_id"`
		SumPoin  int       `gorm:"column:sum_poin"`
		Count    int       `gorm:"column:count"`
	}
	var rows []row
	err := r.db.WithContext(ctx).
		Table("soal_bab").
		Select("bab_id, COALESCE(SUM(poin),0) AS sum_poin, COUNT(*) AS count").
		Where("bab_id IN ? AND mode IN ('ulangan','keduanya')", babIDs).
		Group("bab_id").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	poin := make(map[uuid.UUID]int, len(rows))
	cnt := make(map[uuid.UUID]int, len(rows))
	for _, x := range rows {
		poin[x.BabID] = x.SumPoin
		cnt[x.BabID] = x.Count
	}
	return poin, cnt, nil
}

// hasilUlanganBabPerBab fetches the latest finished hasil for the
// (siswa, bab) pair where mode='ulangan' AND status='selesai'. Returns
// raw nilai_total (not normalized) + hasil_id for FE drilldown.
type hasilRow struct {
	BabID      uuid.UUID `gorm:"column:bab_id"`
	HasilID    uuid.UUID `gorm:"column:id"`
	NilaiTotal *float64  `gorm:"column:nilai_total"`
}

func (r *Repo) hasilUlanganBabBySiswa(ctx context.Context, siswaID uuid.UUID, babIDs []uuid.UUID) (map[uuid.UUID]hasilRow, error) {
	out := map[uuid.UUID]hasilRow{}
	if len(babIDs) == 0 {
		return out, nil
	}
	// One row per bab — pick the latest selesai_at via DISTINCT ON.
	var rows []hasilRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT DISTINCT ON (bab_id) bab_id, id, nilai_total
		FROM hasil_soal_bab
		WHERE siswa_id = ?
		  AND bab_id IN (?)
		  AND mode = 'ulangan'
		  AND status = 'selesai'
		ORDER BY bab_id, COALESCE(selesai_at, mulai_at) DESC, created_at DESC
	`, siswaID, babIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.BabID] = x
	}
	return out, nil
}

// nilaiTugasPerBab computes per-bab tugas average (NilaiSetelahPenalty
// / MaxNilai × 100) over submission status='graded'. Returns map
// bab_id → (avg_pct, count_graded, count_tugas_total).
type tugasAggRow struct {
	BabID       uuid.UUID `gorm:"column:bab_id"`
	AvgPct      *float64  `gorm:"column:avg_pct"`
	GradedCount int       `gorm:"column:graded_count"`
	TugasTotal  int       `gorm:"column:tugas_total"`
}

func (r *Repo) nilaiTugasPerBab(ctx context.Context, kelasID, siswaID uuid.UUID) (map[uuid.UUID]tugasAggRow, error) {
	out := map[uuid.UUID]tugasAggRow{}
	var rows []tugasAggRow
	// LEFT JOIN dari tugas → submission (siswa) supaya bab tanpa
	// submission masih punya tugas_total > 0 (penting untuk re-normalize).
	err := r.db.WithContext(ctx).Raw(`
		SELECT t.bab_id,
		       AVG(CASE WHEN s.status = 'graded' AND s.nilai_setelah_penalty IS NOT NULL AND t.max_nilai > 0
		                THEN (s.nilai_setelah_penalty::float / t.max_nilai::float) * 100
		           END) AS avg_pct,
		       COUNT(CASE WHEN s.status = 'graded' THEN 1 END) AS graded_count,
		       COUNT(*) AS tugas_total
		FROM tugas t
		LEFT JOIN submission s ON s.tugas_id = t.id AND s.siswa_id = ?
		WHERE t.kelas_id = ?
		  AND t.bab_id IS NOT NULL
		GROUP BY t.bab_id
	`, siswaID, kelasID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("nilai tugas per bab: %w", err)
	}
	for _, x := range rows {
		out[x.BabID] = x
	}
	return out, nil
}

// ujianAggRow holds per-ujian best/last + attempt count for a siswa.
type ujianAggRow struct {
	UjianID        uuid.UUID  `gorm:"column:ujian_id"`
	NilaiTerbaik   *float64   `gorm:"column:nilai_terbaik"`
	NilaiTerakhir  *float64   `gorm:"column:nilai_terakhir"`
	AttemptCount   int        `gorm:"column:attempt_count"`
	HasilTerakhir  *uuid.UUID `gorm:"column:hasil_terakhir_id"`
}

// nilaiUjianByKelas aggregates Ujian (ulangan harian) results per
// (ujian_id) for one siswa in a kelas. Status='selesai' only;
// dibatalkan/berlangsung skipped.
func (r *Repo) nilaiUjianByKelas(ctx context.Context, kelasID, siswaID uuid.UUID) (map[uuid.UUID]ujianAggRow, error) {
	out := map[uuid.UUID]ujianAggRow{}
	var rows []ujianAggRow
	err := r.db.WithContext(ctx).Raw(`
		WITH attempts AS (
			SELECT h.ujian_id,
			       h.id,
			       h.nilai_total,
			       COALESCE(h.selesai_at, h.mulai_at) AS at,
			       ROW_NUMBER() OVER (PARTITION BY h.ujian_id ORDER BY COALESCE(h.selesai_at, h.mulai_at) DESC) AS rn_last
			FROM hasil_ujian h
			JOIN ujian u ON u.id = h.ujian_id
			WHERE u.kelas_id = ?
			  AND h.siswa_id = ?
			  AND h.status = 'selesai'
			  AND h.deleted_at IS NULL
		)
		SELECT ujian_id,
		       MAX(nilai_total) AS nilai_terbaik,
		       MAX(CASE WHEN rn_last = 1 THEN nilai_total END) AS nilai_terakhir,
		       MAX(CASE WHEN rn_last = 1 THEN id END) AS hasil_terakhir_id,
		       COUNT(*) AS attempt_count
		FROM attempts
		GROUP BY ujian_id
	`, kelasID, siswaID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("nilai ujian per kelas: %w", err)
	}
	for _, x := range rows {
		out[x.UjianID] = x
	}
	return out, nil
}

// crossKelasRow is one row per kelas that the siswa is enrolled in.
type crossKelasRow struct {
	KelasID   uuid.UUID `gorm:"column:kelas_id"`
	KelasNama string    `gorm:"column:kelas_nama"`
	GuruNama  string    `gorm:"column:guru_nama"`
}

// ListEnrolledKelas returns kelas the siswa is actively enrolled in,
// sorted by kelas nama ASC. Used by GET /siswa/nilai (cross-class).
func (r *Repo) ListEnrolledKelas(ctx context.Context, siswaID uuid.UUID) ([]crossKelasRow, error) {
	var rows []crossKelasRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT k.id AS kelas_id, k.nama AS kelas_nama, COALESCE(u.name, '') AS guru_nama
		FROM enrollment e
		JOIN kelas k ON k.id = e.kelas_id
		LEFT JOIN users u ON u.id = k.guru_id
		WHERE e.siswa_id = ?
		  AND e.status = 'active'
		  AND k.archived_at IS NULL
		ORDER BY k.nama ASC
	`, siswaID).Scan(&rows).Error
	return rows, err
}
