// Service layer untuk pengumuman: input validation, ownership/enrollment
// guards, optimistic concurrency, audit logging. Handler stays thin.
//
// Authorization:
//   - Create/Update/Delete: guru pemilik kelas atau admin.
//   - List/Get: guru pemilik kelas + admin (full visibility incl. archived);
//     siswa enrolled (status=published only — archived hidden).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #66 Pengumuman passive timestamp: tidak ada per-siswa read receipt.
package pengumuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/storage"
)

// Sentinel errors yang di-mapping ke HTTP status di handler.
var (
	ErrInvalidInput            = errors.New("pengumuman: invalid input")
	ErrNotFound                = errors.New("pengumuman: not found")
	ErrForbidden               = errors.New("pengumuman: forbidden")
	ErrKelasArchived           = errors.New("pengumuman: kelas archived")
	ErrBabNotInKelas           = errors.New("pengumuman: bab does not belong to this kelas")
	ErrIsiTooLong              = errors.New("pengumuman: isi exceeds size limit")
	ErrAttachmentUnsupported   = errors.New("pengumuman: attachment mime not allowed")
	ErrAttachmentTooLarge      = errors.New("pengumuman: attachment too large")
	ErrAttachmentMissing       = errors.New("pengumuman: attachment missing")
	ErrAttachmentUploadFailed  = errors.New("pengumuman: attachment upload failed")
	ErrAttachmentStorageNeeded = errors.New("pengumuman: storage unavailable")
)

// MaxJudulBytes caps the judul length (200 chars roughly).
const MaxJudulBytes = 200

// MaxIsiBytes caps the isi (markdown body) at 50KB — sama dengan materi
// markdown locked roadmap §3.C.2 untuk konsistensi.
const MaxIsiBytes = 50 * 1024

// MaxAttachmentBytes caps one pengumuman attachment at 20MB.
const MaxAttachmentBytes = 20 * 1024 * 1024

const AttachmentPresignTTL = 15 * time.Minute

const MaxAttachmentsPerPengumuman = 10

// repoAPI subset yang dipakai service. Interface dipisah supaya gampang
// di-mock di test.
type repoAPI interface {
	Create(ctx context.Context, p *Pengumuman) error
	FindByID(ctx context.Context, id uuid.UUID) (*Pengumuman, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Pengumuman, error)
	UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, isi string, status Status) error
	AddAttachment(ctx context.Context, a *Attachment) error
	CountAttachmentsByPengumuman(ctx context.Context, pengumumanID uuid.UUID) (int64, error)
	FindAttachmentByID(ctx context.Context, pengumumanID, attachmentID uuid.UUID) (*Attachment, error)
	DeleteAttachment(ctx context.Context, pengumumanID, attachmentID uuid.UUID) (string, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// kelasLookup hydrates kelas ownership/lifecycle.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// babLookup verifies bab→kelas ownership when BabID is supplied.
type babLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
}

// enrollmentLookup verifies siswa enrolment in the kelas (List/Get for siswa).
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// auditLogger lets the service write audit rows without a hard auth dep.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles pengumuman business logic.
type Service struct {
	repo   repoAPI
	kelas  kelasLookup
	bab    babLookup
	enroll enrollmentLookup
	audit  auditLogger
	store  storage.Storage
	now    func() time.Time
}

// NewService wires pengumuman Repo + kelas/bab/enrollment lookups + audit
// logger. Pass nil for `enroll` to disable siswa list/get path (used in
// tests that only exercise guru flows).
func NewService(repo repoAPI, kelas kelasLookup, bab babLookup, enroll enrollmentLookup, audit auditLogger) *Service {
	return &Service{repo: repo, kelas: kelas, bab: bab, enroll: enroll, audit: audit, now: time.Now}
}

func (s *Service) SetStorage(store storage.Storage) { s.store = store }

// ---------- Create ----------

// CreateInput holds fields for POST /kelas/:id/pengumuman.
type CreateInput struct {
	BabID *uuid.UUID
	Judul string
	Isi   string
}

// Create publishes a pengumuman. Owner-only + kelas active. BabID kalau
// dikasih wajib milik kelas yang sama. Status auto = 'published'.
func (s *Service) Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Pengumuman, error) {
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if len(judul) > MaxJudulBytes {
		return nil, fmt.Errorf("%w: judul exceeds %d chars", ErrInvalidInput, MaxJudulBytes)
	}
	isi := in.Isi
	if len(isi) > MaxIsiBytes {
		return nil, fmt.Errorf("%w: isi exceeds %d bytes", ErrIsiTooLong, MaxIsiBytes)
	}

	k, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	if in.BabID != nil {
		if err := s.assertBabInKelas(ctx, *in.BabID, kelasID); err != nil {
			return nil, err
		}
	}

	p := &Pengumuman{
		KelasID:     kelasID,
		BabID:       in.BabID,
		Judul:       judul,
		Isi:         isi,
		CreatedByID: callerID,
		Status:      StatusPublished,
		Version:     1,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("pengumuman create: %w", err)
	}

	s.logAudit(ctx, "pengumuman_created", &callerID, callerRole, &p.ID, &kelasID, ip, userAgent, map[string]any{
		"pengumuman_id": p.ID.String(),
		"bab_id":        babIDStr(in.BabID),
		"judul":         p.Judul,
	})
	return p, nil
}

// ---------- List ----------

// ListInput narrows ListByKelas results.
//
// BabID semantics (mirror materi):
//   - nil:                   no bab filter (return all in kelas)
//   - non-nil + zero UUID:   pin bab_id IS NULL (kelas-wide pengumuman)
//   - non-nil + real UUID:   pin bab_id = BabID
//
// Status: nil → guru sees all; siswa always pinned to &StatusPublished
// (handler logic). Limit caps result count (default 50 di handler).
type ListInput struct {
	BabID  *uuid.UUID
	Status *Status
	Limit  int
}

// ListByKelas returns pengumuman in a kelas. Authorization branches by
// role: guru/admin see all + can filter status; siswa enrolled forced
// to status='published'.
func (s *Service) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Pengumuman, error) {
	if callerRole == string(auth.Siswa) {
		// Verify kelas exists; collapse missing → forbidden (no info leak).
		if _, err := s.kelas.FindByID(ctx, kelasID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrForbidden
			}
			return nil, fmt.Errorf("pengumuman list kelas: %w", err)
		}
		if err := s.assertEnrolled(ctx, kelasID, callerID); err != nil {
			return nil, err
		}
		// Force published-only for siswa; ignore inbound status filter.
		pub := StatusPublished
		f := ListFilter{
			Status: &pub,
			Bab:    babFilterFromList(in.BabID),
			Limit:  in.Limit,
		}
		return s.repo.ListByKelas(ctx, kelasID, f)
	}

	// Guru/admin path — manage guard.
	if _, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	f := ListFilter{
		Status: in.Status,
		Bab:    babFilterFromList(in.BabID),
		Limit:  in.Limit,
	}
	return s.repo.ListByKelas(ctx, kelasID, f)
}

// ---------- Get ----------

// Get returns a pengumuman by id with role-based visibility.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Pengumuman, error) {
	p, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman get: %w", err)
	}

	if callerRole == string(auth.Siswa) {
		if p.Status != StatusPublished {
			// Siswa never sees archived pengumuman.
			return nil, ErrNotFound
		}
		if err := s.assertEnrolled(ctx, p.KelasID, callerID); err != nil {
			return nil, err
		}
		return p, nil
	}

	if _, err := s.findKelasOrForbidden(ctx, p.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return p, nil
}

// ---------- Attachment ----------

type AttachmentUploadInput struct {
	Filename string
	Body     []byte
}

type AttachmentURLResult struct {
	URL       string
	ExpiresAt time.Time
}

var allowedAttachmentMimes = map[string]string{
	"image/jpeg":      "jpg",
	"image/png":       "png",
	"image/webp":      "webp",
	"application/pdf": "pdf",
}

func (s *Service) UploadAttachment(ctx context.Context, id, callerID uuid.UUID, callerRole string, in AttachmentUploadInput, ip, userAgent string) (*Pengumuman, error) {
	if s.store == nil {
		return nil, ErrAttachmentStorageNeeded
	}
	if len(in.Body) == 0 {
		return nil, fmt.Errorf("%w: empty body", ErrInvalidInput)
	}
	if int64(len(in.Body)) > MaxAttachmentBytes {
		return nil, ErrAttachmentTooLarge
	}
	sniff := in.Body
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	mime := strings.TrimSpace(strings.SplitN(http.DetectContentType(sniff), ";", 2)[0])
	ext, ok := allowedAttachmentMimes[mime]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrAttachmentUnsupported, mime)
	}
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman attachment find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	objectKey, err := storage.BuildKey(storage.CategoryPengumuman, uuid.NewString()+"."+ext)
	if err != nil {
		return nil, fmt.Errorf("pengumuman attachment key: %w", err)
	}
	if err := s.store.PutObject(ctx, storage.PutObjectInput{Key: objectKey, Body: bytes.NewReader(in.Body), Size: int64(len(in.Body)), ContentType: mime}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAttachmentUploadFailed, err)
	}
	filename := strings.TrimSpace(filepath.Base(in.Filename))
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "lampiran." + ext
	}
	size := int64(len(in.Body))
	count, err := s.repo.CountAttachmentsByPengumuman(ctx, id)
	if err != nil {
		_ = s.store.DeleteObject(context.Background(), objectKey)
		return nil, fmt.Errorf("pengumuman attachment count: %w", err)
	}
	if count >= MaxAttachmentsPerPengumuman {
		_ = s.store.DeleteObject(context.Background(), objectKey)
		return nil, fmt.Errorf("%w: max %d attachments", ErrInvalidInput, MaxAttachmentsPerPengumuman)
	}
	if err := s.repo.AddAttachment(ctx, &Attachment{
		PengumumanID:     id,
		ObjectKey:        objectKey,
		OriginalFilename: filename,
		MimeType:         mime,
		SizeBytes:        size,
	}); err != nil {
		_ = s.store.DeleteObject(context.Background(), objectKey)
		return nil, fmt.Errorf("pengumuman attachment insert: %w", err)
	}
	return s.repo.FindByID(ctx, id)
}

func (s *Service) DeleteAttachment(ctx context.Context, id, attachmentID, callerID uuid.UUID, callerRole string, ip, userAgent string) (*Pengumuman, error) {
	if s.store == nil {
		return nil, ErrAttachmentStorageNeeded
	}
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman attachment find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	if _, err := s.repo.FindAttachmentByID(ctx, id, attachmentID); errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAttachmentMissing
	} else if err != nil {
		return nil, fmt.Errorf("pengumuman attachment find row: %w", err)
	}
	oldKey, err := s.repo.DeleteAttachment(ctx, id, attachmentID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	_ = s.store.DeleteObject(context.Background(), oldKey)
	return s.repo.FindByID(ctx, id)
}

func (s *Service) PresignAttachmentURL(ctx context.Context, id, attachmentID, callerID uuid.UUID, callerRole string) (*AttachmentURLResult, error) {
	if s.store == nil {
		return nil, ErrAttachmentStorageNeeded
	}
	p, err := s.Get(ctx, id, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	a, err := s.repo.FindAttachmentByID(ctx, p.ID, attachmentID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAttachmentMissing
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman attachment find row: %w", err)
	}
	url, err := s.store.PresignGet(ctx, a.ObjectKey, AttachmentPresignTTL)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, ErrAttachmentMissing
		}
		return nil, fmt.Errorf("pengumuman attachment presign: %w", err)
	}
	return &AttachmentURLResult{URL: url, ExpiresAt: s.now().Add(AttachmentPresignTTL)}, nil
}

// ---------- Update ----------

// UpdateInput is the PATCH payload. Pointer fields are optional ("nil =
// leave unchanged"). ExpectedVersion is required.
type UpdateInput struct {
	ExpectedVersion int
	Judul           *string
	Isi             *string
	Status          *Status
}

// Update applies a partial update with optimistic concurrency. Owner-only.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Pengumuman, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman update find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	judul := existing.Judul
	if in.Judul != nil {
		judul = strings.TrimSpace(*in.Judul)
		if judul == "" {
			return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
		}
		if len(judul) > MaxJudulBytes {
			return nil, fmt.Errorf("%w: judul exceeds %d chars", ErrInvalidInput, MaxJudulBytes)
		}
	}
	isi := existing.Isi
	if in.Isi != nil {
		if len(*in.Isi) > MaxIsiBytes {
			return nil, fmt.Errorf("%w: isi exceeds %d bytes", ErrIsiTooLong, MaxIsiBytes)
		}
		isi = *in.Isi
	}
	status := existing.Status
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: status must be published|archived", ErrInvalidInput)
		}
		status = *in.Status
	}

	if judul == existing.Judul && isi == existing.Isi && status == existing.Status {
		// No-op PATCH: return existing without bumping version.
		return existing, nil
	}

	if err := s.repo.UpdateBasic(ctx, id, in.ExpectedVersion, judul, isi, status); err != nil {
		return nil, mapRepoErr(err)
	}
	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("pengumuman update refetch: %w", err)
	}

	action := "pengumuman_updated"
	if existing.Status != status && status == StatusArchived {
		action = "pengumuman_archived"
	}
	s.logAudit(ctx, action, &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"pengumuman_id": id.String(),
		"old_status":    string(existing.Status),
		"new_status":    string(fresh.Status),
		"old_version":   existing.Version,
		"new_version":   fresh.Version,
	})
	return fresh, nil
}

// ---------- Delete ----------

// Delete hard-deletes a pengumuman. Owner-only. Audit-logged.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Pengumuman, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("pengumuman delete: %w", err)
	}

	s.logAudit(ctx, "pengumuman_deleted", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"pengumuman_id": id.String(),
		"judul":         existing.Judul,
		"status":        string(existing.Status),
	})
	return existing, nil
}

// ---------- Helpers ----------

// findKelasOrForbidden hydrates kelas + asserts callerID/role can manage it.
// Returns ErrNotFound if the kelas is missing, ErrForbidden if caller isn't
// owner/admin.
func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pengumuman kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

// assertEnrolled verifies the siswa has an active enrollment in kelas.
// Missing row OR status≠active → ErrForbidden.
func (s *Service) assertEnrolled(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	if s.enroll == nil {
		return fmt.Errorf("pengumuman: enrollment lookup not configured")
	}
	enr, err := s.enroll.FindEnrollment(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("pengumuman enrollment lookup: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return ErrForbidden
	}
	return nil
}

// assertBabInKelas verifies the bab exists and belongs to the same kelas.
func (s *Service) assertBabInKelas(ctx context.Context, babID, kelasID uuid.UUID) error {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: bab not found", ErrInvalidInput)
	}
	if err != nil {
		return fmt.Errorf("pengumuman bab lookup: %w", err)
	}
	if b.KelasID != kelasID {
		return ErrBabNotInKelas
	}
	return nil
}

// canManageKelas mirrors materi/bab convention.
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

// mapRepoErr converts repo-level sentinels into service-level sentinels.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	default:
		return fmt.Errorf("pengumuman repo: %w", err)
	}
}

// babFilterFromList interpretes nil / zero UUID / real UUID semantics for List.
func babFilterFromList(bid *uuid.UUID) BabFilter {
	if bid == nil {
		return BabFilter{Mode: BabFilterAny}
	}
	if *bid == uuid.Nil {
		return BabFilter{Mode: BabFilterNull}
	}
	return BabFilter{Mode: BabFilterEq, BabID: *bid}
}

func babIDStr(b *uuid.UUID) string {
	if b == nil {
		return ""
	}
	return b.String()
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

// logAudit is best-effort: a logging failure must not poison the user-facing
// success response, so we swallow the error here (matches kelas + bab + materi).
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "pengumuman"
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
