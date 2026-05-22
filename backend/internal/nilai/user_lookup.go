// User-name lookup adapter for the nilai rekap matrix.
//
// auth.Repo.FindUserByID returns *auth.User; the rekap service only needs
// the display name. This adapter shrinks the dep so tests can inject mocks
// trivially.
package nilai

import (
	"context"

	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/auth"
)

// UserNameAdapter wraps an *auth.Repo into a rekapUserLookup.
type UserNameAdapter struct {
	Repo *auth.Repo
}

// NameByID returns the user's name. Missing user returns empty string +
// nil error so the rekap matrix can render "—" instead of failing the
// whole request for one orphaned siswa row.
func (a UserNameAdapter) NameByID(ctx context.Context, id uuid.UUID) (string, error) {
	if a.Repo == nil {
		return "", nil
	}
	u, err := a.Repo.FindUserByID(ctx, id)
	if err != nil {
		return "", nil
	}
	if u == nil {
		return "", nil
	}
	return u.Name, nil
}
