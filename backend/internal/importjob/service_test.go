package importjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/storage"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// stubStore is a minimal in-memory storage.Storage for service tests. Mirrors
// MockStorage but with hooks to inject errors at specific operations.
type stubStore struct {
	mu          sync.Mutex
	objects     map[string][]byte
	putErr      error
	deleteCalls []string
}

func newStubStore() *stubStore {
	return &stubStore{objects: map[string][]byte{}}
}

func (s *stubStore) PutObject(ctx context.Context, in storage.PutObjectInput) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	body := make([]byte, 0)
	if in.Body != nil {
		buf := bytes.Buffer{}
		_, _ = buf.ReadFrom(in.Body)
		body = buf.Bytes()
	}
	s.objects[in.Key] = body
	return nil
}

func (s *stubStore) GetObject(ctx context.Context, key string) (*storage.Object, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, ok := s.objects[key]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return &storage.Object{Key: key, Size: int64(len(body)), Body: nopCloser{bytes.NewReader(body)}}, nil
}

func (s *stubStore) DeleteObject(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	s.deleteCalls = append(s.deleteCalls, key)
	return nil
}

func (s *stubStore) ObjectExists(ctx context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[key]
	return ok, nil
}

func (s *stubStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[key]; !ok {
		return "", storage.ErrObjectNotFound
	}
	return "stub://" + key, nil
}

func (s *stubStore) PresignGetDownload(ctx context.Context, key string, ttl time.Duration, filename string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[key]; !ok {
		return "", storage.ErrObjectNotFound
	}
	if filename != "" {
		return "stub://" + key + "?filename=" + filename, nil
	}
	return "stub://" + key, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// stubRepo captures inserts so tests can assert state without touching GORM.
type stubRepo struct {
	mu        sync.Mutex
	created   []*ImportJob
	createErr error
	// findFn is consulted by FindByIDForAdmin if non-nil; otherwise we look
	// up `created` in-memory by id+admin_id.
	findFn       func(ctx context.Context, id, adminID uuid.UUID) (*ImportJob, error)
	setStatusErr error
	statusCalls  []statusCall
}

type statusCall struct {
	id     uuid.UUID
	status Status
}

func (r *stubRepo) Create(ctx context.Context, j *ImportJob) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.created = append(r.created, j)
	return nil
}

func (r *stubRepo) FindByIDForAdmin(ctx context.Context, id, adminID uuid.UUID) (*ImportJob, error) {
	if r.findFn != nil {
		return r.findFn(ctx, id, adminID)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.created {
		if j.ID == id && j.AdminID != nil && *j.AdminID == adminID {
			return j, nil
		}
	}
	return nil, errors.New("not found")
}

func (r *stubRepo) SetStatus(ctx context.Context, id uuid.UUID, status Status, confirmedAt, completedAt *time.Time) error {
	if r.setStatusErr != nil {
		return r.setStatusErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusCalls = append(r.statusCalls, statusCall{id: id, status: status})
	for _, j := range r.created {
		if j.ID == id {
			j.Status = status
			if confirmedAt != nil {
				j.ConfirmedAt = confirmedAt
			}
			if completedAt != nil {
				j.CompletedAt = completedAt
			}
		}
	}
	return nil
}

func (r *stubRepo) SetCounts(ctx context.Context, id uuid.UUID, success, fail int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.created {
		if j.ID == id {
			j.SuccessCount = success
			j.FailCount = fail
		}
	}
	return nil
}

func (r *stubRepo) SetErrorsJSON(ctx context.Context, id uuid.UUID, errorsJSON []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.created {
		if j.ID == id {
			j.ErrorsJSON = errorsJSON
		}
	}
	return nil
}

func (r *stubRepo) SetCredentialsPath(ctx context.Context, id uuid.UUID, path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, j := range r.created {
		if j.ID == id {
			p := path
			j.CredentialsCSV = &p
		}
	}
	return nil
}

func newSvc(store storage.Storage, repo jobRepo) *Service {
	s := NewService(repo, store, 0)
	s.SetClock(func() time.Time {
		return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	})
	return s
}

func TestService_PreviewUpload_Happy(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	csv := []byte(`nama,email,kode_kelas
Andi,andi@a.id,KLS-1
Budi,budi@a.id,
Citra,citra@a.id,KLS-2
`)

	adminID := uuid.New()
	res, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID:  adminID,
		Filename: "/upload.csv",
		Body:     csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	if res.Job == nil || res.Job.ID == uuid.Nil {
		t.Fatal("expected Job with non-nil id")
	}
	if res.Job.Status != StatusPreview {
		t.Errorf("Status = %s, want preview", res.Job.Status)
	}
	if res.Job.TotalRows != 3 || res.Job.ValidCount != 3 || res.Job.InvalidCount != 0 {
		t.Errorf("counts = %+v", res.Job)
	}
	if res.Job.ObjectKey == nil {
		t.Fatal("ObjectKey should be populated")
	}
	if !strings.HasPrefix(*res.Job.ObjectKey, "import/") {
		t.Errorf("ObjectKey = %q, want prefix 'import/'", *res.Job.ObjectKey)
	}
	if res.Job.AdminID == nil || *res.Job.AdminID != adminID {
		t.Errorf("AdminID = %v, want %v", res.Job.AdminID, adminID)
	}
	wantExpiry := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	if !res.Job.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("ExpiresAt = %v, want %v", res.Job.ExpiresAt, wantExpiry)
	}

	// R2 has the object.
	if _, ok := store.objects[*res.Job.ObjectKey]; !ok {
		t.Errorf("object not stored at %q", *res.Job.ObjectKey)
	}

	// PreviewRowsJSON populated and parseable.
	var rows []Row
	if err := json.Unmarshal(res.Job.PreviewRowsJSON, &rows); err != nil {
		t.Fatalf("PreviewRowsJSON unmarshal: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("preview rows persisted = %d, want 3", len(rows))
	}

	// Repo received the row.
	if len(repo.created) != 1 || repo.created[0].ID != res.Job.ID {
		t.Errorf("repo.created = %+v", repo.created)
	}
}

func TestService_PreviewUpload_FilenameSanitized(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"upload.csv", "upload.csv"},
		{"/etc/passwd", "passwd.csv"},
		{"C:\\Users\\admin\\users.csv", "users.csv"},
		{"  ", "upload.csv"},
		{"plain", "plain.csv"},
		{"../../escape.csv", "escape.csv"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			store := newStubStore()
			repo := &stubRepo{}
			svc := newSvc(store, repo)
			_, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
				AdminID:  uuid.New(),
				Filename: tc.in,
				Body:     []byte("nama,email\nAndi,andi@a.id\n"),
			})
			if err != nil {
				t.Fatalf("PreviewUpload: %v", err)
			}
			got := repo.created[0].Filename
			if got != tc.want {
				t.Errorf("Filename = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestService_PreviewUpload_RejectsBinary(t *testing.T) {
	svc := newSvc(newStubStore(), &stubRepo{})
	// Random binary that DetectContentType won't classify as text/*.
	body := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	_, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID:  uuid.New(),
		Filename: "fake.csv",
		Body:     body,
	})
	if !errors.Is(err, ErrUnsupportedMime) {
		t.Fatalf("err = %v, want wraps ErrUnsupportedMime", err)
	}
}

func TestService_PreviewUpload_BubblesParserError(t *testing.T) {
	svc := newSvc(newStubStore(), &stubRepo{})
	_, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID:  uuid.New(),
		Filename: "x.csv",
		Body:     []byte("email\nx@y.id\n"), // no nama column
	})
	if !errors.Is(err, ErrMissingNamaColumn) {
		t.Fatalf("err = %v, want wraps ErrMissingNamaColumn", err)
	}
}

func TestService_PreviewUpload_R2PutFailure(t *testing.T) {
	store := newStubStore()
	store.putErr = errors.New("simulated r2 put failure")
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	_, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID:  uuid.New(),
		Filename: "x.csv",
		Body:     []byte("nama,email\nAndi,andi@a.id\n"),
	})
	if err == nil || !strings.Contains(err.Error(), "r2 put") {
		t.Fatalf("err = %v, want 'r2 put' wrapped error", err)
	}
	if len(repo.created) != 0 {
		t.Errorf("repo.created should be empty when R2 fails, got %d", len(repo.created))
	}
}

func TestService_PreviewUpload_DBFailureCleansR2(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{createErr: errors.New("db down")}
	svc := newSvc(store, repo)

	_, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID:  uuid.New(),
		Filename: "x.csv",
		Body:     []byte("nama,email\nAndi,andi@a.id\n"),
	})
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("err = %v, want wraps ErrPersistFailed", err)
	}
	if len(store.deleteCalls) != 1 {
		t.Fatalf("deleteCalls = %v, want exactly 1 compensating delete", store.deleteCalls)
	}
	if !strings.HasPrefix(store.deleteCalls[0], "import/") {
		t.Errorf("deleted key = %q, want 'import/' prefix", store.deleteCalls[0])
	}
	// R2 object should be gone.
	if len(store.objects) != 0 {
		t.Errorf("R2 still has %d objects after compensating delete", len(store.objects))
	}
}

func TestService_PreviewUpload_PreviewLimitCaps(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := NewService(repo, store, 5)
	svc.SetClock(func() time.Time { return time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) })

	// 10 rows, limit = 5.
	var buf bytes.Buffer
	buf.WriteString("nama,email\n")
	for i := 0; i < 10; i++ {
		buf.WriteString("Row")
		buf.WriteString(itoa(i))
		buf.WriteString(",row")
		buf.WriteString(itoa(i))
		buf.WriteString("@a.id\n")
	}
	res, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: uuid.New(),
		Filename: "x.csv",
		Body:    buf.Bytes(),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	if len(res.Rows) != 5 {
		t.Errorf("returned rows = %d, want 5", len(res.Rows))
	}
	// PreviewRowsJSON also capped.
	var stored []Row
	if err := json.Unmarshal(res.Job.PreviewRowsJSON, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 5 {
		t.Errorf("PreviewRowsJSON rows = %d, want 5", len(stored))
	}
	// But Stats reflects all 10.
	if res.ParseStats.Total != 10 {
		t.Errorf("Stats.Total = %d, want 10", res.ParseStats.Total)
	}
}

// itoa is a tiny strconv-free helper to keep this test file lightweight.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// --- GetPreview tests (Task 2.D.3) ---

func TestService_GetPreview_Happy(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	csv := []byte("nama,email\nAndi,andi@a.id\nBudi,budi@a.id\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	got, err := svc.GetPreview(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("GetPreview: %v", err)
	}
	if got.Job.ID != uploaded.Job.ID {
		t.Errorf("Job.ID = %v, want %v", got.Job.ID, uploaded.Job.ID)
	}
	if len(got.Rows) != 2 {
		t.Errorf("Rows = %d, want 2", len(got.Rows))
	}
	if got.Rows[0].Email != "andi@a.id" {
		t.Errorf("Rows[0].Email = %q, want andi@a.id", got.Rows[0].Email)
	}
}

func TestService_GetPreview_NotFound(t *testing.T) {
	svc := newSvc(newStubStore(), &stubRepo{})
	_, err := svc.GetPreview(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestService_GetPreview_WrongAdmin(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	owner := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: owner, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	other := uuid.New()
	_, err = svc.GetPreview(context.Background(), uploaded.Job.ID, other)
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound (cross-admin scope)", err)
	}
}

func TestService_GetPreview_Expired(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	// Advance clock past PreviewTTL.
	svc.SetClock(func() time.Time {
		return uploaded.Job.ExpiresAt.Add(time.Minute)
	})

	_, err = svc.GetPreview(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobExpired) {
		t.Fatalf("err = %v, want ErrJobExpired", err)
	}
}

func TestService_GetPreview_NotInPreview(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	// Simulate cancellation (or any non-preview state).
	uploaded.Job.Status = StatusCancelled

	_, err = svc.GetPreview(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobNotInPreview) {
		t.Fatalf("err = %v, want ErrJobNotInPreview", err)
	}
}

// --- Cancel tests (Task 2.D.3) ---

func TestService_Cancel_Happy(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	objKey := *uploaded.Job.ObjectKey
	if _, ok := store.objects[objKey]; !ok {
		t.Fatalf("setup: object should be in store before cancel")
	}

	res, err := svc.Cancel(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if res.Job.Status != StatusCancelled {
		t.Errorf("Status = %s, want cancelled", res.Job.Status)
	}
	if res.ObjectKey != objKey {
		t.Errorf("ObjectKey = %q, want %q", res.ObjectKey, objKey)
	}

	// SetStatus called once with cancelled.
	if len(repo.statusCalls) != 1 || repo.statusCalls[0].status != StatusCancelled {
		t.Errorf("statusCalls = %+v", repo.statusCalls)
	}
	// R2 object deleted.
	if _, ok := store.objects[objKey]; ok {
		t.Errorf("R2 object %q not deleted", objKey)
	}
	if len(store.deleteCalls) != 1 || store.deleteCalls[0] != objKey {
		t.Errorf("deleteCalls = %v, want [%q]", store.deleteCalls, objKey)
	}
}

func TestService_Cancel_NotFound(t *testing.T) {
	svc := newSvc(newStubStore(), &stubRepo{})
	_, err := svc.Cancel(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestService_Cancel_AlreadyCancelled(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	uploaded.Job.Status = StatusCancelled

	_, err = svc.Cancel(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobNotInPreview) {
		t.Fatalf("err = %v, want ErrJobNotInPreview (idempotent guard)", err)
	}
	// Must NOT call DeleteObject on second cancel.
	if len(store.deleteCalls) != 0 {
		t.Errorf("deleteCalls = %v, want empty (no double-delete)", store.deleteCalls)
	}
}

func TestService_Cancel_DBFailureSurfaces(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := newSvc(store, repo)

	adminID := uuid.New()
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	repo.setStatusErr = errors.New("db down")

	_, err = svc.Cancel(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("err = %v, want wraps ErrPersistFailed", err)
	}
	// R2 object must NOT be deleted if DB update failed.
	if _, ok := store.objects[*uploaded.Job.ObjectKey]; !ok {
		t.Errorf("R2 object should still exist when DB update fails")
	}
}

// --- Confirm tests (Task 2.D.4) ---

// stubUserCreator captures CreateUser + lookups for Confirm tests.
type stubUserCreator struct {
	mu       sync.Mutex
	created  []*auth.User
	emails   map[string]*auth.User // pre-seeded existing users
	createFn func(u *auth.User) error
	findErr  error
}

func newStubUsers() *stubUserCreator {
	return &stubUserCreator{emails: map[string]*auth.User{}}
}

func (s *stubUserCreator) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.emails[email]; ok {
		return u, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubUserCreator) CreateUser(ctx context.Context, u *auth.User) error {
	if s.createFn != nil {
		return s.createFn(u)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.emails[u.Email]; dup {
		return errors.New("duplicate key value violates unique constraint \"users_email_key\"")
	}
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	s.emails[u.Email] = u
	s.created = append(s.created, u)
	return nil
}

// stubKelasRepo captures FindByKodeInvite + Enroll for Confirm tests.
type stubKelasRepo struct {
	mu       sync.Mutex
	byKode   map[string]*kelas.Kelas
	enrolled []enrollCall
	findErr  error
	enrollFn func(kelasID, siswaID uuid.UUID, via kelas.JoinedVia) (bool, error)
}

type enrollCall struct {
	KelasID uuid.UUID
	SiswaID uuid.UUID
	Via     kelas.JoinedVia
}

func newStubKelas() *stubKelasRepo {
	return &stubKelasRepo{byKode: map[string]*kelas.Kelas{}}
}

func (s *stubKelasRepo) FindByKodeInvite(ctx context.Context, kode string) (*kelas.Kelas, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.byKode[kode]; ok {
		return k, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (s *stubKelasRepo) Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via kelas.JoinedVia) (bool, error) {
	if s.enrollFn != nil {
		return s.enrollFn(kelasID, siswaID, via)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrolled = append(s.enrolled, enrollCall{KelasID: kelasID, SiswaID: siswaID, Via: via})
	return true, nil
}

// confirmEnv builds a Service ready for Confirm with all dependencies wired.
func confirmEnv(t *testing.T) (*Service, *stubStore, *stubRepo, *stubUserCreator, *stubKelasRepo) {
	t.Helper()
	store := newStubStore()
	repo := &stubRepo{}
	users := newStubUsers()
	kr := newStubKelas()
	svc := newSvc(store, repo)
	svc.SetUserCreator(users)
	svc.SetKelasRepo(kr)
	svc.SetBcryptCost(bcrypt.MinCost) // fast tests
	return svc, store, repo, users, kr
}

func TestService_Confirm_Happy(t *testing.T) {
	svc, store, _, users, kr := confirmEnv(t)
	adminID := uuid.New()

	kelasID := uuid.New()
	kr.byKode["KLS-1"] = &kelas.Kelas{ID: kelasID, Nama: "Kelas Satu", KodeInvite: "KLS-1"}

	csv := []byte("nama,email,kode_kelas\n" +
		"Andi,andi@a.id,KLS-1\n" +
		"Budi,budi@a.id,\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	res, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if res.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", res.SuccessCount)
	}
	if res.FailCount != 0 {
		t.Errorf("FailCount = %d, want 0", res.FailCount)
	}
	if res.Job.Status != StatusCompleted {
		t.Errorf("Status = %s, want completed", res.Job.Status)
	}
	if res.CredentialsObjectKey == "" {
		t.Errorf("CredentialsObjectKey should be populated")
	}
	if !strings.HasPrefix(res.CredentialsObjectKey, "credentials/") {
		t.Errorf("credentials key = %q, want 'credentials/' prefix", res.CredentialsObjectKey)
	}
	if _, ok := store.objects[res.CredentialsObjectKey]; !ok {
		t.Errorf("R2 should contain credentials.csv at %s", res.CredentialsObjectKey)
	}
	if len(users.created) != 2 {
		t.Errorf("users created = %d, want 2", len(users.created))
	}
	for _, u := range users.created {
		if u.Role != auth.Siswa {
			t.Errorf("user %s role = %s, want siswa", u.Email, u.Role)
		}
		if !u.MustChangePassword {
			t.Errorf("user %s must_change_password should be true", u.Email)
		}
		if u.PasswordHash == "" {
			t.Errorf("user %s password_hash empty", u.Email)
		}
	}
	if len(kr.enrolled) != 1 {
		t.Errorf("enrolled calls = %d, want 1 (only Andi has kode_kelas)", len(kr.enrolled))
	}
}

func TestService_Confirm_DuplicateEmail(t *testing.T) {
	svc, _, _, users, _ := confirmEnv(t)
	adminID := uuid.New()

	// Pre-seed existing user with email that's also in CSV.
	users.emails["andi@a.id"] = &auth.User{ID: uuid.New(), Email: "andi@a.id"}

	csv := []byte("nama,email\nAndi,andi@a.id\nBudi,budi@a.id\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	res, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if res.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", res.SuccessCount)
	}
	if res.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", res.FailCount)
	}
	if len(res.Failures) != 1 || res.Failures[0].Reason != ConfirmReasonDuplicateInDB {
		t.Errorf("Failures = %+v, want one duplicate_in_db", res.Failures)
	}
}

func TestService_Confirm_KelasNotFound(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	adminID := uuid.New()

	csv := []byte("nama,email,kode_kelas\nAndi,andi@a.id,DOES-NOT-EXIST\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	res, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	// User created (we don't roll back on enroll failure).
	if res.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1 (user created despite kelas miss)", res.SuccessCount)
	}
	if res.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1 (kelas not found)", res.FailCount)
	}
	if len(res.Failures) != 1 || res.Failures[0].Reason != ConfirmReasonKelasNotFound {
		t.Errorf("Failures = %+v, want one kelas_not_found", res.Failures)
	}
}

func TestService_Confirm_IdempotentGuard(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	adminID := uuid.New()

	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nA,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	// First confirm: ok.
	if _, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	// Second confirm: status moved off preview → 409.
	_, err = svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobNotInPreview) {
		t.Fatalf("err = %v, want ErrJobNotInPreview (idempotent guard)", err)
	}
}

func TestService_Confirm_NotFoundCrossAdmin(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	owner := uuid.New()

	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: owner, Filename: "x.csv", Body: []byte("nama,email\nA,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	other := uuid.New()
	_, err = svc.Confirm(context.Background(), uploaded.Job.ID, other)
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound (cross-admin scope)", err)
	}
}

func TestService_Confirm_Expired(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	adminID := uuid.New()

	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nA,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	svc.SetClock(func() time.Time { return uploaded.Job.ExpiresAt.Add(time.Minute) })

	_, err = svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobExpired) {
		t.Fatalf("err = %v, want ErrJobExpired", err)
	}
}

func TestService_Confirm_DepsNotWired(t *testing.T) {
	store := newStubStore()
	repo := &stubRepo{}
	svc := NewService(repo, store, 0)
	// Don't wire users / kelasRepo.

	_, err := svc.Confirm(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrInternalConfirm) {
		t.Fatalf("err = %v, want ErrInternalConfirm", err)
	}
}

func TestService_Confirm_PartialInvalidRows(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	adminID := uuid.New()

	csv := []byte("nama,email\n" +
		"Andi,andi@a.id\n" +
		"BadGuy,not-an-email\n" +
		"Budi,budi@a.id\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	res, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if res.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", res.SuccessCount)
	}
	if res.FailCount != 1 {
		t.Errorf("FailCount = %d, want 1", res.FailCount)
	}
	if len(res.Failures) != 1 || res.Failures[0].Reason != ConfirmReasonInvalidRow {
		t.Errorf("Failures = %+v, want one invalid_row", res.Failures)
	}
}

func TestService_Confirm_GeneratedPasswordsAreUnique(t *testing.T) {
	svc, _, _, users, _ := confirmEnv(t)
	adminID := uuid.New()

	var buf bytes.Buffer
	buf.WriteString("nama,email\n")
	for i := 0; i < 8; i++ {
		buf.WriteString("U")
		buf.WriteString(itoa(i))
		buf.WriteString(",u")
		buf.WriteString(itoa(i))
		buf.WriteString("@a.id\n")
	}
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: buf.Bytes(),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	if _, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if len(users.created) != 8 {
		t.Fatalf("users created = %d, want 8", len(users.created))
	}
	seenHashes := map[string]struct{}{}
	for _, u := range users.created {
		if _, dup := seenHashes[u.PasswordHash]; dup {
			t.Errorf("duplicate password hash for %s — RNG broken", u.Email)
		}
		seenHashes[u.PasswordHash] = struct{}{}
	}
}

// --- DownloadCredentials tests (Task 2.D.5) ---

// runConfirmHappy runs PreviewUpload + Confirm against the confirmEnv harness
// and returns the completed ImportJob ID. Used by Download tests as setup.
func runConfirmHappy(t *testing.T, svc *Service, adminID uuid.UUID, kr *stubKelasRepo) uuid.UUID {
	t.Helper()
	kr.byKode["KLS-1"] = &kelas.Kelas{ID: uuid.New(), Nama: "Kelas Satu", KodeInvite: "KLS-1"}
	csv := []byte("nama,email,kode_kelas\nAndi,andi@a.id,KLS-1\nBudi,budi@a.id,\n")
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: csv,
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}
	if _, err := svc.Confirm(context.Background(), uploaded.Job.ID, adminID); err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	return uploaded.Job.ID
}

func TestService_DownloadCredentials_Happy(t *testing.T) {
	svc, _, _, _, kr := confirmEnv(t)
	adminID := uuid.New()
	jobID := runConfirmHappy(t, svc, adminID, kr)

	res, err := svc.DownloadCredentials(context.Background(), jobID, adminID)
	if err != nil {
		t.Fatalf("DownloadCredentials: %v", err)
	}
	if res.URL == "" || !strings.HasPrefix(res.URL, "stub://credentials/") {
		t.Errorf("URL = %q, want stub://credentials/... prefix", res.URL)
	}
	wantFilename := "credentials-" + jobID.String() + ".csv"
	if res.Filename != wantFilename {
		t.Errorf("Filename = %q, want %q", res.Filename, wantFilename)
	}
	if !strings.Contains(res.URL, "filename="+wantFilename) {
		t.Errorf("URL %q missing filename query param", res.URL)
	}
	if !strings.HasPrefix(res.ObjectKey, "credentials/") {
		t.Errorf("ObjectKey = %q, want 'credentials/' prefix", res.ObjectKey)
	}
	if res.TTL != 15*time.Minute {
		t.Errorf("TTL = %v, want 15m default", res.TTL)
	}
	// ExpiresAt = CompletedAt + 1h. CompletedAt = clock fn = 2026-05-20T12:00.
	wantExpires := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	if !res.ExpiresAt.Equal(wantExpires) {
		t.Errorf("ExpiresAt = %v, want %v", res.ExpiresAt, wantExpires)
	}
}

func TestService_DownloadCredentials_NotFound(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	_, err := svc.DownloadCredentials(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound", err)
	}
}

func TestService_DownloadCredentials_WrongAdmin(t *testing.T) {
	svc, _, _, _, kr := confirmEnv(t)
	owner := uuid.New()
	jobID := runConfirmHappy(t, svc, owner, kr)

	other := uuid.New()
	_, err := svc.DownloadCredentials(context.Background(), jobID, other)
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("err = %v, want ErrJobNotFound (cross-admin scope)", err)
	}
}

func TestService_DownloadCredentials_NotCompleted(t *testing.T) {
	svc, _, _, _, _ := confirmEnv(t)
	adminID := uuid.New()

	// Upload but DO NOT confirm — status stays preview.
	uploaded, err := svc.PreviewUpload(context.Background(), PreviewUploadInput{
		AdminID: adminID, Filename: "x.csv", Body: []byte("nama,email\nAndi,a@a.id\n"),
	})
	if err != nil {
		t.Fatalf("PreviewUpload: %v", err)
	}

	_, err = svc.DownloadCredentials(context.Background(), uploaded.Job.ID, adminID)
	if !errors.Is(err, ErrJobNotCompleted) {
		t.Fatalf("err = %v, want ErrJobNotCompleted", err)
	}
}

func TestService_DownloadCredentials_Expired(t *testing.T) {
	svc, _, _, _, kr := confirmEnv(t)
	adminID := uuid.New()
	jobID := runConfirmHappy(t, svc, adminID, kr)

	// Advance clock past CredentialsTTL (1h after CompletedAt).
	svc.SetClock(func() time.Time {
		return time.Date(2026, 5, 20, 13, 0, 1, 0, time.UTC)
	})

	_, err := svc.DownloadCredentials(context.Background(), jobID, adminID)
	if !errors.Is(err, ErrCredentialsExpired) {
		t.Fatalf("err = %v, want ErrCredentialsExpired", err)
	}
}

func TestService_DownloadCredentials_Missing(t *testing.T) {
	svc, store, repo, _, kr := confirmEnv(t)
	adminID := uuid.New()
	jobID := runConfirmHappy(t, svc, adminID, kr)

	// Drop the credentials object from the stub store to simulate cron
	// cleanup racing the download. Service should map ErrObjectNotFound
	// to ErrCredentialsMissing.
	for _, j := range repo.created {
		if j.ID == jobID && j.CredentialsCSV != nil {
			_ = store.DeleteObject(context.Background(), *j.CredentialsCSV)
		}
	}

	_, err := svc.DownloadCredentials(context.Background(), jobID, adminID)
	if !errors.Is(err, ErrCredentialsMissing) {
		t.Fatalf("err = %v, want ErrCredentialsMissing", err)
	}
}

func TestService_DownloadCredentials_PresignTTLOverride(t *testing.T) {
	svc, _, _, _, kr := confirmEnv(t)
	svc.SetPresignTTL(2 * time.Minute)
	adminID := uuid.New()
	jobID := runConfirmHappy(t, svc, adminID, kr)

	res, err := svc.DownloadCredentials(context.Background(), jobID, adminID)
	if err != nil {
		t.Fatalf("DownloadCredentials: %v", err)
	}
	if res.TTL != 2*time.Minute {
		t.Errorf("TTL = %v, want 2m (set via SetPresignTTL)", res.TTL)
	}
}
