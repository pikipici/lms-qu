// Bulk reorder endpoint for bab within a kelas.
//
// Reorder takes a complete ordered list of bab_ids + their current versions,
// runs a transaction that bumps urutan = position+1 for each row, and
// rejects 409 if ANY row's version mismatches at the moment of update
// (concurrent edit by another guru tab).
package bab

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Reorder-specific sentinel errors.
var (
	// ErrReorderEmpty is returned when the order array is empty.
	ErrReorderEmpty = errors.New("bab: reorder order list is empty")
	// ErrReorderDuplicate is returned when the order array contains a
	// duplicate bab_id.
	ErrReorderDuplicate = errors.New("bab: reorder order has duplicate id")
	// ErrReorderForeignBab is returned when the order contains a bab_id
	// that doesn't belong to the target kelas.
	ErrReorderForeignBab = errors.New("bab: reorder bab not in kelas")
	// ErrReorderMissing is returned when the order doesn't cover every bab
	// in the kelas (caller must include all bab — no partial reorder).
	ErrReorderMissing = errors.New("bab: reorder missing bab from kelas")
)

// ReorderInput is the bulk-reorder payload.
//
// Order is the new desired sequence (index 0 -> urutan=1, index 1 -> urutan=2,
// ...). Versions maps each bab_id to its caller-known current version; this
// is the optimistic-concurrency guard that rejects partial overlapping
// edits.
//
// All bab in the kelas must appear in Order — there is no partial reorder
// in MVP. Foreign bab (from a different kelas) is a 400.
type ReorderInput struct {
	Order    []uuid.UUID
	Versions map[uuid.UUID]int
}

// ReorderConflict carries the per-row mismatch info FE needs to refresh.
type ReorderConflict struct {
	BabID          uuid.UUID `json:"bab_id"`
	CurrentVersion int       `json:"current_version"`
}

// ReorderConflictErr wraps ErrVersionConflict with the list of mismatching
// rows. Handler unwraps this to surface the conflicts in the 409 body.
type ReorderConflictErr struct {
	Conflicts []ReorderConflict
}

// Error makes ReorderConflictErr satisfy error.
func (e *ReorderConflictErr) Error() string {
	return fmt.Sprintf("bab: reorder version conflict on %d row(s)", len(e.Conflicts))
}

// Unwrap exposes the underlying ErrVersionConflict so callers can match on
// it via errors.Is.
func (e *ReorderConflictErr) Unwrap() error { return ErrVersionConflict }

// Reorder applies a bulk position update inside a single transaction.
// Ownership guard: caller must own the kelas (or be admin).
//
// Two-phase update strategy: bumping urutan in-place can violate uniqueness
// if we ever add a UNIQUE(kelas_id, urutan) — currently we only have a
// regular btree so collisions are tolerated. To stay future-proof, we
// shift every row to a temp range first (urutan = -1 - i), then assign the
// final values (urutan = i+1). This means each row gets two version bumps
// per call. The handler's audit_log row reflects the final ordering only.
func (s *Service) Reorder(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ReorderInput, ip, userAgent string) ([]Bab, error) {
	if len(in.Order) == 0 {
		return nil, ErrReorderEmpty
	}

	// Detect duplicate bab_id in the order array.
	seen := make(map[uuid.UUID]struct{}, len(in.Order))
	for _, id := range in.Order {
		if _, dup := seen[id]; dup {
			return nil, fmt.Errorf("%w: %s", ErrReorderDuplicate, id)
		}
		seen[id] = struct{}{}
	}

	// Ownership guard via kelas lookup.
	if _, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	// Load the full bab set for the kelas (include archived, since archived
	// rows can still appear in the guru's list and must be reordered as
	// part of the kelas-level bulk operation).
	current, err := s.repo.ListByKelas(ctx, kelasID, ListFilter{IncludeArchived: true})
	if err != nil {
		return nil, fmt.Errorf("bab reorder list: %w", err)
	}
	if len(current) != len(in.Order) {
		return nil, fmt.Errorf("%w: kelas has %d bab, order has %d", ErrReorderMissing, len(current), len(in.Order))
	}

	// Index existing bab by id for O(1) lookup.
	currentByID := make(map[uuid.UUID]*Bab, len(current))
	for i := range current {
		currentByID[current[i].ID] = &current[i]
	}

	// Validate every id in Order belongs to the kelas, and pick up the
	// current version. If caller didn't supply Versions[id], fall back to
	// the existing version (lenient: client can omit Versions to "force"
	// reorder, but the row may still 409 if another tab edited it between
	// list and reorder — race window is tighter that way).
	versions := make(map[uuid.UUID]int, len(in.Order))
	for _, id := range in.Order {
		row, ok := currentByID[id]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrReorderForeignBab, id)
		}
		if v, supplied := in.Versions[id]; supplied {
			versions[id] = v
		} else {
			versions[id] = row.Version
		}
	}

	// Run the two-phase reorder in a single transaction. Phase 1 shifts
	// each row to a temp negative urutan with the caller-supplied version.
	// Phase 2 assigns final positions with the bumped version.
	db := s.repo.DB()
	conflicts := []ReorderConflict{}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Phase 1: shift to temp range. Version checked against caller's view.
		for i, id := range in.Order {
			tempUrutan := -1 - i // negative, monotonically decreasing
			expectedV := versions[id]
			if err := s.repo.UpdateUrutan(ctx, tx, id, expectedV, tempUrutan); err != nil {
				if errors.Is(err, ErrVersionConflict) {
					row := currentByID[id]
					conflicts = append(conflicts, ReorderConflict{BabID: id, CurrentVersion: row.Version})
					return ErrVersionConflict
				}
				return err
			}
		}
		// Phase 2: assign final urutan = position+1 with the bumped version.
		for i, id := range in.Order {
			expectedV := versions[id] + 1 // bumped after phase 1
			if err := s.repo.UpdateUrutan(ctx, tx, id, expectedV, i+1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if len(conflicts) > 0 {
			return nil, &ReorderConflictErr{Conflicts: conflicts}
		}
		return nil, fmt.Errorf("bab reorder tx: %w", err)
	}

	// Refetch the final ordering for the response + audit meta.
	fresh, err := s.repo.ListByKelas(ctx, kelasID, ListFilter{IncludeArchived: true})
	if err != nil {
		return nil, fmt.Errorf("bab reorder refetch: %w", err)
	}

	// Audit: 1 entry per call. Order field in meta is the final id sequence.
	orderStrs := make([]string, len(in.Order))
	for i, id := range in.Order {
		orderStrs[i] = id.String()
	}
	s.logAudit(ctx, "bab_reordered", &callerID, callerRole, nil, &kelasID, ip, userAgent, map[string]any{
		"kelas_id": kelasID.String(),
		"count":    len(in.Order),
		"order":    orderStrs,
	})
	return fresh, nil
}
