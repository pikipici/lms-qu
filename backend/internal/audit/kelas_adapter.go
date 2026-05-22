package audit

import (
	"context"

	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/kelas"
)

// KelasFinderAdapter wraps internal/kelas.Repo into the audit kelasFinder
// interface (avoids tight import in service.go).
type KelasFinderAdapter struct {
	Repo interface {
		FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
	}
}

// FindByID adapts the internal/kelas.Repo signature to the audit
// kelasMini projection.
func (a KelasFinderAdapter) FindByID(ctx context.Context, id uuid.UUID) (*kelasMini, error) {
	k, err := a.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	status := "active"
	if k.ArchivedAt != nil {
		status = "archived"
	}
	return &kelasMini{
		ID:     k.ID,
		GuruID: k.GuruID,
		Status: status,
	}, nil
}
