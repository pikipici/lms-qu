package submission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
	"github.com/pikip/lms/backend/internal/storage"
	"github.com/pikip/lms/backend/internal/tugas"
)

type handlerRouteFixture struct {
	app     *fiber.App
	handler *Handler
	svc     *Service
	repo    *stubSubmissionRepo
	tugasID uuid.UUID
	subID   uuid.UUID
	attID   uuid.UUID
	siswaID uuid.UUID
	guruID  uuid.UUID
	kelasID uuid.UUID
}

type stubSubmissionRepo struct {
	sub      *Submission
	rows     []Submission
	mineRows []SubmissionWithTugas
	atts     []Attachment
}

func (r *stubSubmissionRepo) DB() *gorm.DB { return nil }
func (r *stubSubmissionRepo) LockForUpdate(ctx context.Context, tx *gorm.DB, tugasID, siswaID uuid.UUID) (*Submission, error) {
	return r.sub, nil
}
func (r *stubSubmissionRepo) LockByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*Submission, error) {
	return r.sub, nil
}
func (r *stubSubmissionRepo) Create(ctx context.Context, tx *gorm.DB, sub *Submission) error {
	sub.ID = uuid.New()
	r.sub = sub
	return nil
}
func (r *stubSubmissionRepo) FindByID(ctx context.Context, id uuid.UUID) (*Submission, error) {
	return r.sub, nil
}
func (r *stubSubmissionRepo) FindByTugasSiswa(ctx context.Context, tugasID, siswaID uuid.UUID) (*Submission, error) {
	return r.sub, nil
}
func (r *stubSubmissionRepo) ListByTugas(ctx context.Context, tugasID uuid.UUID, status StatusFilter) ([]Submission, error) {
	return r.rows, nil
}
func (r *stubSubmissionRepo) CountByTugas(ctx context.Context, tugasID uuid.UUID) (int64, error) {
	return int64(len(r.rows)), nil
}
func (r *stubSubmissionRepo) ListBySiswa(ctx context.Context, siswaID uuid.UUID) ([]Submission, error) {
	return r.rows, nil
}
func (r *stubSubmissionRepo) ListBySiswaWithTugas(ctx context.Context, siswaID uuid.UUID, limit int) ([]SubmissionWithTugas, error) {
	return r.mineRows, nil
}
func (r *stubSubmissionRepo) UpdateOnResubmit(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, catatan string, isLate bool) error {
	return nil
}
func (r *stubSubmissionRepo) GradeUpdate(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, fields map[string]any) error {
	if r.sub != nil {
		r.sub.Status = StatusGraded
	}
	return nil
}
func (r *stubSubmissionRepo) AddAttachment(ctx context.Context, tx *gorm.DB, att *Attachment) error {
	att.ID = uuid.New()
	r.atts = append(r.atts, *att)
	return nil
}
func (r *stubSubmissionRepo) FindAttachmentByID(ctx context.Context, submissionID, attachmentID uuid.UUID) (*Attachment, error) {
	if len(r.atts) > 0 {
		return &r.atts[0], nil
	}
	return &Attachment{ID: attachmentID, SubmissionID: submissionID, ObjectKey: "submission/file.txt", OriginalFilename: "file.txt", MimeType: "text/plain"}, nil
}
func (r *stubSubmissionRepo) ListAttachmentsBySubmission(ctx context.Context, submissionID uuid.UUID) ([]Attachment, error) {
	return r.atts, nil
}
func (r *stubSubmissionRepo) DeleteAttachmentsBySubmission(ctx context.Context, tx *gorm.DB, submissionID uuid.UUID) ([]string, error) {
	return nil, nil
}

type stubSubmissionAudit struct{}

func (stubSubmissionAudit) LogAudit(ctx context.Context, entry *auth.AuditLog) error { return nil }

type stubSubmissionTugasRepo struct{ row *tugas.Tugas }

func (r stubSubmissionTugasRepo) FindByID(ctx context.Context, id uuid.UUID) (*tugas.Tugas, error) {
	return r.row, nil
}

type stubSubmissionKelasRepo struct{ row *kelas.Kelas }

func (r stubSubmissionKelasRepo) FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error) {
	return r.row, nil
}

type stubSubmissionEnrollRepo struct{ row *kelas.Enrollment }

func (r stubSubmissionEnrollRepo) FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error) {
	return r.row, nil
}

func newHandlerRouteFixture(t *testing.T) handlerRouteFixture {
	t.Helper()
	kelasID, tugasID, subID, attID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	siswaID, guruID := uuid.New(), uuid.New()
	now := time.Now()
	tugasRow := &tugas.Tugas{ID: tugasID, KelasID: kelasID, Judul: "Tugas", Status: tugas.StatusPublished, Version: 1}
	sub := &Submission{ID: subID, TugasID: tugasID, SiswaID: siswaID, Status: StatusSubmitted, Version: 1, SubmittedAt: now}
	repo := &stubSubmissionRepo{
		sub:      sub,
		rows:     []Submission{*sub},
		mineRows: []SubmissionWithTugas{{Submission: *sub, TugasID: tugasID, KelasID: kelasID, Judul: "Tugas"}},
		atts:     []Attachment{{ID: attID, SubmissionID: subID, ObjectKey: "submission/file.txt", OriginalFilename: "file.txt", MimeType: "text/plain"}},
	}
	svc := NewService(repo, stubSubmissionTugasRepo{row: tugasRow}, stubSubmissionKelasRepo{row: &kelas.Kelas{ID: kelasID, GuruID: guruID}}, stubSubmissionEnrollRepo{row: &kelas.Enrollment{KelasID: kelasID, SiswaID: siswaID, Status: kelas.EnrollmentActive}}, stubSubmissionAudit{}, storage.NewMockStorage())
	h := NewHandler(svc)
	app := fiber.New()
	app.Use(middleware.RequestID())
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(middleware.LocalsUserID, siswaID)
		c.Locals(middleware.LocalsUserRole, string(auth.Siswa))
		if strings.Contains(c.Path(), "/guru/") || strings.Contains(c.Path(), "/grade") {
			c.Locals(middleware.LocalsUserID, guruID)
			c.Locals(middleware.LocalsUserRole, string(auth.Guru))
		}
		return c.Next()
	})
	app.Post("/siswa/tugas/:id/submit", h.Submit)
	app.Get("/siswa/tugas/:id/submission", h.GetMySubmission)
	app.Get("/guru/tugas/:id/submissions", h.ListByTugas)
	app.Get("/submission/:id", h.Get)
	app.Get("/submission/:id/attachments/:attID/url", h.AttachmentURL)
	app.Get("/siswa/submissions", h.ListMine)
	app.Post("/submission/:id/grade", h.Grade)
	return handlerRouteFixture{app: app, handler: h, svc: svc, repo: repo, tugasID: tugasID, subID: subID, attID: attID, siswaID: siswaID, guruID: guruID, kelasID: kelasID}
}

func TestHandlerSubmissionRoutesHappyPath(t *testing.T) {
	fx := newHandlerRouteFixture(t)

	assertJSONStatus(t, fx.app, httptest.NewRequest(http.MethodGet, "/siswa/tugas/"+fx.tugasID.String()+"/submission", nil), fiber.StatusOK)
	assertJSONStatus(t, fx.app, httptest.NewRequest(http.MethodGet, "/guru/tugas/"+fx.tugasID.String()+"/submissions?status=submitted", nil), fiber.StatusOK)
	assertJSONStatus(t, fx.app, httptest.NewRequest(http.MethodGet, "/submission/"+fx.subID.String(), nil), fiber.StatusOK)
	assertJSONStatus(t, fx.app, httptest.NewRequest(http.MethodGet, "/siswa/submissions?limit=5", nil), fiber.StatusOK)

}

func TestHandlerSubmissionRoutesValidationErrors(t *testing.T) {
	fx := newHandlerRouteFixture(t)
	tests := []struct {
		name string
		req  *http.Request
	}{
		{"submit invalid id", httptest.NewRequest(http.MethodPost, "/siswa/tugas/not-uuid/submit", nil)},
		{"get mine invalid id", httptest.NewRequest(http.MethodGet, "/siswa/tugas/not-uuid/submission", nil)},
		{"list invalid status", httptest.NewRequest(http.MethodGet, "/guru/tugas/"+fx.tugasID.String()+"/submissions?status=bogus", nil)},
		{"get invalid id", httptest.NewRequest(http.MethodGet, "/submission/not-uuid", nil)},
		{"attachment invalid sub", httptest.NewRequest(http.MethodGet, "/submission/not-uuid/attachments/"+fx.attID.String()+"/url", nil)},
		{"attachment invalid att", httptest.NewRequest(http.MethodGet, "/submission/"+fx.subID.String()+"/attachments/not-uuid/url", nil)},
		{"list mine invalid limit", httptest.NewRequest(http.MethodGet, "/siswa/submissions?limit=0", nil)},
		{"grade invalid id", httptest.NewRequest(http.MethodPost, "/submission/not-uuid/grade", strings.NewReader(`{}`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONStatus(t, fx.app, tt.req, fiber.StatusBadRequest)
		})
	}
}

func assertJSONStatus(t *testing.T, app *fiber.App, req *http.Request, want int) map[string]any {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return body
}
