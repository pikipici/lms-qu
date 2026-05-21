// Cross-kelas siswa submission listing (Task 4.D.2).
//
// Surfaces "Tugas Saya" — semua submission siswa lintas kelas untuk
// dashboard riwayat. JOIN-backed (lihat repo.ListBySiswaWithTugas) supaya
// FE gak perlu fetch tugas + kelas per row.
package submission

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MySubmissionItem hydrates one submission + tugas snapshot untuk siswa
// dashboard. Mirror minimum fields siswa butuhkan untuk row.
type MySubmissionItem struct {
	SubmissionID         uuid.UUID  `json:"submission_id"`
	TugasID              uuid.UUID  `json:"tugas_id"`
	KelasID              uuid.UUID  `json:"kelas_id"`
	BabID                *uuid.UUID `json:"bab_id,omitempty"`
	Judul                string     `json:"judul"`
	Deadline             *time.Time `json:"deadline,omitempty"`
	IzinkanLate          bool       `json:"izinkan_late"`
	PenaltyPersen        int16      `json:"penalty_persen"`
	Status               Status     `json:"status"`
	IsLate               bool       `json:"is_late"`
	NilaiAsli            *float64   `json:"nilai_asli,omitempty"`
	PenaltyPersenApplied *int16     `json:"penalty_persen_applied,omitempty"`
	NilaiSetelahPenalty  *float64   `json:"nilai_setelah_penalty,omitempty"`
	Feedback             string     `json:"feedback"`
	GradedAt             *time.Time `json:"graded_at,omitempty"`
	SubmittedAt          time.Time  `json:"submitted_at"`
	Version              int        `json:"version"`
}

// DefaultMineLimit caps per-request page size when caller doesn't specify.
const DefaultMineLimit = 100

// MaxMineLimit clamps the upper bound to keep payload bounded.
const MaxMineLimit = 500

// ListMine returns ALL submissions belonging to the calling siswa, joined
// with tugas snapshot, ordered by submitted_at DESC. Caller (handler)
// already guards role=siswa via RoleGuard.
//
// limit <=0 → DefaultMineLimit. limit > MaxMineLimit → clamp.
func (s *Service) ListMine(ctx context.Context, siswaID uuid.UUID, limit int) ([]MySubmissionItem, error) {
	if limit <= 0 {
		limit = DefaultMineLimit
	}
	if limit > MaxMineLimit {
		limit = MaxMineLimit
	}
	rows, err := s.repo.ListBySiswaWithTugas(ctx, siswaID, limit)
	if err != nil {
		return nil, fmt.Errorf("submission list mine: %w", err)
	}
	out := make([]MySubmissionItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, MySubmissionItem{
			SubmissionID:         r.Submission.ID,
			TugasID:              r.TugasID,
			KelasID:              r.KelasID,
			BabID:                r.BabID,
			Judul:                r.Judul,
			Deadline:             r.Deadline,
			IzinkanLate:          r.IzinkanLate,
			PenaltyPersen:        r.PenaltyPersen,
			Status:               r.Submission.Status,
			IsLate:               r.Submission.IsLate,
			NilaiAsli:            r.Submission.NilaiAsli,
			PenaltyPersenApplied: r.Submission.PenaltyPersenApplied,
			NilaiSetelahPenalty:  r.Submission.NilaiSetelahPenalty,
			Feedback:             r.Submission.Feedback,
			GradedAt:             r.Submission.GradedAt,
			SubmittedAt:          r.Submission.SubmittedAt,
			Version:              r.Submission.Version,
		})
	}
	return out, nil
}
