package importjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/middleware"
)

// stubUploadService satisfies uploadService for handler tests.
type stubUploadService struct {
	called bool
	gotIn  PreviewUploadInput
	res    *PreviewUploadResult
	err    error

	// Resume / Cancel hooks (Task 2.D.3).
	getRes    *GetPreviewResult
	getErr    error
	gotGetID  uuid.UUID
	gotGetAdm uuid.UUID

	cancelRes    *CancelResult
	cancelErr    error
	gotCancelID  uuid.UUID
	gotCancelAdm uuid.UUID

	// Confirm hooks (Task 2.D.4).
	confirmRes    *ConfirmResult
	confirmErr    error
	gotConfirmID  uuid.UUID
	gotConfirmAdm uuid.UUID

	// DownloadCredentials hooks (Task 2.D.5).
	downloadRes    *DownloadCredentialsResult
	downloadErr    error
	gotDownloadID  uuid.UUID
	gotDownloadAdm uuid.UUID
}

func (s *stubUploadService) PreviewUpload(ctx context.Context, in PreviewUploadInput) (*PreviewUploadResult, error) {
	s.called = true
	s.gotIn = in
	if s.err != nil {
		return nil, s.err
	}
	return s.res, nil
}

func (s *stubUploadService) GetPreview(ctx context.Context, id, adminID uuid.UUID) (*GetPreviewResult, error) {
	s.gotGetID = id
	s.gotGetAdm = adminID
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getRes, nil
}

func (s *stubUploadService) Cancel(ctx context.Context, id, adminID uuid.UUID) (*CancelResult, error) {
	s.gotCancelID = id
	s.gotCancelAdm = adminID
	if s.cancelErr != nil {
		return nil, s.cancelErr
	}
	return s.cancelRes, nil
}

func (s *stubUploadService) Confirm(ctx context.Context, id, adminID uuid.UUID) (*ConfirmResult, error) {
	s.gotConfirmID = id
	s.gotConfirmAdm = adminID
	if s.confirmErr != nil {
		return nil, s.confirmErr
	}
	return s.confirmRes, nil
}

func (s *stubUploadService) DownloadCredentials(ctx context.Context, id, adminID uuid.UUID) (*DownloadCredentialsResult, error) {
	s.gotDownloadID = id
	s.gotDownloadAdm = adminID
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	return s.downloadRes, nil
}

// stubAudit captures LogAudit calls.
type stubAudit struct {
	mu      stubAuditLock
	entries []*auth.AuditLog
	err     error
}

type stubAuditLock struct{}

func (s *stubAudit) LogAudit(ctx context.Context, entry *auth.AuditLog) error {
	if s.err != nil {
		return s.err
	}
	s.entries = append(s.entries, entry)
	return nil
}

// testApp wires Handler under a Fiber app with adminID injected via locals.
// BodyLimit is set generous enough to test our own MaxCSVBytes (5MB) check;
// production also has a generous BodyLimit per cfg.Storage.MaxTugasFileMB.
func testApp(t *testing.T, svc uploadService, audit auditLogger, adminID uuid.UUID) *fiber.App {
	t.Helper()
	app := fiber.New(fiber.Config{
		BodyLimit: MaxCSVBytes * 2, // generous; per-route handler enforces real limit
	})
	app.Use(middleware.RequestID())
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, adminID)
		c.Locals(middleware.LocalsUserRole, string(auth.Admin))
		return c.Next()
	})
	h := NewHandler(svc, audit)
	app.Post("/api/v1/admin/import-csv/upload", h.PreviewUpload)
	app.Get("/api/v1/admin/import-csv/:job_id", h.GetPreview)
	app.Post("/api/v1/admin/import-csv/:job_id/cancel", h.Cancel)
	app.Post("/api/v1/admin/import-csv/:job_id/confirm", h.Confirm)
	return app
}

// buildMultipart returns the body + content-type for a single-file POST.
func buildMultipart(t *testing.T, fieldName, filename string, body []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, err := mw.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := w.Write(body); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close mw: %v", err)
	}
	return &buf, mw.FormDataContentType()
}

func TestHandler_PreviewUpload_Happy(t *testing.T) {
	jobID := uuid.New()
	objKey := "import/" + jobID.String() + ".csv"
	expires := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	svc := &stubUploadService{
		res: &PreviewUploadResult{
			Job: &ImportJob{
				ID:        jobID,
				ObjectKey: &objKey,
				Filename:  "users.csv",
				Status:    StatusPreview,
				ExpiresAt: expires,
			},
			ParseStats: ParseStat{Total: 3, Valid: 3},
			Rows: []Row{
				{LineNo: 2, Nama: "Andi", Email: "andi@a.id", Status: RowValid},
				{LineNo: 3, Nama: "Budi", Email: "budi@a.id", Status: RowValid},
				{LineNo: 4, Nama: "Citra", Email: "citra@a.id", Status: RowValid},
			},
		},
	}
	audit := &stubAudit{}
	app := testApp(t, svc, audit, uuid.New())

	csv := []byte("nama,email\nAndi,andi@a.id\n")
	body, ct := buildMultipart(t, "file", "users.csv", csv)

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/upload", body, ct)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", resp.StatusCode, mustBody(resp))
	}
	if !svc.called {
		t.Fatal("service was not called")
	}
	if svc.gotIn.Filename != "users.csv" {
		t.Errorf("filename forwarded = %q, want users.csv", svc.gotIn.Filename)
	}
	if !bytes.Equal(svc.gotIn.Body, csv) {
		t.Errorf("body forwarded mismatch")
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["job_id"] != jobID.String() {
		t.Errorf("job_id = %v, want %s", got["job_id"], jobID)
	}
	if got["valid_count"] != float64(3) {
		t.Errorf("valid_count = %v, want 3", got["valid_count"])
	}
	if rows, _ := got["preview_rows"].([]any); len(rows) != 3 {
		t.Errorf("preview_rows length = %d, want 3", len(rows))
	}

	// Audit recorded.
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit.entries))
	}
	if audit.entries[0].Action != "import_csv_uploaded" {
		t.Errorf("audit action = %s", audit.entries[0].Action)
	}
}

func TestHandler_PreviewUpload_MissingFile(t *testing.T) {
	app := testApp(t, &stubUploadService{}, &stubAudit{}, uuid.New())

	// Empty multipart with no file part.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("notfile", "foo")
	mw.Close()

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/upload", &buf, mw.FormDataContentType())
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	body := mustBody(resp)
	if !strings.Contains(body, "missing_file") {
		t.Errorf("body = %s, want code missing_file", body)
	}
}

func TestHandler_PreviewUpload_OversizeRejected(t *testing.T) {
	app := testApp(t, &stubUploadService{}, &stubAudit{}, uuid.New())

	// 5MB + 1 byte
	big := bytes.Repeat([]byte("a"), MaxCSVBytes+1)
	body, ct := buildMultipart(t, "file", "huge.csv", big)
	resp := do(t, app, "POST", "/api/v1/admin/import-csv/upload", body, ct)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "file_too_large") {
		t.Errorf("expected code file_too_large")
	}
}

func TestHandler_PreviewUpload_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{"empty csv", ErrEmptyCSV, http.StatusBadRequest, "empty_csv"},
		{"missing nama", ErrMissingNamaColumn, http.StatusBadRequest, "missing_nama_column"},
		{"missing email", ErrMissingEmailColumn, http.StatusBadRequest, "missing_email_column"},
		{"too many rows", ErrTooManyRows, http.StatusBadRequest, "too_many_rows"},
		{"too large parser", ErrCSVTooLarge, http.StatusRequestEntityTooLarge, "csv_too_large"},
		{"invalid utf8", ErrInvalidUTF8, http.StatusBadRequest, "invalid_utf8"},
		{"unsupported mime", ErrUnsupportedMime, http.StatusUnsupportedMediaType, "unsupported_mime"},
		{"persist failed", ErrPersistFailed, http.StatusInternalServerError, "persist_failed"},
		{"unknown", errors.New("anything"), http.StatusInternalServerError, "internal"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubUploadService{err: tc.serviceErr}
			app := testApp(t, svc, &stubAudit{}, uuid.New())
			body, ct := buildMultipart(t, "file", "x.csv", []byte("nama,email\nAndi,andi@a.id\n"))
			resp := do(t, app, "POST", "/api/v1/admin/import-csv/upload", body, ct)
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, mustBody(resp))
			}
			if !strings.Contains(mustBody(resp), `"`+tc.wantCode+`"`) {
				t.Errorf("response missing code %q", tc.wantCode)
			}
		})
	}
}

func TestHandler_PreviewUpload_AuditFailureNotFatal(t *testing.T) {
	jobID := uuid.New()
	objKey := "import/" + jobID.String() + ".csv"
	svc := &stubUploadService{
		res: &PreviewUploadResult{
			Job: &ImportJob{
				ID:        jobID,
				ObjectKey: &objKey,
				Filename:  "x.csv",
				Status:    StatusPreview,
				ExpiresAt: time.Now().Add(time.Hour),
			},
			ParseStats: ParseStat{Total: 1, Valid: 1},
		},
	}
	audit := &stubAudit{err: errors.New("audit table down")}
	app := testApp(t, svc, audit, uuid.New())

	body, ct := buildMultipart(t, "file", "x.csv", []byte("nama,email\nAndi,andi@a.id\n"))
	resp := do(t, app, "POST", "/api/v1/admin/import-csv/upload", body, ct)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (audit failure must not fail the request)", resp.StatusCode)
	}
}

// --- GetPreview handler tests (Task 2.D.3) ---

func TestHandler_GetPreview_Happy(t *testing.T) {
	jobID := uuid.New()
	objKey := "import/" + jobID.String() + ".csv"
	expires := time.Date(2026, 5, 20, 13, 0, 0, 0, time.UTC)
	svc := &stubUploadService{
		getRes: &GetPreviewResult{
			Job: &ImportJob{
				ID:           jobID,
				Filename:     "users.csv",
				ObjectKey:    &objKey,
				Status:       StatusPreview,
				TotalRows:    3,
				ValidCount:   2,
				InvalidCount: 1,
				ExpiresAt:    expires,
			},
			Rows: []Row{
				{LineNo: 2, Nama: "Andi", Email: "andi@a.id", Status: RowValid},
				{LineNo: 3, Nama: "Budi", Email: "budi@a.id", Status: RowValid},
			},
		},
	}
	adminID := uuid.New()
	app := testApp(t, svc, &stubAudit{}, adminID)

	resp := do(t, app, "GET", "/api/v1/admin/import-csv/"+jobID.String(), nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, mustBody(resp))
	}
	if svc.gotGetID != jobID {
		t.Errorf("gotGetID = %v, want %v", svc.gotGetID, jobID)
	}
	if svc.gotGetAdm != adminID {
		t.Errorf("gotGetAdm = %v, want %v", svc.gotGetAdm, adminID)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["job_id"] != jobID.String() {
		t.Errorf("job_id = %v, want %s", got["job_id"], jobID)
	}
	if got["status"] != string(StatusPreview) {
		t.Errorf("status = %v, want preview", got["status"])
	}
	if rows, _ := got["preview_rows"].([]any); len(rows) != 2 {
		t.Errorf("preview_rows length = %d, want 2", len(rows))
	}
	if got["filename"] != "users.csv" {
		t.Errorf("filename = %v, want users.csv", got["filename"])
	}
}

func TestHandler_GetPreview_InvalidJobID(t *testing.T) {
	svc := &stubUploadService{}
	app := testApp(t, svc, &stubAudit{}, uuid.New())

	resp := do(t, app, "GET", "/api/v1/admin/import-csv/not-a-uuid", nil, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "invalid_job_id") {
		t.Errorf("expected code invalid_job_id")
	}
}

func TestHandler_GetPreview_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{"not found", ErrJobNotFound, http.StatusNotFound, "not_found"},
		{"expired", ErrJobExpired, http.StatusGone, "preview_expired"},
		{"not in preview", ErrJobNotInPreview, http.StatusConflict, "not_in_preview"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubUploadService{getErr: tc.serviceErr}
			app := testApp(t, svc, &stubAudit{}, uuid.New())
			resp := do(t, app, "GET", "/api/v1/admin/import-csv/"+uuid.New().String(), nil, "")
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, mustBody(resp))
			}
			if !strings.Contains(mustBody(resp), `"`+tc.wantCode+`"`) {
				t.Errorf("response missing code %q", tc.wantCode)
			}
		})
	}
}

// --- Cancel handler tests (Task 2.D.3) ---

func TestHandler_Cancel_Happy(t *testing.T) {
	jobID := uuid.New()
	objKey := "import/" + jobID.String() + ".csv"
	svc := &stubUploadService{
		cancelRes: &CancelResult{
			Job: &ImportJob{
				ID:        jobID,
				Filename:  "users.csv",
				ObjectKey: &objKey,
				Status:    StatusCancelled,
			},
			ObjectKey: objKey,
		},
	}
	audit := &stubAudit{}
	adminID := uuid.New()
	app := testApp(t, svc, audit, adminID)

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/"+jobID.String()+"/cancel", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, mustBody(resp))
	}
	if svc.gotCancelID != jobID {
		t.Errorf("gotCancelID = %v, want %v", svc.gotCancelID, jobID)
	}
	if svc.gotCancelAdm != adminID {
		t.Errorf("gotCancelAdm = %v, want %v", svc.gotCancelAdm, adminID)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != string(StatusCancelled) {
		t.Errorf("status = %v, want cancelled", got["status"])
	}

	// Audit recorded.
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit.entries))
	}
	if audit.entries[0].Action != "import_csv_cancelled" {
		t.Errorf("audit action = %s, want import_csv_cancelled", audit.entries[0].Action)
	}
}

func TestHandler_Cancel_AlreadyCancelled(t *testing.T) {
	svc := &stubUploadService{cancelErr: ErrJobNotInPreview}
	audit := &stubAudit{}
	app := testApp(t, svc, audit, uuid.New())

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/"+uuid.New().String()+"/cancel", nil, "")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "not_in_preview") {
		t.Errorf("expected code not_in_preview")
	}
	// No audit log on conflict.
	if len(audit.entries) != 0 {
		t.Errorf("audit entries = %d, want 0 on failure", len(audit.entries))
	}
}

func TestHandler_Cancel_NotFound(t *testing.T) {
	svc := &stubUploadService{cancelErr: ErrJobNotFound}
	app := testApp(t, svc, &stubAudit{}, uuid.New())

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/"+uuid.New().String()+"/cancel", nil, "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "not_found") {
		t.Errorf("expected code not_found")
	}
}

// --- Confirm handler tests (Task 2.D.4) ---

func TestHandler_Confirm_Happy(t *testing.T) {
	jobID := uuid.New()
	objKey := "import/" + jobID.String() + ".csv"
	credKey := "credentials/" + jobID.String() + ".csv"
	svc := &stubUploadService{
		confirmRes: &ConfirmResult{
			Job: &ImportJob{
				ID:        jobID,
				Filename:  "users.csv",
				ObjectKey: &objKey,
				Status:    StatusCompleted,
			},
			SuccessCount:         3,
			FailCount:            1,
			CredentialsObjectKey: credKey,
			Failures: []ConfirmFailure{
				{LineNo: 4, Email: "dup@a.id", Reason: ConfirmReasonDuplicateInDB},
			},
		},
	}
	audit := &stubAudit{}
	adminID := uuid.New()
	app := testApp(t, svc, audit, adminID)

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/"+jobID.String()+"/confirm", nil, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, mustBody(resp))
	}
	if svc.gotConfirmID != jobID {
		t.Errorf("gotConfirmID = %v, want %v", svc.gotConfirmID, jobID)
	}
	if svc.gotConfirmAdm != adminID {
		t.Errorf("gotConfirmAdm = %v, want %v", svc.gotConfirmAdm, adminID)
	}

	var got map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["status"] != string(StatusCompleted) {
		t.Errorf("status = %v, want completed", got["status"])
	}
	if got["success_count"] != float64(3) {
		t.Errorf("success_count = %v, want 3", got["success_count"])
	}
	if got["fail_count"] != float64(1) {
		t.Errorf("fail_count = %v, want 1", got["fail_count"])
	}
	if got["credentials_object_key"] != credKey {
		t.Errorf("credentials_object_key = %v, want %s", got["credentials_object_key"], credKey)
	}
	if failures, _ := got["failures"].([]any); len(failures) != 1 {
		t.Errorf("failures length = %d, want 1", len(failures))
	}

	// Audit recorded.
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit.entries))
	}
	if audit.entries[0].Action != "import_csv_confirmed" {
		t.Errorf("audit action = %s, want import_csv_confirmed", audit.entries[0].Action)
	}
}

func TestHandler_Confirm_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{"not found", ErrJobNotFound, http.StatusNotFound, "not_found"},
		{"not in preview", ErrJobNotInPreview, http.StatusConflict, "not_in_preview"},
		{"expired", ErrJobExpired, http.StatusGone, "preview_expired"},
		{"rows mismatch", ErrConfirmRowsMismatch, http.StatusConflict, "rows_mismatch"},
		{"internal confirm", ErrInternalConfirm, http.StatusInternalServerError, "confirm_failed"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubUploadService{confirmErr: tc.serviceErr}
			audit := &stubAudit{}
			app := testApp(t, svc, audit, uuid.New())
			resp := do(t, app, "POST", "/api/v1/admin/import-csv/"+uuid.New().String()+"/confirm", nil, "")
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, mustBody(resp))
			}
			if !strings.Contains(mustBody(resp), `"`+tc.wantCode+`"`) {
				t.Errorf("response missing code %q", tc.wantCode)
			}
			// No audit on failure.
			if len(audit.entries) != 0 {
				t.Errorf("audit entries = %d, want 0 on error", len(audit.entries))
			}
		})
	}
}

func TestHandler_Confirm_InvalidJobID(t *testing.T) {
	svc := &stubUploadService{}
	app := testApp(t, svc, &stubAudit{}, uuid.New())

	resp := do(t, app, "POST", "/api/v1/admin/import-csv/not-a-uuid/confirm", nil, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "invalid_job_id") {
		t.Errorf("expected code invalid_job_id")
	}
}

// --- DownloadCredentials tests (Task 2.D.5) ---

func TestHandler_DownloadCredentials_Happy(t *testing.T) {
	jobID := uuid.New()
	credKey := "credentials/" + jobID.String() + ".csv"
	completed := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	presignedURL := "https://r2.example.com/" + credKey + "?X-Amz-Sig=stub"
	svc := &stubUploadService{
		downloadRes: &DownloadCredentialsResult{
			Job: &ImportJob{
				ID:          jobID,
				Filename:    "users.csv",
				Status:      StatusCompleted,
				CompletedAt: &completed,
			},
			URL:       presignedURL,
			ObjectKey: credKey,
			Filename:  "credentials-" + jobID.String() + ".csv",
			TTL:       15 * time.Minute,
			ExpiresAt: completed.Add(time.Hour),
		},
	}
	audit := &stubAudit{}
	adminID := uuid.New()
	app := testApp(t, svc, audit, adminID)

	resp := do(t, app, "GET", "/api/v1/admin/import-csv/"+jobID.String()+"/credentials.csv", nil, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302; body=%s", resp.StatusCode, mustBody(resp))
	}
	if got := resp.Header.Get("Location"); got != presignedURL {
		t.Errorf("Location = %q, want %q", got, presignedURL)
	}
	if svc.gotDownloadID != jobID {
		t.Errorf("gotDownloadID = %v, want %v", svc.gotDownloadID, jobID)
	}
	if svc.gotDownloadAdm != adminID {
		t.Errorf("gotDownloadAdm = %v, want %v", svc.gotDownloadAdm, adminID)
	}

	// Audit recorded.
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit.entries))
	}
	if audit.entries[0].Action != "file_url_issued" {
		t.Errorf("audit action = %s, want file_url_issued", audit.entries[0].Action)
	}
	// Meta has stable shape.
	metaStr := string(audit.entries[0].Meta)
	if !strings.Contains(metaStr, credKey) {
		t.Errorf("audit meta missing object_key %q: %s", credKey, metaStr)
	}
	if !strings.Contains(metaStr, "credentials-"+jobID.String()+".csv") {
		t.Errorf("audit meta missing filename: %s", metaStr)
	}
	if !strings.Contains(metaStr, "\"ttl_sec\":900") {
		t.Errorf("audit meta missing ttl_sec=900: %s", metaStr)
	}
}

func TestHandler_DownloadCredentials_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		serviceErr error
		wantStatus int
		wantCode   string
	}{
		{"not found", ErrJobNotFound, http.StatusNotFound, "not_found"},
		{"not completed", ErrJobNotCompleted, http.StatusConflict, "not_completed"},
		{"expired", ErrCredentialsExpired, http.StatusGone, "credentials_expired"},
		{"missing", ErrCredentialsMissing, http.StatusNotFound, "credentials_missing"},
		{"internal download", ErrInternalDownload, http.StatusInternalServerError, "download_failed"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubUploadService{downloadErr: tc.serviceErr}
			audit := &stubAudit{}
			app := testApp(t, svc, audit, uuid.New())
			resp := do(t, app, "GET", "/api/v1/admin/import-csv/"+uuid.New().String()+"/credentials.csv", nil, "")
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, mustBody(resp))
			}
			if !strings.Contains(mustBody(resp), `"`+tc.wantCode+`"`) {
				t.Errorf("response missing code %q", tc.wantCode)
			}
			// No audit on failure.
			if len(audit.entries) != 0 {
				t.Errorf("audit entries = %d, want 0 on error", len(audit.entries))
			}
		})
	}
}

func TestHandler_DownloadCredentials_InvalidJobID(t *testing.T) {
	svc := &stubUploadService{}
	app := testApp(t, svc, &stubAudit{}, uuid.New())

	resp := do(t, app, "GET", "/api/v1/admin/import-csv/not-a-uuid/credentials.csv", nil, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if !strings.Contains(mustBody(resp), "invalid_job_id") {
		t.Errorf("expected code invalid_job_id")
	}
}

// --- helpers ---

func do(t *testing.T, app *fiber.App, method, path string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.RemoteAddr = "203.0.113.55:1234"
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func mustBody(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return string(b)
}
