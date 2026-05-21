package materi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/storage"
)

// ---------- shared fakes for service-level tests ----------

type fakeRepo struct {
	createFn       func(ctx context.Context, m *Materi) error
	findByIDFn     func(ctx context.Context, id uuid.UUID) (*Materi, error)
	maxUrutanFn    func(ctx context.Context, kelasID uuid.UUID, f BabFilter) (int, error)
	listByKelasFn  func(ctx context.Context, kelasID uuid.UUID, f BabFilter) ([]Materi, error)
	updateBasicFn  func(ctx context.Context, id uuid.UUID, expectedVersion int, judul, konten string, urutan int) error
	deleteFn       func(ctx context.Context, id uuid.UUID) (*string, error)
	markReadFn     func(ctx context.Context, materiID, siswaID uuid.UUID) (*Read, bool, error)
}

func (r *fakeRepo) Create(ctx context.Context, m *Materi) error {
	if r.createFn == nil {
		return nil
	}
	return r.createFn(ctx, m)
}
func (r *fakeRepo) FindByID(ctx context.Context, id uuid.UUID) (*Materi, error) {
	return r.findByIDFn(ctx, id)
}
func (r *fakeRepo) MaxUrutan(ctx context.Context, kelasID uuid.UUID, f BabFilter) (int, error) {
	if r.maxUrutanFn == nil {
		return 0, nil
	}
	return r.maxUrutanFn(ctx, kelasID, f)
}
func (r *fakeRepo) ListByKelas(ctx context.Context, kelasID uuid.UUID, f BabFilter) ([]Materi, error) {
	return r.listByKelasFn(ctx, kelasID, f)
}
func (r *fakeRepo) UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, konten string, urutan int) error {
	return r.updateBasicFn(ctx, id, expectedVersion, judul, konten, urutan)
}
func (r *fakeRepo) Delete(ctx context.Context, id uuid.UUID) (*string, error) {
	return r.deleteFn(ctx, id)
}
func (r *fakeRepo) MarkRead(ctx context.Context, materiID, siswaID uuid.UUID) (*Read, bool, error) {
	if r.markReadFn == nil {
		return &Read{MateriID: materiID, SiswaID: siswaID, ReadAt: time.Now()}, true, nil
	}
	return r.markReadFn(ctx, materiID, siswaID)
}

type fakeKelas struct {
	rec *kelas.Kelas
	err error
}

func (k *fakeKelas) FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
	return k.rec, k.err
}

type fakeBab struct {
	rec *bab.Bab
	err error
}

func (b *fakeBab) FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error) {
	return b.rec, b.err
}

type fakeAudit struct {
	entries []*auth.AuditLog
}

func (a *fakeAudit) LogAudit(ctx context.Context, entry *auth.AuditLog) error {
	a.entries = append(a.entries, entry)
	return nil
}

// failingDeleteStorage wraps MockStorage but forces DeleteObject errors so we
// can exercise the compensating-delete + R2-orphan audit path.
type failingDeleteStorage struct {
	*storage.MockStorage
	deleteErr error
}

func (f *failingDeleteStorage) DeleteObject(ctx context.Context, key string) error {
	return f.deleteErr
}

// fakeEnroll satisfies enrollmentLookup for upload_test fixtures (most
// tests don't exercise MarkRead path; fakeEnroll defaults to nil enroll
// arg, but a few tests in read_test.go re-use the helper with a real
// stub).
type fakeEnroll struct {
	rec *kelas.Enrollment
	err error
}

func (e *fakeEnroll) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error) {
	if e.err != nil {
		return nil, e.err
	}
	return e.rec, nil
}

// helper to seed a small valid PDF body so http.DetectContentType returns
// application/pdf without us needing to embed a real PDF.
func pdfBody() []byte {
	// "%PDF-1.4\n" + minimal trailer-ish content; sufficient for sniff.
	return []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< >>\n%%EOF\n")
}

func newSvcWithStore(t *testing.T, store storage.Storage, repo *fakeRepo, k *kelas.Kelas, b *bab.Bab) (*Service, *fakeAudit) {
	t.Helper()
	audit := &fakeAudit{}
	kl := &fakeKelas{rec: k}
	if k == nil {
		kl.err = gorm.ErrRecordNotFound
	}
	bb := &fakeBab{rec: b}
	if b == nil {
		bb.err = gorm.ErrRecordNotFound
	}
	return NewService(repo, kl, bb, audit, store, nil), audit
}

func ownedKelas(guruID uuid.UUID) *kelas.Kelas {
	return &kelas.Kelas{ID: uuid.New(), GuruID: guruID}
}

// ---------- Service.Upload ----------

func TestService_Upload_Happy(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	var captured *Materi
	repo := &fakeRepo{
		createFn: func(ctx context.Context, m *Materi) error {
			m.ID = uuid.New()
			captured = m
			return nil
		},
	}
	svc, audit := newSvcWithStore(t, store, repo, k, nil)

	row, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul:            "Bab 1 PDF",
		OriginalFilename: "../etc/passwd/lecture-1.pdf",
		Body:             pdfBody(),
	}, "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("upload err: %v", err)
	}
	if row != captured {
		t.Fatalf("captured row mismatch")
	}
	if row.Tipe != TipePDF {
		t.Fatalf("tipe = %s want pdf", row.Tipe)
	}
	if row.ObjectKey == nil || !strings.HasPrefix(*row.ObjectKey, "materi/") || !strings.HasSuffix(*row.ObjectKey, ".pdf") {
		t.Fatalf("object_key shape wrong: %v", row.ObjectKey)
	}
	if row.OriginalFilename == nil || strings.Contains(*row.OriginalFilename, "/") || strings.Contains(*row.OriginalFilename, "..") {
		t.Fatalf("original_filename not sanitized: %v", row.OriginalFilename)
	}
	if row.SizeBytes == nil || *row.SizeBytes != int64(len(pdfBody())) {
		t.Fatalf("size_bytes mismatch: %v", row.SizeBytes)
	}
	if store.Len() != 1 {
		t.Fatalf("expected 1 R2 object, got %d", store.Len())
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "materi_uploaded" {
		t.Fatalf("expected materi_uploaded audit, got %+v", audit.entries)
	}
}

func TestService_Upload_MimeMismatch(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	svc, _ := newSvcWithStore(t, store, &fakeRepo{}, k, nil)

	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul: "Bukan PDF",
		Body:  []byte("plain text definitely not a pdf"),
	}, "ip", "ua")
	if !errors.Is(err, ErrUnsupportedMime) {
		t.Fatalf("expected ErrUnsupportedMime, got %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("R2 must stay empty on mime mismatch, got %d", store.Len())
	}
}

func TestService_Upload_PayloadTooLarge(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	svc, _ := newSvcWithStore(t, store, &fakeRepo{}, k, nil)

	body := make([]byte, MaxMateriBytes+1)
	copy(body, pdfBody())
	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul: "Big",
		Body:  body,
	}, "ip", "ua")
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
	}
}

func TestService_Upload_DBFail_CompensatingR2Delete(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	repo := &fakeRepo{
		createFn: func(ctx context.Context, m *Materi) error { return errors.New("db pq: connection refused") },
	}
	svc, _ := newSvcWithStore(t, store, repo, k, nil)

	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul: "Will fail",
		Body:  pdfBody(),
	}, "ip", "ua")
	if err == nil {
		t.Fatalf("expected error from db insert")
	}
	if store.Len() != 0 {
		t.Fatalf("compensating R2 delete failed; %d orphan objects left", store.Len())
	}
}

func TestService_Upload_KelasArchived(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	now := nowVar()
	k.ArchivedAt = &now
	store := storage.NewMockStorage()
	svc, _ := newSvcWithStore(t, store, &fakeRepo{}, k, nil)

	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul: "x", Body: pdfBody(),
	}, "ip", "ua")
	if !errors.Is(err, ErrKelasArchived) {
		t.Fatalf("expected ErrKelasArchived, got %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("R2 must stay empty when kelas archived")
	}
}

func TestService_Upload_BabNotInKelas(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	otherKelas := uuid.New()
	otherBab := &bab.Bab{ID: uuid.New(), KelasID: otherKelas}
	store := storage.NewMockStorage()
	svc, _ := newSvcWithStore(t, store, &fakeRepo{}, k, otherBab)

	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		BabID: &otherBab.ID,
		Judul: "x",
		Body:  pdfBody(),
	}, "ip", "ua")
	if !errors.Is(err, ErrBabNotInKelas) {
		t.Fatalf("expected ErrBabNotInKelas, got %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("R2 must stay empty when bab→kelas FK violated")
	}
}

func TestService_Upload_R2Required(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	svc, _ := newSvcWithStore(t, nil, &fakeRepo{}, k, nil)

	_, err := svc.Upload(context.Background(), k.ID, guruID, string(auth.Guru), UploadInput{
		Judul: "x", Body: pdfBody(),
	}, "ip", "ua")
	if !errors.Is(err, ErrR2Required) {
		t.Fatalf("expected ErrR2Required, got %v", err)
	}
}

// ---------- Service.PresignFileURL ----------

func TestService_Presign_PDF_Happy(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	objectKey := "materi/abc.pdf"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: objectKey, Body: bytes.NewReader(pdfBody()), Size: int64(len(pdfBody())), ContentType: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}
	mid := uuid.New()
	original := "lecture.pdf"
	mime := "application/pdf"
	size := int64(len(pdfBody()))
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: id, KelasID: k.ID, Tipe: TipePDF, ObjectKey: &objectKey, OriginalFilename: &original, MimeType: &mime, SizeBytes: &size}, nil
		},
	}
	svc, audit := newSvcWithStore(t, store, repo, k, nil)

	res, err := svc.PresignFileURL(context.Background(), mid, guruID, string(auth.Guru), "ip", "ua")
	if err != nil {
		t.Fatalf("presign err: %v", err)
	}
	if res.URL == "" {
		t.Fatalf("expected URL")
	}
	if res.OriginalFilename != original {
		t.Fatalf("original mismatch: %s", res.OriginalFilename)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "materi_file_url_issued" {
		t.Fatalf("expected materi_file_url_issued audit, got %+v", audit.entries)
	}
}

func TestService_Presign_RejectsNonPDF(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	mid := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: id, KelasID: k.ID, Tipe: TipeMarkdown}, nil
		},
	}
	svc, _ := newSvcWithStore(t, store, repo, k, nil)

	_, err := svc.PresignFileURL(context.Background(), mid, guruID, string(auth.Guru), "ip", "ua")
	if !errors.Is(err, ErrTipeUnsupported) {
		t.Fatalf("expected ErrTipeUnsupported, got %v", err)
	}
}

func TestService_Presign_NotFound(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc, _ := newSvcWithStore(t, store, repo, k, nil)

	_, err := svc.PresignFileURL(context.Background(), uuid.New(), guruID, string(auth.Guru), "ip", "ua")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ---------- Service.Delete (compensating R2 cleanup for tipe=pdf) ----------

func TestService_Delete_PDF_R2DeleteHappy(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	objectKey := "materi/zzz.pdf"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: objectKey, Body: bytes.NewReader(pdfBody()), Size: int64(len(pdfBody())), ContentType: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}
	mid := uuid.New()
	keyPtr := objectKey
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: mid, KelasID: k.ID, Tipe: TipePDF, ObjectKey: &keyPtr}, nil
		},
		deleteFn: func(ctx context.Context, id uuid.UUID) (*string, error) {
			return &keyPtr, nil
		},
	}
	svc, audit := newSvcWithStore(t, store, repo, k, nil)

	_, returnedKey, err := svc.Delete(context.Background(), mid, guruID, string(auth.Guru), "ip", "ua")
	if err != nil {
		t.Fatalf("delete err: %v", err)
	}
	if returnedKey == nil || *returnedKey != objectKey {
		t.Fatalf("returned key mismatch: %v", returnedKey)
	}
	if store.Len() != 0 {
		t.Fatalf("expected R2 empty after delete, got %d", store.Len())
	}
	// audit should be just materi_deleted (no orphan).
	if len(audit.entries) != 1 || audit.entries[0].Action != "materi_deleted" {
		t.Fatalf("expected single materi_deleted audit, got %+v", audit.entries)
	}
	if strings.Contains(string(audit.entries[0].Meta), "r2_orphan") {
		t.Fatalf("did not expect r2_orphan flag in audit: %s", audit.entries[0].Meta)
	}
}

func TestService_Delete_PDF_R2Fail_LogsOrphan(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := &failingDeleteStorage{
		MockStorage: storage.NewMockStorage(),
		deleteErr:   errors.New("r2: temporary failure"),
	}
	objectKey := "materi/aaa.pdf"
	if err := store.PutObject(context.Background(), storage.PutObjectInput{
		Key: objectKey, Body: bytes.NewReader(pdfBody()), Size: int64(len(pdfBody())), ContentType: "application/pdf",
	}); err != nil {
		t.Fatal(err)
	}
	mid := uuid.New()
	keyPtr := objectKey
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: mid, KelasID: k.ID, Tipe: TipePDF, ObjectKey: &keyPtr}, nil
		},
		deleteFn: func(ctx context.Context, id uuid.UUID) (*string, error) {
			return &keyPtr, nil
		},
	}
	svc, audit := newSvcWithStore(t, store, repo, k, nil)

	_, _, err := svc.Delete(context.Background(), mid, guruID, string(auth.Guru), "ip", "ua")
	if err != nil {
		t.Fatalf("delete should not surface R2 failure: %v", err)
	}
	// Two audit entries: materi_r2_orphan + materi_deleted (with r2_orphan flag).
	if len(audit.entries) != 2 {
		t.Fatalf("expected 2 audit entries, got %d: %+v", len(audit.entries), audit.entries)
	}
	if audit.entries[0].Action != "materi_r2_orphan" {
		t.Fatalf("expected materi_r2_orphan first, got %s", audit.entries[0].Action)
	}
	if audit.entries[1].Action != "materi_deleted" {
		t.Fatalf("expected materi_deleted second, got %s", audit.entries[1].Action)
	}
	if !strings.Contains(string(audit.entries[1].Meta), `"r2_orphan":true`) {
		t.Fatalf("expected r2_orphan:true in materi_deleted meta, got %s", audit.entries[1].Meta)
	}
}

func TestService_Delete_NonPDF_NoR2Call(t *testing.T) {
	guruID := uuid.New()
	k := ownedKelas(guruID)
	store := storage.NewMockStorage()
	mid := uuid.New()
	repo := &fakeRepo{
		findByIDFn: func(ctx context.Context, id uuid.UUID) (*Materi, error) {
			return &Materi{ID: mid, KelasID: k.ID, Tipe: TipeMarkdown}, nil
		},
		deleteFn: func(ctx context.Context, id uuid.UUID) (*string, error) {
			return nil, nil
		},
	}
	svc, audit := newSvcWithStore(t, store, repo, k, nil)

	_, returnedKey, err := svc.Delete(context.Background(), mid, guruID, string(auth.Guru), "ip", "ua")
	if err != nil {
		t.Fatalf("delete err: %v", err)
	}
	if returnedKey != nil {
		t.Fatalf("expected nil object key for markdown, got %v", returnedKey)
	}
	if len(audit.entries) != 1 || audit.entries[0].Action != "materi_deleted" {
		t.Fatalf("expected single materi_deleted audit, got %+v", audit.entries)
	}
}

// ---------- Handler.Upload (multipart) ----------

func TestHandler_Upload_HappyMultipart(t *testing.T) {
	guruID := uuid.New()
	kelasID := uuid.New()
	mid := uuid.New()
	objectKey := "materi/" + mid.String() + ".pdf"
	original := "lecture.pdf"
	size := int64(len(pdfBody()))

	svc := &stubSvc{
		uploadFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in UploadInput, ip, ua string) (*Materi, error) {
			if kID != kelasID {
				t.Fatalf("kelasID mismatch")
			}
			if in.OriginalFilename != "lecture.pdf" {
				t.Fatalf("original filename mismatch: %s", in.OriginalFilename)
			}
			mime := "application/pdf"
			return &Materi{
				ID: mid, KelasID: kID, Judul: in.Judul, Tipe: TipePDF,
				ObjectKey: &objectKey, OriginalFilename: &original, MimeType: &mime, SizeBytes: &size,
				Urutan: 1, Version: 1,
			}, nil
		},
	}
	app := newUploadApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	resp, body := doMultipart(t, app, "POST", "/kelas/"+kelasID.String()+"/materi/upload", map[string]string{
		"judul": "Bab 1",
	}, "file", "lecture.pdf", "application/pdf", pdfBody())
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"object_key":"`+objectKey+`"`) {
		t.Fatalf("expected object_key in body, got %s", body)
	}
	if !strings.Contains(string(body), `"original_filename":"lecture.pdf"`) {
		t.Fatalf("expected original_filename in body, got %s", body)
	}
}

func TestHandler_Upload_MimeMismatch_415(t *testing.T) {
	svc := &stubSvc{
		uploadFn: func(ctx context.Context, kID, cID uuid.UUID, role string, in UploadInput, ip, ua string) (*Materi, error) {
			return nil, fmt.Errorf("%w: bad", ErrUnsupportedMime)
		},
	}
	app := newUploadApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	resp, body := doMultipart(t, app, "POST", "/kelas/"+uuid.NewString()+"/materi/upload", map[string]string{
		"judul": "x",
	}, "file", "fake.pdf", "application/octet-stream", []byte("not a pdf at all"))
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `"code":"unsupported_mime"`) {
		t.Fatalf("expected code=unsupported_mime, got %s", body)
	}
}

func TestHandler_Upload_MissingFile_400(t *testing.T) {
	svc := &stubSvc{}
	app := newUploadApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("judul", "x")
	_ = w.Close()
	req := httptest.NewRequest("POST", "/kelas/"+uuid.NewString()+"/materi/upload", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), `"code":"missing_file"`) {
		t.Fatalf("expected code=missing_file, got %s", respBody)
	}
}

// ---------- Handler.FileURL ----------

func TestHandler_FileURL_Happy(t *testing.T) {
	guruID := uuid.New()
	mid := uuid.New()
	svc := &stubSvc{
		presignFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*FileURLResult, error) {
			if id != mid {
				t.Fatalf("id mismatch")
			}
			return &FileURLResult{
				URL:              "https://r2.example/materi/" + mid.String() + ".pdf?sig=...",
				ExpiresAt:        nowVar(),
				OriginalFilename: "lecture.pdf",
				MimeType:         "application/pdf",
			}, nil
		},
	}
	app := newUploadApp(t, &Handler{svc: svc}, string(auth.Guru), guruID)
	req := httptest.NewRequest("GET", "/materi/"+mid.String()+"/file-url", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"url":"https://r2.example/`) {
		t.Fatalf("expected url in body, got %s", body)
	}
	if !strings.Contains(string(body), `"original_filename":"lecture.pdf"`) {
		t.Fatalf("expected original_filename in body, got %s", body)
	}
}

func TestHandler_FileURL_NonPDF_400(t *testing.T) {
	svc := &stubSvc{
		presignFn: func(ctx context.Context, id, cID uuid.UUID, role, ip, ua string) (*FileURLResult, error) {
			return nil, fmt.Errorf("%w: only pdf", ErrTipeUnsupported)
		},
	}
	app := newUploadApp(t, &Handler{svc: svc}, string(auth.Guru), uuid.New())
	req := httptest.NewRequest("GET", "/materi/"+uuid.NewString()+"/file-url", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"code":"tipe_unsupported"`) {
		t.Fatalf("expected code=tipe_unsupported, got %s", body)
	}
}

// ---------- helpers ----------

// newUploadApp adds /kelas/:id/materi/upload + /materi/:id/file-url routes
// to the app fixture so handler tests can hit upload-specific endpoints.
func newUploadApp(t *testing.T, h *Handler, role string, userID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, userID)
		c.Locals(middleware.LocalsUserRole, role)
		return c.Next()
	})
	app.Post("/kelas/:id/materi/upload", h.Upload)
	app.Get("/materi/:id/file-url", h.FileURL)
	return app
}

// doMultipart issues a multipart POST with text fields + a single file field.
func doMultipart(t *testing.T, app *fiber.App, method, path string, fields map[string]string, fileField, filename, mime string, body []byte) (*http.Response, []byte) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name=%q; filename=%q`, fileField, filename)}
	header["Content-Type"] = []string{mime}
	part, err := w.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, respBody
}

// nowVar returns time.Now() — wrapper kept so tests using ArchivedAt/
// ExpiresAt can read like nowVar() without each block re-importing time
// inline at the call site.
func nowVar() time.Time {
	return time.Now()
}
