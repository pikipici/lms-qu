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

func (m *mockRepo) ListByGuru(ctx context.Context, guruID uuid.UUID, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	var matched []Kelas
	for _, row := range m.rows {
		if row.GuruID != guruID {
			continue
		}
		if !includeArchived && row.ArchivedAt != nil {
			continue
		}
		matched = append(matched, *row)
	}
	return paginate(matched, limit, offset)
}

func (m *mockRepo) ListAll(ctx context.Context, includeArchived bool, limit, offset int) ([]Kelas, int64, error) {
	var matched []Kelas
	for _, row := range m.rows {
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

func newSvc(t *testing.T) (*Service, *mockRepo, *recordingAudit) {
	t.Helper()
	repo := newMockRepo()
	audit := &recordingAudit{}
	svc := NewService(repo, audit)
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) }
	return svc, repo, audit
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
	if k.BobotSoalUlangan != 60 || k.BobotTugas != 40 {
		t.Fatalf("bobot mismatch %d/%d", k.BobotSoalUlangan, k.BobotTugas)
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

func TestService_Create_RejectsBobotMismatch(t *testing.T) {
	svc, _, _ := newSvc(t)
	_, err := svc.Create(context.Background(), uuid.New(), CreateInput{
		Nama:             "X",
		BobotSoalUlangan: 70,
		BobotTugas:       40,
	}, "", "")
	if !errors.Is(err, ErrBobotInvalid) {
		t.Fatalf("expected ErrBobotInvalid, got %v", err)
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
	if updated.BobotSoalUlangan != 70 {
		t.Fatalf("bobot mismatch: %d", updated.BobotSoalUlangan)
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
