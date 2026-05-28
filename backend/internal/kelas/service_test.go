package kelas

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
)

// mockRepo implements kelasRepo backed by in-memory maps. ListAll, ListByGuru,
// FindByID share the same store. FindByKodeInvite walks the store on demand.
type mockRepo struct {
	rows        map[uuid.UUID]*Kelas
	enrollments map[string]*Enrollment // key = kelasID|siswaID

	createErr      error
	updateBasicErr error
	archiveErr     error
	enrollErr      error

	createdRows []*Kelas
	updateCalls int
	archiveIDs  []uuid.UUID
	enrollCalls []enrollCall

	now func() time.Time
}

type enrollCall struct {
	KelasID uuid.UUID
	SiswaID uuid.UUID
	Via     JoinedVia
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		rows:        map[uuid.UUID]*Kelas{},
		enrollments: map[string]*Enrollment{},
		now:         time.Now,
	}
}

func (m *mockRepo) Create(ctx context.Context, k *Kelas) error {
	if m.createErr != nil {
		return m.createErr
	}
	if k.ID == uuid.Nil {
		k.ID = uuid.New()
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = m.now()
	}
	k.UpdatedAt = m.now()
	if k.Version == 0 {
		k.Version = 1
	}
	cp := *k
	m.rows[cp.ID] = &cp
	m.createdRows = append(m.createdRows, &cp)
	return nil
}

func (m *mockRepo) FindByID(ctx context.Context, id uuid.UUID) (*Kelas, error) {
	row, ok := m.rows[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *row
	return &cp, nil
}

func (m *mockRepo) FindByKodeInvite(ctx context.Context, kode string) (*Kelas, error) {
	for _, row := range m.rows {
		if row.KodeInvite == kode {
			cp := *row
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (m *mockRepo) ListByGuru(ctx context.Context, guruID uuid.UUID, sekolahID *uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	var matched []Kelas
	for _, row := range m.rows {
		if row.GuruID != guruID {
			continue
		}
		if sekolahID != nil && (row.SekolahID == nil || *row.SekolahID != *sekolahID) {
			continue
		}
		if !includeArchived && row.ArchivedAt != nil {
			continue
		}
		matched = append(matched, *row)
	}
	return paginate(matched, limit, offset)
}

func (m *mockRepo) ListAll(ctx context.Context, sekolahID *uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	var matched []Kelas
	for _, row := range m.rows {
		if sekolahID != nil && (row.SekolahID == nil || *row.SekolahID != *sekolahID) {
			continue
		}
		if !includeArchived && row.ArchivedAt != nil {
			continue
		}
		matched = append(matched, *row)
	}
	return paginate(matched, limit, offset)
}

func (m *mockRepo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, nama, deskripsi string, bobotSoalUlangan, bobotTugas int) error {
	m.updateCalls++
	if m.updateBasicErr != nil {
		return m.updateBasicErr
	}
	row, ok := m.rows[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if row.Version != expectedVersion {
		return ErrVersionConflict
	}
	row.Nama = nama
	row.Deskripsi = deskripsi
	row.BobotSoalUlangan = bobotSoalUlangan
	row.BobotTugas = bobotTugas
	row.Version++
	row.UpdatedAt = m.now()
	return nil
}

func (m *mockRepo) Archive(ctx context.Context, id uuid.UUID) error {
	if m.archiveErr != nil {
		return m.archiveErr
	}
	row, ok := m.rows[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	if row.ArchivedAt != nil {
		return gorm.ErrRecordNotFound
	}
	t := m.now()
	row.ArchivedAt = &t
	row.UpdatedAt = t
	m.archiveIDs = append(m.archiveIDs, id)
	return nil
}

func (m *mockRepo) Unarchive(ctx context.Context, id uuid.UUID) error {
	row, ok := m.rows[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	row.ArchivedAt = nil
	return nil
}

func enrollKey(kelasID, siswaID uuid.UUID) string {
	return kelasID.String() + "|" + siswaID.String()
}

func (m *mockRepo) Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via JoinedVia) (bool, error) {
	if m.enrollErr != nil {
		return false, m.enrollErr
	}
	m.enrollCalls = append(m.enrollCalls, enrollCall{KelasID: kelasID, SiswaID: siswaID, Via: via})
	key := enrollKey(kelasID, siswaID)
	if _, exists := m.enrollments[key]; exists {
		return false, nil
	}
	m.enrollments[key] = &Enrollment{
		KelasID:   kelasID,
		SiswaID:   siswaID,
		Status:    EnrollmentActive,
		JoinedVia: via,
	}
	return true, nil
}

func (m *mockRepo) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*Enrollment, error) {
	row, ok := m.enrollments[enrollKey(kelasID, siswaID)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *row
	return &cp, nil
}

func (m *mockRepo) ListEnrollmentsBySiswa(ctx context.Context, siswaID uuid.UUID, limit, offset int) ([]Enrollment, int64, error) {
	rows := make([]Enrollment, 0)
	for _, e := range m.enrollments {
		if e.SiswaID == siswaID {
			rows = append(rows, *e)
		}
	}
	total := int64(len(rows))
	if offset >= len(rows) {
		return []Enrollment{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
}

func (m *mockRepo) ListEnrollmentsByKelas(ctx context.Context, kelasID uuid.UUID, limit, offset int) ([]Enrollment, int64, error) {
	rows := make([]Enrollment, 0)
	for _, e := range m.enrollments {
		if e.KelasID == kelasID {
			rows = append(rows, *e)
		}
	}
	total := int64(len(rows))
	if offset >= len(rows) {
		return []Enrollment{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
}

// RemoveEnrollment is a test-only helper mirroring *Repo.RemoveEnrollment
// (soft-flip status to removed). Bukan bagian dari kelasRepo interface karena
// service belum perlu — dipakai di service_test.go untuk siap-siap fixture
// "removed enrollment hidden by default".
func (m *mockRepo) RemoveEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	row, ok := m.enrollments[enrollKey(kelasID, siswaID)]
	if !ok || row.Status != EnrollmentActive {
		return gorm.ErrRecordNotFound
	}
	row.Status = EnrollmentRemoved
	return nil
}

func paginate(rows []Kelas, limit, offset int) ([]Kelas, int64, error) {
	total := int64(len(rows))
	if offset >= len(rows) {
		return []Kelas{}, total, nil
	}
	end := offset + limit
	if end > len(rows) {
		end = len(rows)
	}
	return rows[offset:end], total, nil
}

type recordingAudit struct {
	entries []*auth.AuditLog
	err     error
}

func (r *recordingAudit) LogAudit(ctx context.Context, entry *auth.AuditLog) error {
	if r.err != nil {
		return r.err
	}
	r.entries = append(r.entries, entry)
	return nil
}

// stubUserLookup is the in-memory userLookup used by service tests. Empty by
// default — tests that exercise ListEnrollmentsByKelas seed users explicitly.
type stubUserLookup struct {
	users map[uuid.UUID]*auth.User
	err   error
}

func newStubUserLookup() *stubUserLookup {
	return &stubUserLookup{users: map[uuid.UUID]*auth.User{}}
}

func (s *stubUserLookup) FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	u, ok := s.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	cp := *u
	return &cp, nil
}

func newSvc(t *testing.T) (*Service, *mockRepo, *recordingAudit) {
	t.Helper()
	repo := newMockRepo()
	audit := &recordingAudit{}
	svc := NewService(repo, audit, nil)
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }
	return svc, repo, audit
}

// newSvcWithUsers mirrors newSvc but also returns the stubUserLookup so tests
// covering ListEnrollmentsByKelas can seed user rows.
func newSvcWithUsers(t *testing.T) (*Service, *mockRepo, *recordingAudit, *stubUserLookup) {
	t.Helper()
	repo := newMockRepo()
	audit := &recordingAudit{}
	users := newStubUserLookup()
	svc := NewService(repo, audit, users)
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }
	return svc, repo, audit, users
}

func TestService_ListMyKelas_HidesArchivedKelas(t *testing.T) {
	svc, repo, _ := newSvc(t)
	siswaID := uuid.New()
	activeID := uuid.New()
	archivedID := uuid.New()
	archivedAt := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)

	repo.rows[activeID] = &Kelas{ID: activeID, Nama: "Active", GuruID: uuid.New(), Version: 1}
	repo.rows[archivedID] = &Kelas{ID: archivedID, Nama: "Archived", GuruID: uuid.New(), Version: 1, ArchivedAt: &archivedAt}
	repo.enrollments[enrollKey(activeID, siswaID)] = &Enrollment{KelasID: activeID, SiswaID: siswaID, Status: EnrollmentActive, JoinedVia: JoinedViaKode}
	repo.enrollments[enrollKey(archivedID, siswaID)] = &Enrollment{KelasID: archivedID, SiswaID: siswaID, Status: EnrollmentActive, JoinedVia: JoinedViaKode}

	res, err := svc.ListMyKelas(context.Background(), siswaID, ListInput{Limit: 10})
	if err != nil {
		t.Fatalf("ListMyKelas error: %v", err)
	}
	if res.Total != 1 || len(res.Items) != 1 {
		t.Fatalf("visible kelas mismatch: total=%d items=%+v", res.Total, res.Items)
	}
	if res.Items[0].Kelas.ID != activeID {
		t.Fatalf("expected active kelas only, got %+v", res.Items[0].Kelas)
	}
}

func TestService_Create_HappyPath(t *testing.T) {
	svc, repo, audit := newSvc(t)
	guruID := uuid.New()

	k, err := svc.Create(context.Background(), guruID, CreateInput{
		Nama:             "Matematika 7A",
		Deskripsi:        "  pelajaran dasar  ",
		BobotSoalUlangan: 60,
		BobotTugas:       40,
	}, "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if k.ID == uuid.Nil {
		t.Fatal("ID should be set after create")
	}
	if k.Nama != "Matematika 7A" {
		t.Fatalf("nama mismatch: %q", k.Nama)
	}
	if k.Deskripsi != "pelajaran dasar" {
		t.Fatalf("deskripsi not trimmed: %q", k.Deskripsi)
	}
	if k.GuruID != guruID {
		t.Fatalf("guru id mismatch")
	}
	if k.Version != 1 {
		t.Fatalf("version expected 1, got %d", k.Version)
	}
	if k.BobotSoalUlangan != 50 || k.BobotTugas != 50 {
		t.Fatalf("class-level bobot should use legacy defaults, got %d/%d", k.BobotSoalUlangan, k.BobotTugas)
	}
	if len(k.KodeInvite) != KodeInviteLength {
		t.Fatalf("kode invite len: %d", len(k.KodeInvite))
	}
	if len(repo.createdRows) != 1 {
		t.Fatalf("expected 1 created row, got %d", len(repo.createdRows))
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "kelas_created" {
		t.Fatalf("expected single kelas_created audit, got %#v", audit.entries)
	}
}

func TestService_Create_DefaultBobot(t *testing.T) {
	svc, _, _ := newSvc(t)
	guruID := uuid.New()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if k.BobotSoalUlangan != 50 || k.BobotTugas != 50 {
		t.Fatalf("expected default 50/50, got %d/%d", k.BobotSoalUlangan, k.BobotTugas)
	}
}

func TestService_Create_RejectsBlankNama(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.Create(context.Background(), uuid.New(), CreateInput{Nama: "  "}, "", "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestService_Create_IgnoresDeprecatedBobot(t *testing.T) {
	svc, _, _ := newSvc(t)
	k, err := svc.Create(context.Background(), uuid.New(), CreateInput{
		Nama:             "X",
		BobotSoalUlangan: 70,
		BobotTugas:       40,
	}, "", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if k.BobotSoalUlangan != 50 || k.BobotTugas != 50 {
		t.Fatalf("deprecated bobot should be ignored, got %d/%d", k.BobotSoalUlangan, k.BobotTugas)
	}
}

func TestService_Update_VersionConflict(t *testing.T) {
	svc, repo, _ := newSvc(t)
	guruID := uuid.New()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a concurrent update bumping version.
	repo.rows[k.ID].Version = 2

	_, err = svc.Update(context.Background(), k.ID, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion:  1,
		Nama:             "Y",
		BobotSoalUlangan: intPtr(50),
		BobotTugas:       intPtr(50),
	}, "", "")
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict, got %v", err)
	}
}

func TestService_Update_HappyPath(t *testing.T) {
	svc, _, audit := newSvc(t)
	guruID := uuid.New()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Update(context.Background(), k.ID, guruID, string(auth.Guru), UpdateInput{
		ExpectedVersion:  1,
		Nama:             "  IPA 7A ",
		Deskripsi:        strRefPtr("fisika"),
		BobotSoalUlangan: intPtr(70),
		BobotTugas:       intPtr(30),
	}, "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if updated.Nama != "IPA 7A" {
		t.Fatalf("nama not trimmed: %q", updated.Nama)
	}
	if updated.Version != 2 {
		t.Fatalf("expected version 2, got %d", updated.Version)
	}
	if updated.BobotSoalUlangan != 50 || updated.BobotTugas != 50 {
		t.Fatalf("deprecated bobot should be retained, got %d/%d", updated.BobotSoalUlangan, updated.BobotTugas)
	}
	if got := lastAuditAction(audit); got != "kelas_updated" {
		t.Fatalf("expected kelas_updated audit, got %q", got)
	}
}

func TestService_Update_ForbiddenForOtherGuru(t *testing.T) {
	svc, _, _ := newSvc(t)
	owner := uuid.New()
	stranger := uuid.New()
	k, err := svc.Create(context.Background(), owner, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(context.Background(), k.ID, stranger, string(auth.Guru), UpdateInput{
		ExpectedVersion: 1, Nama: "Y", BobotSoalUlangan: intPtr(50), BobotTugas: intPtr(50),
	}, "", "")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestService_Update_AdminCanEditOtherGurusKelas(t *testing.T) {
	svc, _, _ := newSvc(t)
	owner := uuid.New()
	admin := uuid.New()
	k, err := svc.Create(context.Background(), owner, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Update(context.Background(), k.ID, admin, string(auth.Admin), UpdateInput{
		ExpectedVersion: 1, Nama: "Y", BobotSoalUlangan: intPtr(50), BobotTugas: intPtr(50),
	}, "", "")
	if err != nil {
		t.Fatalf("admin should be allowed: %v", err)
	}
}

func TestService_Archive_AlreadyArchived(t *testing.T) {
	svc, repo, _ := newSvc(t)
	guruID := uuid.New()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	repo.rows[k.ID].ArchivedAt = &t0

	_, err = svc.Archive(context.Background(), k.ID, guruID, string(auth.Guru), "", "")
	if !errors.Is(err, ErrAlreadyArchived) {
		t.Fatalf("expected ErrAlreadyArchived, got %v", err)
	}
}

func TestService_Archive_HappyPath(t *testing.T) {
	svc, _, audit := newSvc(t)
	guruID := uuid.New()
	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Archive(context.Background(), k.ID, guruID, string(auth.Guru), "", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if updated.ArchivedAt == nil {
		t.Fatal("ArchivedAt should be set")
	}
	if got := lastAuditAction(audit); got != "kelas_archived" {
		t.Fatalf("expected kelas_archived audit, got %q", got)
	}
}

func TestService_Duplicate_RegeneratesKodeAndDoesNotCarryArchive(t *testing.T) {
	svc, repo, audit := newSvc(t)
	guruID := uuid.New()
	original, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	repo.rows[original.ID].ArchivedAt = &t0

	dup, err := svc.Duplicate(context.Background(), original.ID, guruID, string(auth.Guru), DuplicateInput{}, "", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dup.ID == original.ID {
		t.Fatal("duplicate should have new ID")
	}
	if dup.KodeInvite == original.KodeInvite {
		t.Fatal("duplicate must regenerate kode invite")
	}
	if dup.Version != 1 {
		t.Fatalf("duplicate version expected 1, got %d", dup.Version)
	}
	if dup.ArchivedAt != nil {
		t.Fatal("duplicate should NOT carry archived_at")
	}
	if !strings.Contains(dup.Nama, "Salinan") {
		t.Fatalf("expected default 'Salinan' suffix, got %q", dup.Nama)
	}
	if got := lastAuditAction(audit); got != "kelas_duplicated" {
		t.Fatalf("expected kelas_duplicated audit, got %q", got)
	}
}

func TestService_Duplicate_CustomNama(t *testing.T) {
	svc, _, _ := newSvc(t)
	guruID := uuid.New()
	original, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	dup, err := svc.Duplicate(context.Background(), original.ID, guruID, string(auth.Guru), DuplicateInput{NewNama: "  IPA 8B  "}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if dup.Nama != "IPA 8B" {
		t.Fatalf("nama not trimmed: %q", dup.Nama)
	}
}

func TestService_Get_OtherGuruForbidden(t *testing.T) {
	svc, _, _ := newSvc(t)
	owner := uuid.New()
	stranger := uuid.New()
	k, err := svc.Create(context.Background(), owner, CreateInput{Nama: "X"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Get(context.Background(), k.ID, stranger, string(auth.Guru))
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestService_ListForGuru_OnlyReturnsOwn(t *testing.T) {
	svc, _, _ := newSvc(t)
	g1 := uuid.New()
	g2 := uuid.New()
	if _, err := svc.Create(context.Background(), g1, CreateInput{Nama: "A"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), g1, CreateInput{Nama: "B"}, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(context.Background(), g2, CreateInput{Nama: "C"}, "", ""); err != nil {
		t.Fatal(err)
	}
	res, err := svc.ListForGuru(context.Background(), g1, ListInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 2 {
		t.Fatalf("expected 2, got %d", res.Total)
	}
	for _, k := range res.Items {
		if k.GuruID != g1 {
			t.Fatalf("leaked g2 kelas: %s", k.ID)
		}
	}
}

func lastAuditAction(r *recordingAudit) string {
	if len(r.entries) == 0 {
		return ""
	}
	return r.entries[len(r.entries)-1].Action
}

// --- ListEnrollmentsByKelas (Task 2.C.4) ---

func TestService_ListEnrollmentsByKelas_HappyPath(t *testing.T) {
	svc, repo, _, users := newSvcWithUsers(t)
	guruID := uuid.New()
	siswa1, siswa2 := uuid.New(), uuid.New()
	users.users[guruID] = &auth.User{ID: guruID, Name: "Guru", Email: "guru@example.com", Role: auth.Guru, Status: auth.Active}
	users.users[siswa1] = &auth.User{ID: siswa1, Name: "Andi", Email: "andi@example.com", Role: auth.Siswa, Status: auth.Active}
	users.users[siswa2] = &auth.User{ID: siswa2, Name: "Budi", Email: "budi@example.com", Role: auth.Siswa, Status: auth.Active}

	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "Kelas A", BobotSoalUlangan: 50, BobotTugas: 50}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enroll(context.Background(), k.ID, siswa1, JoinedViaKode); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enroll(context.Background(), k.ID, siswa2, JoinedViaAdmin); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ListEnrollmentsByKelas(context.Background(), k.ID, guruID, string(auth.Guru), EnrollmentListInput{Limit: 10})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.Total != 2 || len(res.Items) != 2 {
		t.Fatalf("unexpected items: total=%d len=%d", res.Total, len(res.Items))
	}
	for _, item := range res.Items {
		if item.Nama == "" || item.Email == "" {
			t.Fatalf("missing hydrate fields: %+v", item)
		}
	}
}

func TestService_ListEnrollmentsByKelas_HidesRemovedByDefault(t *testing.T) {
	svc, repo, _, users := newSvcWithUsers(t)
	guruID := uuid.New()
	siswa := uuid.New()
	users.users[guruID] = &auth.User{ID: guruID, Name: "Guru", Email: "guru@example.com", Role: auth.Guru, Status: auth.Active}
	users.users[siswa] = &auth.User{ID: siswa, Name: "Andi", Email: "andi@example.com", Role: auth.Siswa, Status: auth.Active}

	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "K", BobotSoalUlangan: 50, BobotTugas: 50}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enroll(context.Background(), k.ID, siswa, JoinedViaKode); err != nil {
		t.Fatal(err)
	}
	// soft-remove
	if err := repo.RemoveEnrollment(context.Background(), k.ID, siswa); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ListEnrollmentsByKelas(context.Background(), k.ID, guruID, string(auth.Guru), EnrollmentListInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("expected removed enrollment to be hidden, got %d items", len(res.Items))
	}
	// total still reflects raw DB rows so pagination math stays correct
	if res.Total != 1 {
		t.Fatalf("expected raw total=1, got %d", res.Total)
	}

	// admin opt-in: include_removed=true → row visible again
	res, err = svc.ListEnrollmentsByKelas(context.Background(), k.ID, guruID, string(auth.Admin), EnrollmentListInput{Limit: 10, IncludeRemoved: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item with IncludeRemoved=true, got %d", len(res.Items))
	}
	if res.Items[0].Status != EnrollmentRemoved {
		t.Fatalf("status mismatch: %s", res.Items[0].Status)
	}
}

func TestService_ListEnrollmentsByKelas_ForbiddenForOtherGuru(t *testing.T) {
	svc, _, _, users := newSvcWithUsers(t)
	owner := uuid.New()
	intruder := uuid.New()
	users.users[owner] = &auth.User{ID: owner, Name: "Guru", Email: "guru@example.com", Role: auth.Guru, Status: auth.Active}

	k, err := svc.Create(context.Background(), owner, CreateInput{Nama: "K", BobotSoalUlangan: 50, BobotTugas: 50}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.ListEnrollmentsByKelas(context.Background(), k.ID, intruder, string(auth.Guru), EnrollmentListInput{Limit: 10})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}

	// admin can read foreign kelas
	_, err = svc.ListEnrollmentsByKelas(context.Background(), k.ID, intruder, string(auth.Admin), EnrollmentListInput{Limit: 10})
	if err != nil {
		t.Fatalf("admin should be allowed, got %v", err)
	}
}

func TestService_ListEnrollmentsByKelas_NotFound(t *testing.T) {
	svc, _, _, _ := newSvcWithUsers(t)
	_, err := svc.ListEnrollmentsByKelas(context.Background(), uuid.New(), uuid.New(), string(auth.Admin), EnrollmentListInput{Limit: 10})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestService_ListEnrollmentsByKelas_DanglingUserSkipped(t *testing.T) {
	svc, repo, _, users := newSvcWithUsers(t)
	guruID := uuid.New()
	live := uuid.New()
	dangling := uuid.New()
	users.users[guruID] = &auth.User{ID: guruID, Name: "Guru", Email: "guru@example.com", Role: auth.Guru, Status: auth.Active}
	users.users[live] = &auth.User{ID: live, Name: "Live", Email: "live@x", Role: auth.Siswa, Status: auth.Active}
	// dangling siswa intentionally missing from users map

	k, err := svc.Create(context.Background(), guruID, CreateInput{Nama: "K", BobotSoalUlangan: 50, BobotTugas: 50}, "1.1.1.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enroll(context.Background(), k.ID, live, JoinedViaKode); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Enroll(context.Background(), k.ID, dangling, JoinedViaKode); err != nil {
		t.Fatal(err)
	}

	res, err := svc.ListEnrollmentsByKelas(context.Background(), k.ID, guruID, string(auth.Guru), EnrollmentListInput{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	// Total tetap 2 (raw DB), tapi items cuma 1 (live siswa). Dangling skipped.
	if res.Total != 2 {
		t.Fatalf("expected raw total=2, got %d", res.Total)
	}
	if len(res.Items) != 1 || res.Items[0].SiswaID != live {
		t.Fatalf("expected only live siswa, got %+v", res.Items)
	}
}
