package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"sync"
	"time"
)

// MockStorage is an in-memory Storage implementation used by tests and as a
// dev fallback when R2 credentials are not configured. It is safe for
// concurrent use.
//
// PresignGet returns a stable opaque URL of the form
// "mock://storage/<key>?expires=<unix>" — useful for assertions in tests
// without standing up a real HTTP server.
type MockStorage struct {
	mu      sync.RWMutex
	objects map[string]mockObject
	nowFn   func() time.Time
}

type mockObject struct {
	body        []byte
	contentType string
}

// NewMockStorage returns an empty in-memory store.
func NewMockStorage() *MockStorage {
	return &MockStorage{
		objects: make(map[string]mockObject),
		nowFn:   time.Now,
	}
}

// SetNowFn overrides the clock used by PresignGet (test hook).
func (m *MockStorage) SetNowFn(fn func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fn == nil {
		fn = time.Now
	}
	m.nowFn = fn
}

// Len returns the number of stored objects (test helper).
func (m *MockStorage) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.objects)
}

// Keys returns a snapshot of all stored keys (test helper).
func (m *MockStorage) Keys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.objects))
	for k := range m.objects {
		out = append(out, k)
	}
	return out
}

// PutObject implements Storage.
func (m *MockStorage) PutObject(ctx context.Context, in PutObjectInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.Key == "" {
		return fmt.Errorf("storage: empty key")
	}
	if in.Body == nil {
		return fmt.Errorf("storage: nil body for key %q", in.Key)
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return fmt.Errorf("storage: read body for key %q: %w", in.Key, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[in.Key] = mockObject{
		body:        body,
		contentType: in.ContentType,
	}
	return nil
}

// GetObject implements Storage.
func (m *MockStorage) GetObject(ctx context.Context, key string) (*Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	obj, ok := m.objects[key]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	return &Object{
		Key:         key,
		Size:        int64(len(obj.body)),
		ContentType: obj.contentType,
		Body:        io.NopCloser(bytes.NewReader(obj.body)),
	}, nil
}

// DeleteObject implements Storage. Missing keys are NOT an error (idempotent).
func (m *MockStorage) DeleteObject(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.objects, key)
	m.mu.Unlock()
	return nil
}

// ObjectExists implements Storage.
func (m *MockStorage) ObjectExists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.RLock()
	_, ok := m.objects[key]
	m.mu.RUnlock()
	return ok, nil
}

// PresignGet implements Storage. Returns a stable mock URL whose query
// "expires" timestamp is testable via SetNowFn.
func (m *MockStorage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return m.presign(ctx, key, ttl, "")
}

// PresignGetDownload implements Storage. Embeds the requested filename in a
// query parameter so tests can assert on it without parsing a real
// Content-Disposition header.
func (m *MockStorage) PresignGetDownload(ctx context.Context, key string, ttl time.Duration, filename string) (string, error) {
	return m.presign(ctx, key, ttl, filename)
}

func (m *MockStorage) presign(ctx context.Context, key string, ttl time.Duration, filename string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m.mu.RLock()
	_, ok := m.objects[key]
	now := m.nowFn()
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	expires := now.Add(ttl).Unix()
	q := url.Values{}
	q.Set("expires", fmt.Sprintf("%d", expires))
	if filename != "" {
		q.Set("filename", filename)
	}
	return fmt.Sprintf("mock://storage/%s?%s", url.PathEscape(key), q.Encode()), nil
}

// Compile-time check.
var _ Storage = (*MockStorage)(nil)
