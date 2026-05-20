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
	"github.com/pikip/lms/backend/internal/storage"
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
	return "stub://" + key, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// stubRepo captures inserts so tests can assert state without touching GORM.
type stubRepo struct {
	mu      sync.Mutex
	created []*ImportJob
	createErr error
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
