// Service layer untuk submission: input validation, ownership/enrollment
// guards, late detection, optimistic concurrency, audit logging.
//
// Authorization:
//   - Submit: siswa enrolled di kelas yang punya tugas. Tugas wajib
//     status='published'. Existing submission with status='graded' di-reject
//     409 already_graded (locked #73 — kalau guru salah grade, hapus +
//     siswa resubmit, bukan re-grade).
//   - Grade: guru pemilik kelas + admin. Submission wajib status='submitted'.
//   - Get/List: guru/admin owner full visibility; siswa hanya own.
//
// Locked decisions referenced:
//   - #20 Tugas BabID nullable.
//   - #46 attachment mime allowlist.
//   - #56 optimistic concurrency.
//   - #69 hard delete + R2 cleanup compensating.
//   - #70 single-row + version bump on resubmit.
//   - #71 late submission gating + penalty calc.
//   - #72 attachment policy: optional + WajibAttachment + cap 5×20MB.
//   - #73 SELECT FOR UPDATE + idempotent guard.
package submission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/storage"
	"github.com/pikip/lms/backend/internal/tugas"
)

// Sentinel errors mapped to HTTP status di handler.
var (
	ErrInvalidInput          = errors.New("submission: invalid input")
	ErrNotFound              = errors.New("submission: not found")
	ErrForbidden             = errors.New("submission: forbidden")
	ErrTugasNotPublished     = errors.New("submission: tugas not published")
	ErrDeadlinePassed        = errors.New("submission: deadline passed (late submission disabled)")
	ErrAlreadyGraded         = errors.New("submission: already graded")
	ErrAttachmentRequired    = errors.New("submission: attachment required")
	ErrAttachmentLimit       = errors.New("submission: attachment limit reached")
	ErrAttachmentTooLarge    = errors.New("submission: attachment too large")
	ErrUnsupportedMime       = errors.New("submission: attachment mime not allowed")
	ErrR2Required            = errors.New("submission: object store not configured")
	ErrAttachmentUploadFailed = errors.New("submission: attachment upload failed")
)

// MaxCatatanBytes caps the catatan markdown body at 50KB (mirror tugas
// deskripsi / pengumuman / materi markdown).
const MaxCatatanBytes = 50 * 1024

// MaxFeedbackBytes caps guru feedback at 5KB.
const MaxFeedbackBytes = 5 * 1024

// MaxAttachmentBytes — locked #72 (mirror tugas attachment cap).
const MaxAttachmentBytes int64 = 20 * 1024 * 1024

// MaxAttachmentsPerSubmission — locked #72.
const MaxAttachmentsPerSubmission = 5

// PresignTTL — locked #62 (15 min default).
const PresignTTL = 15 * time.Minute

// repoAPI is the subset of *Repo the service depends on.
type repoAPI interface {
	DB() *gorm.DB
	LockForUpdate(ctx context.Context, tx *gorm.DB, tugasID, siswaID uuid.UUID) (*Submission, error)
	LockByID(ctx context.Context, tx *gorm.DB, id uuid.UUID) (*Submission, error)
	Create(ctx context.Context, tx *gorm.DB, s *Submission) error
	FindByID(ctx context.Context, id uuid.UUID) (*Submission, error)
	FindByTugasSiswa(ctx context.Context, tugasID, siswaID uuid.UUID) (*Submission, error)
	ListByTugas(ctx context.Context, tugasID uuid.UUID, f StatusFilter) ([]Submission, error)
	UpdateOnResubmit(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, catatan string, isLate bool) error
	GradeUpdate(ctx context.Context, tx *gorm.DB, id uuid.UUID, expectedVersion int, fields map[string]any) error
	AddAttachment(ctx context.Context, tx *gorm.DB, a *Attachment) error
	FindAttachmentByID(ctx context.Context, submissionID, attachmentID uuid.UUID) (*Attachment, error)
	ListAttachmentsBySubmission(ctx context.Context, submissionID uuid.UUID) ([]Attachment, error)
	DeleteAttachmentsBySubmission(ctx context.Context, tx *gorm.DB, submissionID uuid.UUID) ([]string, error)
}

// tugasLookup hydrates tugas + ownership/lifecycle.
type tugasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*tugas.Tugas, error)
}

// kelasLookup hydrates kelas ownership.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// enrollmentLookup verifies siswa enrolment in the kelas.
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// auditLogger lets the service write audit rows without a hard auth dep.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles submission business logic.
type Service struct {
	repo   repoAPI
	tugas  tugasLookup
	kelas  kelasLookup
	enroll enrollmentLookup
	audit  auditLogger
	store  storage.Storage
	now    func() time.Time
}

// NewService wires submission Repo + tugas/kelas/enrollment lookups + audit
// logger + R2 storage. now defaults to time.Now (override di test).
func NewService(repo repoAPI, tg tugasLookup, kl kelasLookup, enr enrollmentLookup, audit auditLogger, store storage.Storage) *Service {
	return &Service{
		repo:   repo,
		tugas:  tg,
		kelas:  kl,
		enroll: enr,
		audit:  audit,
		store:  store,
		now:    time.Now,
	}
}

// SetNow overrides the clock for deterministic tests.
func (s *Service) SetNow(fn func() time.Time) { s.now = fn }

// ---------- Helpers ----------

func (s *Service) findTugasOrNotFound(ctx context.Context, id uuid.UUID) (*tugas.Tugas, error) {
	t, err := s.tugas.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission tugas find: %w", err)
	}
	return t, nil
}

func (s *Service) assertEnrolled(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	if s.enroll == nil {
		return fmt.Errorf("submission: enrollment lookup not configured")
	}
	enr, err := s.enroll.FindEnrollment(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("submission enrollment lookup: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return ErrForbidden
	}
	return nil
}

func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("submission kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

func canManageKelas(k *kelas.Kelas, callerID uuid.UUID, callerRole string) bool {
	if k == nil {
		return false
	}
	if callerRole == string(auth.Admin) {
		return true
	}
	if callerRole == string(auth.Guru) && k.GuruID == callerID {
		return true
	}
	return false
}

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func marshalMeta(fields map[string]any) datatypes.JSON {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

// logAudit best-effort: a logging failure must not poison success response.
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "submission"
	role := actorRole
	if actorID == nil {
		role = ""
	}
	entry := &auth.AuditLog{
		ActorID:       actorID,
		ActorRole:     ptrString(role),
		Action:        action,
		TargetType:    &targetType,
		TargetID:      targetID,
		TargetKelasID: targetKelasID,
		Meta:          marshalMeta(meta),
		IP:            ptrString(ip),
		UserAgent:     ptrString(userAgent),
		At:            s.now(),
	}
	_ = s.audit.LogAudit(ctx, entry)
}
