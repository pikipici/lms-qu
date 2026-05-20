package admin

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
)

// fakeUserRepo lets us script per-id behavior for the user lookups.
type fakeUserRepo struct {
	users   map[uuid.UUID]*auth.User
	auditCh []*auth.AuditLog
	failOn  uuid.UUID // simulate transient FindUserByID error
}

func (f *fakeUserRepo) FindUserByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	if id == f.failOn {
		return nil, errBoom
	}
	u, ok := f.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return u, nil
}

func (f *fakeUserRepo) LogAudit(_ context.Context, e *auth.AuditLog) error {
	f.auditCh = append(f.auditCh, e)
	return nil
}

// fakeKelasRepo backs FindByID + FindEnrollment + Enroll. Behavior is keyed on
// (kelas, siswa) so tests can preload state.
type fakeKelasRepo struct {
	kelas       map[uuid.UUID]*kelas.Kelas
	enrollments map[string]*kelas.Enrollment
	enrollErr   map[uuid.UUID]error // siswa-keyed Enroll injection
	findEnrErr  map[uuid.UUID]error // siswa-keyed FindEnrollment injection
}

func enrollKey(kelasID, siswaID uuid.UUID) string {
	return kelasID.String() + "|" + siswaID.String()
}

func (f *fakeKelasRepo) FindByID(_ context.Context, id uuid.UUID) (*kelas.Kelas, error) {
	k, ok := f.kelas[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return k, nil
}

func (f *fakeKelasRepo) FindEnrollment(_ context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error) {
	if err, ok := f.findEnrErr[siswaID]; ok {
		return nil, err
	}
	e, ok := f.enrollments[enrollKey(kelasID, siswaID)]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return e, nil
}

func (f *fakeKelasRepo) Enroll(_ context.Context, kelasID, siswaID uuid.UUID, via kelas.JoinedVia) (bool, error) {
	if err, ok := f.enrollErr[siswaID]; ok {
		return false, err
	}
	key := enrollKey(kelasID, siswaID)
	if _, exists := f.enrollments[key]; exists {
		return false, nil
	}
	f.enrollments[key] = &kelas.Enrollment{
		KelasID: kelasID, SiswaID: siswaID,
		Status: kelas.EnrollmentActive, JoinedVia: via, JoinedAt: time.Now(),
	}
	return true, nil
}

var errBoom = &fakeErr{msg: "boom"}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

// newTestApp builds a fiber app with the admin role injected via a tiny
// middleware that mirrors what middleware.BearerAuth does (sets user_id /
// role into Locals). We bypass actual JWT verification.
func newTestApp(t *testing.T, adminID uuid.UUID, h *KelasEnrollHandler) *fiber.App {
	t.Helper()
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", adminID)
		c.Locals("user_role", string(auth.Admin))
		return c.Next()
	})
	app.Post("/api/v1/admin/kelas/:id/enroll", h.BulkEnroll)
	return app
}

func doJSON(app *fiber.App, method, path, body string) (*http.Response, []byte) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, -1)
	defer resp.Body.Close()
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return resp, buf
}

func TestBulkEnroll_HappyPath_Mixed(t *testing.T) {
	adminID := uuid.New()
	kelasID := uuid.New()
	siswaActive := uuid.New()
	siswaPriorActive := uuid.New()
	siswaRemoved := uuid.New()
	siswaGuru := uuid.New()
	siswaSuspended := uuid.New()

	users := &fakeUserRepo{users: map[uuid.UUID]*auth.User{
		siswaActive:      {ID: siswaActive, Role: auth.Siswa, Status: auth.Active},
		siswaPriorActive: {ID: siswaPriorActive, Role: auth.Siswa, Status: auth.Active},
		siswaRemoved:     {ID: siswaRemoved, Role: auth.Siswa, Status: auth.Active},
		siswaGuru:        {ID: siswaGuru, Role: auth.Guru, Status: auth.Active},
		siswaSuspended:   {ID: siswaSuspended, Role: auth.Siswa, Status: auth.Suspended},
	}}
	k := &kelas.Kelas{ID: kelasID, Nama: "Matematika", GuruID: uuid.New(), Version: 1}
	krepo := &fakeKelasRepo{
		kelas:       map[uuid.UUID]*kelas.Kelas{kelasID: k},
		enrollments: map[string]*kelas.Enrollment{},
	}
	// Preload prior states.
	krepo.enrollments[enrollKey(kelasID, siswaPriorActive)] = &kelas.Enrollment{
		KelasID: kelasID, SiswaID: siswaPriorActive,
		Status: kelas.EnrollmentActive, JoinedVia: kelas.JoinedViaKode,
	}
	krepo.enrollments[enrollKey(kelasID, siswaRemoved)] = &kelas.Enrollment{
		KelasID: kelasID, SiswaID: siswaRemoved,
		Status: kelas.EnrollmentRemoved, JoinedVia: kelas.JoinedViaKode,
	}

	h := NewKelasEnrollHandler(users, krepo)
	app := newTestApp(t, adminID, h)

	body := `{"siswa_ids":["` + siswaActive.String() + `","` + siswaPriorActive.String() +
		`","` + siswaRemoved.String() + `","` + siswaGuru.String() +
		`","` + siswaSuspended.String() + `","not-a-uuid","` + siswaActive.String() + `"]}`
	resp, raw := doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, raw)
	}
	var out BulkEnrollResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Enrolled) != 1 || out.Enrolled[0].SiswaID != siswaActive.String() {
		t.Fatalf("enrolled mismatch: %+v", out.Enrolled)
	}
	if len(out.AlreadyEnrolled) != 1 || out.AlreadyEnrolled[0].SiswaID != siswaPriorActive.String() {
		t.Fatalf("already_enrolled mismatch: %+v", out.AlreadyEnrolled)
	}
	// invalid: removed, guru, suspended, not-a-uuid, duplicate (siswaActive 2nd) → 5
	if len(out.Invalid) != 5 {
		t.Fatalf("invalid count = %d, want 5; %+v", len(out.Invalid), out.Invalid)
	}
	reasons := map[string]int{}
	for _, it := range out.Invalid {
		reasons[it.Reason]++
	}
	wantReasons := map[string]int{
		ReasonEnrollmentRemoved:  1,
		ReasonNotSiswa:           1,
		ReasonUserInactive:       1,
		ReasonInvalidUUID:        1,
		ReasonDuplicateInRequest: 1,
	}
	for r, n := range wantReasons {
		if reasons[r] != n {
			t.Fatalf("reason %s = %d, want %d (got %v)", r, reasons[r], n, reasons)
		}
	}

	// Audit: 1 enrolled + 1 already_enrolled + 3 invalid_* (uuid + duplicate skip audit)
	if got := len(users.auditCh); got != 5 {
		t.Fatalf("audit rows = %d, want 5", got)
	}
}

func TestBulkEnroll_InvalidKelasID(t *testing.T) {
	h := NewKelasEnrollHandler(&fakeUserRepo{users: map[uuid.UUID]*auth.User{}}, &fakeKelasRepo{
		kelas: map[uuid.UUID]*kelas.Kelas{}, enrollments: map[string]*kelas.Enrollment{},
	})
	app := newTestApp(t, uuid.New(), h)
	resp, _ := doJSON(app, "POST", "/api/v1/admin/kelas/not-uuid/enroll", `{"siswa_ids":["x"]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestBulkEnroll_KelasNotFound(t *testing.T) {
	kelasID := uuid.New()
	h := NewKelasEnrollHandler(&fakeUserRepo{users: map[uuid.UUID]*auth.User{}}, &fakeKelasRepo{
		kelas: map[uuid.UUID]*kelas.Kelas{}, enrollments: map[string]*kelas.Enrollment{},
	})
	app := newTestApp(t, uuid.New(), h)
	resp, _ := doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll",
		`{"siswa_ids":["`+uuid.New().String()+`"]}`)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestBulkEnroll_KelasArchived(t *testing.T) {
	kelasID := uuid.New()
	now := time.Now()
	krepo := &fakeKelasRepo{
		kelas: map[uuid.UUID]*kelas.Kelas{
			kelasID: {ID: kelasID, Nama: "X", ArchivedAt: &now},
		},
		enrollments: map[string]*kelas.Enrollment{},
	}
	h := NewKelasEnrollHandler(&fakeUserRepo{users: map[uuid.UUID]*auth.User{}}, krepo)
	app := newTestApp(t, uuid.New(), h)
	resp, _ := doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll",
		`{"siswa_ids":["`+uuid.New().String()+`"]}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestBulkEnroll_EmptyAndOversize(t *testing.T) {
	kelasID := uuid.New()
	krepo := &fakeKelasRepo{
		kelas:       map[uuid.UUID]*kelas.Kelas{kelasID: {ID: kelasID, Nama: "X"}},
		enrollments: map[string]*kelas.Enrollment{},
	}
	h := NewKelasEnrollHandler(&fakeUserRepo{users: map[uuid.UUID]*auth.User{}}, krepo)
	app := newTestApp(t, uuid.New(), h)

	resp, _ := doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll", `{"siswa_ids":[]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty: status = %d, want 400", resp.StatusCode)
	}

	// Oversize: 101 ids
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = `"` + uuid.New().String() + `"`
	}
	resp, _ = doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll",
		`{"siswa_ids":[`+strings.Join(ids, ",")+`]}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversize: status = %d, want 400", resp.StatusCode)
	}
}

func TestBulkEnroll_InvalidBody(t *testing.T) {
	kelasID := uuid.New()
	krepo := &fakeKelasRepo{
		kelas:       map[uuid.UUID]*kelas.Kelas{kelasID: {ID: kelasID, Nama: "X"}},
		enrollments: map[string]*kelas.Enrollment{},
	}
	h := NewKelasEnrollHandler(&fakeUserRepo{users: map[uuid.UUID]*auth.User{}}, krepo)
	app := newTestApp(t, uuid.New(), h)
	resp, _ := doJSON(app, "POST", "/api/v1/admin/kelas/"+kelasID.String()+"/enroll", `not json`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
