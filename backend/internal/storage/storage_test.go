package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildKey_Happy(t *testing.T) {
	cases := []struct {
		name     string
		category string
		parts    []string
		want     string
	}{
		{"single part", CategoryImport, []string{"abc.csv"}, "import/abc.csv"},
		{"nested", CategoryImport, []string{"job-uuid", "preview.csv"}, "import/job-uuid/preview.csv"},
		{"trim slash", CategoryTugas, []string{"/foo/", "bar.pdf"}, "tugas/foo/bar.pdf"},
		{"multi seg", CategoryMateri, []string{"k1", "k2", "file.pdf"}, "materi/k1/k2/file.pdf"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildKey(tc.category, tc.parts...)
			if err != nil {
				t.Fatalf("BuildKey err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("BuildKey = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildKey_Errors(t *testing.T) {
	cases := []struct {
		name     string
		category string
		parts    []string
	}{
		{"unknown category", "ngaco", []string{"x"}},
		{"no parts", CategoryTugas, nil},
		{"empty part", CategoryTugas, []string{""}},
		{"whitespace part", CategoryTugas, []string{"   "}},
		{"traversal", CategoryTugas, []string{"..", "secret"}},
		{"backslash", CategoryTugas, []string{"a\\b"}},
		{"null byte", CategoryTugas, []string{"a\x00b"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildKey(tc.category, tc.parts...)
			if err == nil {
				t.Fatalf("BuildKey expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestIsValidCategory(t *testing.T) {
	for _, ok := range []string{CategoryTugas, CategorySoal, CategoryMateri, CategorySubmission, CategoryImport} {
		if !IsValidCategory(ok) {
			t.Fatalf("IsValidCategory(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "Tugas", "tugas/foo", "ngaco"} {
		if IsValidCategory(bad) {
			t.Fatalf("IsValidCategory(%q) = true, want false", bad)
		}
	}
}

func TestMockStorage_RoundTrip(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()

	body := []byte("hello world")
	if err := m.PutObject(ctx, PutObjectInput{
		Key:         "import/job-1/preview.csv",
		Body:        bytes.NewReader(body),
		Size:        int64(len(body)),
		ContentType: "text/csv",
	}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	if m.Len() != 1 {
		t.Fatalf("Len = %d, want 1", m.Len())
	}

	exists, err := m.ObjectExists(ctx, "import/job-1/preview.csv")
	if err != nil || !exists {
		t.Fatalf("ObjectExists = (%v,%v), want (true,nil)", exists, err)
	}

	missing, err := m.ObjectExists(ctx, "import/job-999/none.csv")
	if err != nil {
		t.Fatalf("ObjectExists missing err: %v", err)
	}
	if missing {
		t.Fatal("ObjectExists for missing key returned true")
	}

	got, err := m.GetObject(ctx, "import/job-1/preview.csv")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if got.ContentType != "text/csv" {
		t.Fatalf("ContentType = %q, want text/csv", got.ContentType)
	}
	if got.Size != int64(len(body)) {
		t.Fatalf("Size = %d, want %d", got.Size, len(body))
	}
	read, err := io.ReadAll(got.Body)
	if err != nil {
		t.Fatalf("ReadAll body: %v", err)
	}
	got.Body.Close()
	if !bytes.Equal(read, body) {
		t.Fatalf("body mismatch: got %q want %q", read, body)
	}
}

func TestMockStorage_GetMissing(t *testing.T) {
	m := NewMockStorage()
	_, err := m.GetObject(context.Background(), "import/x.csv")
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("err = %v, want wraps ErrObjectNotFound", err)
	}
}

func TestMockStorage_DeleteIdempotent(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()
	// Delete missing — must NOT error.
	if err := m.DeleteObject(ctx, "import/none"); err != nil {
		t.Fatalf("DeleteObject missing: %v", err)
	}
	// Put + delete + delete again.
	if err := m.PutObject(ctx, PutObjectInput{
		Key: "import/x.csv", Body: strings.NewReader("a"), Size: 1,
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := m.DeleteObject(ctx, "import/x.csv"); err != nil {
		t.Fatalf("Delete first: %v", err)
	}
	if err := m.DeleteObject(ctx, "import/x.csv"); err != nil {
		t.Fatalf("Delete second (idempotent): %v", err)
	}
	if m.Len() != 0 {
		t.Fatalf("Len = %d after delete, want 0", m.Len())
	}
}

func TestMockStorage_PresignGet(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	m.SetNowFn(func() time.Time { return now })

	// Missing key.
	if _, err := m.PresignGet(ctx, "import/missing", time.Minute); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("PresignGet missing = %v, want wraps ErrObjectNotFound", err)
	}

	// Present key.
	if err := m.PutObject(ctx, PutObjectInput{Key: "import/x.csv", Body: strings.NewReader("a")}); err != nil {
		t.Fatal(err)
	}
	got, err := m.PresignGet(ctx, "import/x.csv", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if !strings.HasPrefix(got, "mock://storage/import%2Fx.csv?") {
		t.Fatalf("PresignGet URL = %q, missing expected prefix", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("URL parse: %v", err)
	}
	if u.Query().Get("expires") == "" {
		t.Fatalf("URL has no expires query: %s", got)
	}
	wantExpires := now.Add(15 * time.Minute).Unix()
	if u.Query().Get("expires") != fmtInt(wantExpires) {
		t.Fatalf("expires = %s, want %d", u.Query().Get("expires"), wantExpires)
	}
}

func TestMockStorage_PutInvalid(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()

	// Empty key.
	if err := m.PutObject(ctx, PutObjectInput{Key: "", Body: strings.NewReader("a")}); err == nil {
		t.Fatal("Put with empty key should error")
	}
	// Nil body.
	if err := m.PutObject(ctx, PutObjectInput{Key: "import/x", Body: nil}); err == nil {
		t.Fatal("Put with nil body should error")
	}
}

func TestMockStorage_Concurrent(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()
	const N = 50

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key, _ := BuildKey(CategoryImport, "job", fmtInt(int64(i)))
			body := []byte(fmtInt(int64(i)))
			if err := m.PutObject(ctx, PutObjectInput{
				Key: key, Body: bytes.NewReader(body), Size: int64(len(body)),
			}); err != nil {
				t.Errorf("Put i=%d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	if m.Len() != N {
		t.Fatalf("Len = %d, want %d", m.Len(), N)
	}
}

func TestMockStorage_KeysCopyAndDownloadURL(t *testing.T) {
	ctx := context.Background()
	m := NewMockStorage()
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	m.SetNowFn(func() time.Time { return now })

	if err := m.PutObject(ctx, PutObjectInput{Key: "import/source.txt", Body: strings.NewReader("payload"), ContentType: "text/plain"}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	keys := m.Keys()
	if len(keys) != 1 || keys[0] != "import/source.txt" {
		t.Fatalf("Keys = %v, want source key", keys)
	}

	if err := m.CopyObject(ctx, "import/source.txt", "import/copy.txt"); err != nil {
		t.Fatalf("CopyObject: %v", err)
	}
	if err := m.DeleteObject(ctx, "import/source.txt"); err != nil {
		t.Fatalf("DeleteObject source: %v", err)
	}
	copyObj, err := m.GetObject(ctx, "import/copy.txt")
	if err != nil {
		t.Fatalf("GetObject copy: %v", err)
	}
	defer copyObj.Body.Close()
	read, err := io.ReadAll(copyObj.Body)
	if err != nil {
		t.Fatalf("ReadAll copy: %v", err)
	}
	if string(read) != "payload" || copyObj.ContentType != "text/plain" {
		t.Fatalf("copy = (%q,%q), want payload/text", string(read), copyObj.ContentType)
	}

	gotURL, err := m.PresignGetDownload(ctx, "import/copy.txt", 0, "laporan final.csv")
	if err != nil {
		t.Fatalf("PresignGetDownload: %v", err)
	}
	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("URL parse: %v", err)
	}
	if u.Query().Get("filename") != "laporan final.csv" {
		t.Fatalf("filename query = %q", u.Query().Get("filename"))
	}
	if u.Query().Get("expires") != fmtInt(now.Add(15*time.Minute).Unix()) {
		t.Fatalf("expires query = %q", u.Query().Get("expires"))
	}
}

func TestMockStorage_CopyObjectErrorsAndContext(t *testing.T) {
	m := NewMockStorage()
	ctx := context.Background()
	if err := m.CopyObject(ctx, "", "dst"); err == nil {
		t.Fatal("CopyObject with empty key should error")
	}
	if err := m.CopyObject(ctx, "missing", "dst"); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("CopyObject missing = %v, want ErrObjectNotFound", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := m.CopyObject(cancelled, "src", "dst"); !errors.Is(err, context.Canceled) {
		t.Fatalf("CopyObject canceled = %v, want context.Canceled", err)
	}
	if _, err := m.PresignGetDownload(cancelled, "src", time.Minute, "x.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("PresignGetDownload canceled = %v, want context.Canceled", err)
	}
}

func TestMockStorage_SetNowFnNilUsesLiveClock(t *testing.T) {
	m := NewMockStorage()
	m.SetNowFn(nil)
	if err := m.PutObject(context.Background(), PutObjectInput{Key: "import/x", Body: strings.NewReader("x")}); err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	before := time.Now().Add(14 * time.Minute).Unix()
	gotURL, err := m.PresignGet(context.Background(), "import/x", 15*time.Minute)
	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	after := time.Now().Add(16 * time.Minute).Unix()
	u, err := url.Parse(gotURL)
	if err != nil {
		t.Fatalf("URL parse: %v", err)
	}
	expires := u.Query().Get("expires")
	if expires < fmtInt(before) || expires > fmtInt(after) {
		t.Fatalf("expires = %s, want between %d and %d", expires, before, after)
	}
}

func TestLegacyInitAndPath(t *testing.T) {
	root := t.TempDir()
	if err := Init(""); err == nil {
		t.Fatal("Init empty root should error")
	}
	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, cat := range []string{CategoryTugas, CategorySoal, CategoryMateri, CategorySubmission, CategoryImport, CategoryCredentials, CategoryBankSoal} {
		if info, err := os.Stat(root + "/" + cat); err != nil || !info.IsDir() {
			t.Fatalf("category dir %s: info=%v err=%v", cat, info, err)
		}
	}
	got, err := Path(root, CategoryTugas, "file.pdf")
	if err != nil || got != filepath.Join(root, CategoryTugas, "file.pdf") {
		t.Fatalf("Path = (%q,%v)", got, err)
	}
	if _, err := Path(root, "bad", "file.pdf"); err == nil {
		t.Fatal("Path unknown category should error")
	}
	if _, err := Path(root, CategoryTugas, ""); err == nil {
		t.Fatal("Path empty filename should error")
	}
}

func TestR2Config_IsConfigured(t *testing.T) {
	cases := []struct {
		name string
		cfg  R2Config
		want bool
	}{
		{"all set", R2Config{AccountID: "a", AccessKeyID: "b", SecretAccessKey: "c", Bucket: "d"}, true},
		{"empty", R2Config{}, false},
		{"missing bucket", R2Config{AccountID: "a", AccessKeyID: "b", SecretAccessKey: "c"}, false},
		{"missing secret", R2Config{AccountID: "a", AccessKeyID: "b", Bucket: "d"}, false},
		{"whitespace only", R2Config{AccountID: "  ", AccessKeyID: "  ", SecretAccessKey: "  ", Bucket: "  "}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsConfigured(); got != tc.want {
				t.Fatalf("IsConfigured = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewStorage_FallbackToMock(t *testing.T) {
	got, err := NewStorage(R2Config{}, FactoryOptions{AllowMockFallback: true})
	if err != nil {
		t.Fatalf("NewStorage err: %v", err)
	}
	if _, ok := got.(*MockStorage); !ok {
		t.Fatalf("expected *MockStorage, got %T", got)
	}
}

func TestNewStorage_FailWithoutFallback(t *testing.T) {
	_, err := NewStorage(R2Config{}, FactoryOptions{AllowMockFallback: false})
	if err == nil {
		t.Fatal("expected error when R2 not configured + fallback disabled")
	}
}

func TestNewStorage_WithCredsTriesR2(t *testing.T) {
	// With non-empty creds, NewStorage attempts to construct an R2Client.
	// We don't hit the network here (no actual API call until first method),
	// so this should succeed even with bogus creds.
	cfg := R2Config{AccountID: "acc", AccessKeyID: "k", SecretAccessKey: "s", Bucket: "b"}
	got, err := NewStorage(cfg, FactoryOptions{AllowMockFallback: true})
	if err != nil {
		t.Fatalf("NewStorage err: %v", err)
	}
	if _, ok := got.(*R2Client); !ok {
		t.Fatalf("expected *R2Client, got %T", got)
	}
}

// fmtInt is a tiny helper to avoid pulling strconv at every call site.
func fmtInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
