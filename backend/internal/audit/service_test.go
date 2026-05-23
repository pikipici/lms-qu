package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type fakeAuditRepo struct {
	rows   []auth.AuditLog
	total  int64
	err    error
	filter auth.AuditLogFilter
	limit  int
	offset int
}

func (f *fakeAuditRepo) ListAuditLogs(ctx context.Context, filter auth.AuditLogFilter, limit, offset int) ([]auth.AuditLog, int64, error) {
	f.filter = filter
	f.limit = limit
	f.offset = offset
	return f.rows, f.total, f.err
}

type fakeKelasFinder struct {
	kelas *kelasMini
	err   error
}

func (f fakeKelasFinder) FindByID(ctx context.Context, id uuid.UUID) (*kelasMini, error) {
	return f.kelas, f.err
}

type fakeUserLookup struct {
	names map[uuid.UUID]string
	err   error
	ids   []uuid.UUID
}

func (f *fakeUserLookup) BulkUserNames(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	f.ids = append([]uuid.UUID(nil), ids...)
	return f.names, f.err
}

func TestIsAllowedAction(t *testing.T) {
	if !IsAllowedAction("kelas_created") {
		t.Fatal("kelas_created should be allowed")
	}
	if IsAllowedAction("auth_login_failed") {
		t.Fatal("auth_login_failed should not be allowed for guru audit")
	}
}

func TestListByKelasSuccessClampsLimitAndEnrichesActor(t *testing.T) {
	kelasID := uuid.New()
	guruID := uuid.New()
	actorID := uuid.New()
	actorRole := string(auth.Guru)
	targetType := "kelas"
	targetID := uuid.New()
	at := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	repo := &fakeAuditRepo{rows: []auth.AuditLog{{
		ID:            uuid.New(),
		ActorID:       &actorID,
		ActorRole:     &actorRole,
		Action:        "kelas_created",
		TargetType:    &targetType,
		TargetID:      &targetID,
		TargetKelasID: &kelasID,
		Meta:          datatypes.JSON([]byte(`{"ok":true}`)),
		At:            at,
	}}, total: 1}
	users := &fakeUserLookup{names: map[uuid.UUID]string{actorID: "Guru One"}}
	svc := NewService(repo, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, users)

	res, err := svc.ListByKelas(context.Background(), kelasID, guruID, string(auth.Guru), "kelas_created", 999, 3)
	if err != nil {
		t.Fatalf("ListByKelas() error = %v", err)
	}
	if res.Total != 1 || res.Limit != 100 || res.Offset != 3 || len(res.Events) != 1 {
		t.Fatalf("response = %+v", res)
	}
	if res.Events[0].ActorName == nil || *res.Events[0].ActorName != "Guru One" {
		t.Fatalf("actor name = %#v", res.Events[0].ActorName)
	}
	if repo.limit != 100 || repo.offset != 3 {
		t.Fatalf("repo pagination = %d/%d", repo.limit, repo.offset)
	}
	if repo.filter.TargetKelasID == nil || *repo.filter.TargetKelasID != kelasID || repo.filter.Action != "kelas_created" || len(repo.filter.Actions) != len(AllowedActions) {
		t.Fatalf("repo filter = %+v", repo.filter)
	}
	if len(users.ids) != 1 || users.ids[0] != actorID {
		t.Fatalf("lookup ids = %#v", users.ids)
	}
}

func TestListByKelasAdminCanAccessOtherGuruKelas(t *testing.T) {
	kelasID := uuid.New()
	repo := &fakeAuditRepo{}
	svc := NewService(repo, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: uuid.New()}}, &fakeUserLookup{})

	res, err := svc.ListByKelas(context.Background(), kelasID, uuid.New(), string(auth.Admin), "", 0, 0)
	if err != nil {
		t.Fatalf("ListByKelas() error = %v", err)
	}
	if res.Limit != 50 || repo.limit != 50 {
		t.Fatalf("default limit response/repo = %d/%d", res.Limit, repo.limit)
	}
}

func TestListByKelasValidationAndOwnershipErrors(t *testing.T) {
	kelasID := uuid.New()
	guruID := uuid.New()
	tests := []struct {
		name   string
		role   string
		action string
		offset int
		finder fakeKelasFinder
		want   error
	}{
		{"bad role", string(auth.Siswa), "", 0, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, ErrForbidden},
		{"invalid action", string(auth.Guru), "not_allowed", 0, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, ErrInvalidAction},
		{"invalid offset", string(auth.Guru), "", -1, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, ErrInvalidPaginate},
		{"not found", string(auth.Guru), "", 0, fakeKelasFinder{err: gorm.ErrRecordNotFound}, ErrNotFound},
		{"other guru", string(auth.Guru), "", 0, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: uuid.New()}}, ErrForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&fakeAuditRepo{}, tt.finder, &fakeUserLookup{})
			_, err := svc.ListByKelas(context.Background(), kelasID, guruID, tt.role, tt.action, 50, tt.offset)
			if !errors.Is(err, tt.want) {
				t.Fatalf("ListByKelas() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestListByKelasPropagatesRepoAndSoftFailsUserLookup(t *testing.T) {
	kelasID := uuid.New()
	guruID := uuid.New()
	actorID := uuid.New()
	repoErr := errors.New("repo down")
	svc := NewService(&fakeAuditRepo{err: repoErr}, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, &fakeUserLookup{})
	_, err := svc.ListByKelas(context.Background(), kelasID, guruID, string(auth.Guru), "", 50, 0)
	if err == nil || !errors.Is(err, repoErr) {
		t.Fatalf("repo error = %v, want wrapped repo down", err)
	}

	repo := &fakeAuditRepo{rows: []auth.AuditLog{{ID: uuid.New(), ActorID: &actorID, Action: "kelas_created"}}, total: 1}
	svc = NewService(repo, fakeKelasFinder{kelas: &kelasMini{ID: kelasID, GuruID: guruID}}, &fakeUserLookup{err: errors.New("lookup down")})
	res, err := svc.ListByKelas(context.Background(), kelasID, guruID, string(auth.Guru), "", 50, 0)
	if err != nil {
		t.Fatalf("ListByKelas() error = %v", err)
	}
	if len(res.Events) != 1 || res.Events[0].ActorName != nil {
		t.Fatalf("soft-fail lookup events = %+v", res.Events)
	}
}
