// Package feed exposes the guru activity feed (Task 7.C, locked #39+#55).
//
// Single endpoint: GET /api/v1/guru/feed?cursor=<base64>&limit=20.
//
// At-query-time aggregator (mirror nilai #90): 3 sources UNION ALL with
// pagination by opaque cursor `(at_unix_micro DESC, id DESC)` base64.
//
//   - submission_baru:    SELECT submission s JOIN tugas t JOIN kelas k
//                         WHERE k.guru_id = ? (or admin: any).
//                         at = s.submitted_at; id = s.id.
//   - ulangan_selesai:    SELECT hasil_ujian h JOIN ujian u JOIN kelas k
//                         WHERE k.guru_id = ? AND h.status='selesai' AND
//                         h.deleted_at IS NULL.
//                         at = COALESCE(h.selesai_at, h.created_at); id = h.id.
//   - siswa_join:         SELECT enrollment e JOIN kelas k
//                         WHERE k.guru_id = ? AND e.status='active'.
//                         at = e.joined_at; id derived (kelas_id || siswa_id
//                         compact uuid).
//
// Locked #55: opaque cursor `(at_unix_micro DESC, id DESC)`. Default limit=20,
// max=50. Polling client gak send cursor untuk dapat slice latest.
package feed

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// EventKind enumerates the activity feed event types.
type EventKind string

const (
	// EventSubmissionBaru: siswa submit tugas baru.
	EventSubmissionBaru EventKind = "submission_baru"
	// EventUlanganSelesai: siswa selesai ulangan harian.
	EventUlanganSelesai EventKind = "ulangan_selesai"
	// EventSiswaJoin: siswa baru join kelas.
	EventSiswaJoin EventKind = "siswa_join"
)

// Event is a single activity feed item.
type Event struct {
	ID        string    `json:"id"`
	Kind      EventKind `json:"kind"`
	At        time.Time `json:"at"`
	KelasID   uuid.UUID `json:"kelas_id"`
	KelasNama string    `json:"kelas_nama"`
	SiswaID   uuid.UUID `json:"siswa_id"`
	SiswaNama string    `json:"siswa_nama"`
	// Submission-specific
	TugasID    *uuid.UUID `json:"tugas_id,omitempty"`
	TugasJudul string     `json:"tugas_judul,omitempty"`
	IsLate     *bool      `json:"is_late,omitempty"`
	// Ulangan-specific
	UjianID    *uuid.UUID `json:"ujian_id,omitempty"`
	UjianJudul string     `json:"ujian_judul,omitempty"`
	HasilID    *uuid.UUID `json:"hasil_id,omitempty"`
	NilaiTotal *float64   `json:"nilai_total,omitempty"`
}

// ListResponse is the feed pagination envelope.
type ListResponse struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"next_cursor"`
}

// Limits enforced server-side regardless of client request.
const (
	defaultLimit = 20
	maxLimit     = 50
)

// Sentinel errors mapped to HTTP at the handler.
var (
	// ErrForbidden — caller role rejected.
	ErrForbidden = errors.New("forbidden")
	// ErrInvalidCursor — cursor cannot be decoded.
	ErrInvalidCursor = errors.New("invalid_cursor")
)

// Repo holds the GORM DB for feed queries.
type Repo struct {
	db *gorm.DB
}

// NewRepo wires the GORM DB.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// rawEventRow is the union row pulled across 3 sources.
type rawEventRow struct {
	Kind       string     `gorm:"column:kind"`
	ID         string     `gorm:"column:id"`
	At         time.Time  `gorm:"column:at"`
	KelasID    uuid.UUID  `gorm:"column:kelas_id"`
	KelasNama  string     `gorm:"column:kelas_nama"`
	SiswaID    uuid.UUID  `gorm:"column:siswa_id"`
	SiswaNama  string     `gorm:"column:siswa_nama"`
	TugasID    *uuid.UUID `gorm:"column:tugas_id"`
	TugasJudul *string    `gorm:"column:tugas_judul"`
	IsLate     *bool      `gorm:"column:is_late"`
	UjianID    *uuid.UUID `gorm:"column:ujian_id"`
	UjianJudul *string    `gorm:"column:ujian_judul"`
	HasilID    *uuid.UUID `gorm:"column:hasil_id"`
	NilaiTotal *float64   `gorm:"column:nilai_total"`
}

// listForGuru runs the UNION ALL query. guruScope=true → filter by guruID
// (k.guru_id == guruID); guruScope=false → admin all.
func (r *Repo) listForGuru(ctx context.Context, guruID uuid.UUID, guruScope bool, beforeAt time.Time, beforeID string, limit int) ([]rawEventRow, error) {
	// SQL: cursor predicate is `(at, id) < (beforeAt, beforeID)` over
	// strict ordering DESC. We pass beforeAt at very-future when cursor
	// is empty (caller responsibility).
	scope := "AND k.guru_id = ?"
	if !guruScope {
		scope = "" // admin sees all
	}

	q := fmt.Sprintf(`
WITH raw_events AS (
  SELECT
    'submission_baru' AS kind,
    s.id::text        AS id,
    s.submitted_at    AS at,
    k.id              AS kelas_id,
    k.nama            AS kelas_nama,
    s.siswa_id        AS siswa_id,
    COALESCE(us.name, '') AS siswa_nama,
    t.id              AS tugas_id,
    t.judul           AS tugas_judul,
    s.is_late         AS is_late,
    NULL::uuid        AS ujian_id,
    NULL::text        AS ujian_judul,
    NULL::uuid        AS hasil_id,
    NULL::numeric     AS nilai_total
  FROM submission s
  JOIN tugas t ON t.id = s.tugas_id
  JOIN kelas k ON k.id = t.kelas_id
  LEFT JOIN users us ON us.id = s.siswa_id
  WHERE k.archived_at IS NULL
    %s

  UNION ALL

  SELECT
    'ulangan_selesai' AS kind,
    h.id::text        AS id,
    COALESCE(h.selesai_at, h.created_at) AS at,
    k.id              AS kelas_id,
    k.nama            AS kelas_nama,
    h.siswa_id        AS siswa_id,
    COALESCE(us.name, '') AS siswa_nama,
    NULL::uuid        AS tugas_id,
    NULL::text        AS tugas_judul,
    NULL::boolean     AS is_late,
    u.id              AS ujian_id,
    u.judul           AS ujian_judul,
    h.id              AS hasil_id,
    h.nilai_total     AS nilai_total
  FROM hasil_ujian h
  JOIN ujian u ON u.id = h.ujian_id
  JOIN kelas k ON k.id = u.kelas_id
  LEFT JOIN users us ON us.id = h.siswa_id
  WHERE h.status = 'selesai'
    AND h.deleted_at IS NULL
    AND k.archived_at IS NULL
    %s

  UNION ALL

  SELECT
    'siswa_join'      AS kind,
    (replace(e.kelas_id::text, '-', '') || '_' || replace(e.siswa_id::text, '-', '')) AS id,
    e.joined_at       AS at,
    k.id              AS kelas_id,
    k.nama            AS kelas_nama,
    e.siswa_id        AS siswa_id,
    COALESCE(us.name, '') AS siswa_nama,
    NULL::uuid        AS tugas_id,
    NULL::text        AS tugas_judul,
    NULL::boolean     AS is_late,
    NULL::uuid        AS ujian_id,
    NULL::text        AS ujian_judul,
    NULL::uuid        AS hasil_id,
    NULL::numeric     AS nilai_total
  FROM enrollment e
  JOIN kelas k ON k.id = e.kelas_id
  LEFT JOIN users us ON us.id = e.siswa_id
  WHERE e.status = 'active'
    AND k.archived_at IS NULL
    %s
)
SELECT *
FROM raw_events
WHERE (at, id) < (?, ?)
ORDER BY at DESC, id DESC
LIMIT ?
`, scope, scope, scope)

	var rows []rawEventRow
	args := []any{}
	if guruScope {
		args = append(args, guruID, guruID, guruID)
	}
	args = append(args, beforeAt, beforeID, limit)

	if err := r.db.WithContext(ctx).Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// Service exposes the feed list operation with caller authorization.
type Service struct {
	repo *Repo
}

// NewService wires the repo.
func NewService(r *Repo) *Service { return &Service{repo: r} }

// List returns the next page of events for guruID (or all when admin).
// limit clamped to [1,maxLimit]; cursor empty → default latest slice.
func (s *Service) List(ctx context.Context, guruID uuid.UUID, callerRole string, cursor string, limit int) (*ListResponse, error) {
	if callerRole != string(auth.Guru) && callerRole != string(auth.Admin) {
		return nil, ErrForbidden
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	beforeAt := time.Now().UTC().Add(24 * time.Hour) // future-most
	beforeID := strings.Repeat("z", 64)              // upper bound
	if cursor != "" {
		t, id, err := decodeCursor(cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		beforeAt = t
		beforeID = id
	}

	guruScope := callerRole == string(auth.Guru)
	raws, err := s.repo.listForGuru(ctx, guruID, guruScope, beforeAt, beforeID, limit+1)
	if err != nil {
		return nil, err
	}

	// Defensive: re-sort. Postgres already ordered, but UNION ALL +
	// CTE planner can occasionally emit out-of-order if expression
	// types differ. Cheap (<= 50 rows).
	sort.Slice(raws, func(i, j int) bool {
		if !raws[i].At.Equal(raws[j].At) {
			return raws[i].At.After(raws[j].At)
		}
		return raws[i].ID > raws[j].ID
	})

	hasMore := false
	if len(raws) > limit {
		hasMore = true
		raws = raws[:limit]
	}

	events := make([]Event, 0, len(raws))
	for _, r := range raws {
		ev := Event{
			ID:        r.ID,
			Kind:      EventKind(r.Kind),
			At:        r.At,
			KelasID:   r.KelasID,
			KelasNama: r.KelasNama,
			SiswaID:   r.SiswaID,
			SiswaNama: r.SiswaNama,
			TugasID:   r.TugasID,
			IsLate:    r.IsLate,
			UjianID:   r.UjianID,
			HasilID:   r.HasilID,
			NilaiTotal: r.NilaiTotal,
		}
		if r.TugasJudul != nil {
			ev.TugasJudul = *r.TugasJudul
		}
		if r.UjianJudul != nil {
			ev.UjianJudul = *r.UjianJudul
		}
		events = append(events, ev)
	}

	next := ""
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		next = encodeCursor(last.At, last.ID)
	}
	return &ListResponse{Events: events, NextCursor: next}, nil
}

// encodeCursor packs (at_unix_micro, id) into a base64 string.
func encodeCursor(t time.Time, id string) string {
	raw := fmt.Sprintf("%d:%s", t.UTC().UnixMicro(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor unpacks the base64 cursor.
func decodeCursor(s string) (time.Time, string, error) {
	bs, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// Try standard base64 as fallback.
		bs, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return time.Time{}, "", fmt.Errorf("decode: %w", err)
		}
	}
	parts := strings.SplitN(string(bs), ":", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("malformed")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("micros: %w", err)
	}
	return time.UnixMicro(micros).UTC(), parts[1], nil
}
