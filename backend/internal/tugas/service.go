// Service layer untuk tugas: input validation, ownership/enrollment guards,
// optimistic concurrency, audit logging. Handler stays thin.
//
// Authorization:
//   - Create/Update/Delete: guru pemilik kelas atau admin.
//   - List/Get: guru pemilik kelas + admin (full visibility incl.
//     draft/archived); siswa enrolled (status=published only — draft +
//     archived hidden, no info leak via 404).
//
// Locked decisions referenced:
//   - #20 BabID nullable: tugas bisa kelas-wide atau bab-scoped.
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #69 hard delete + R2 cleanup compensating (handled di Task 4.A.3
//     untuk attachment; service.Delete returns ObjectKeys keluar).
//   - #71 late submission gating: validate penalty_persen 0-100.
//   - #72 wajib_attachment policy.
package tugas

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
)

// Sentinel errors mapped to HTTP status di handler.
var (
	ErrInvalidInput     = errors.New("tugas: invalid input")
	ErrNotFound         = errors.New("tugas: not found")
	ErrForbidden        = errors.New("tugas: forbidden")
	ErrKelasArchived    = errors.New("tugas: kelas archived")
	ErrBabNotInKelas    = errors.New("tugas: bab does not belong to this kelas")
	ErrDeskripsiTooLong = errors.New("tugas: deskripsi exceeds size limit")
)

// MaxJudulBytes caps the judul length (200 chars roughly).
const MaxJudulBytes = 200

// MaxDeskripsiBytes caps deskripsi (markdown body) at 50KB — sama dengan
// pengumuman/materi markdown untuk konsistensi.
const MaxDeskripsiBytes = 50 * 1024

// repoAPI is the subset of *Repo the service depends on. Interface dipisah
// supaya gampang di-mock di test.
type repoAPI interface {
	Create(ctx context.Context, t *Tugas) error
	FindByID(ctx context.Context, id uuid.UUID) (*Tugas, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f ListFilter) ([]Tugas, error)
	UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]any) error
	Delete(ctx context.Context, id uuid.UUID) ([]string, error)
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

// Service handles tugas business logic.
type Service struct {
	repo   repoAPI
	kelas  kelasLookup
	bab    babLookup
	enroll enrollmentLookup
	audit  auditLogger
	now    func() time.Time
}

// NewService wires tugas Repo + kelas/bab/enrollment lookups + audit logger.
// Pass nil for `enroll` to disable siswa list/get path (tests).
func NewService(repo repoAPI, kelas kelasLookup, bab babLookup, enroll enrollmentLookup, audit auditLogger) *Service {
	return &Service{repo: repo, kelas: kelas, bab: bab, enroll: enroll, audit: audit, now: time.Now}
}

// ---------- Create ----------

// CreateInput holds fields for POST /kelas/:id/tugas.
type CreateInput struct {
	BabID           *uuid.UUID
	Judul           string
	Deskripsi       string
	Deadline        *time.Time
	IzinkanLate     bool
	PenaltyPersen   int16
	WajibAttachment bool
	Status          *Status // optional; default = draft
}

// Create publishes a tugas. Owner-only + kelas active. BabID kalau dikasih
// wajib milik kelas yang sama. Status default = draft (siswa gak lihat
// sampai guru explicit publish).
func (s *Service) Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Tugas, error) {
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if len(judul) > MaxJudulBytes {
		return nil, fmt.Errorf("%w: judul exceeds %d chars", ErrInvalidInput, MaxJudulBytes)
	}
	deskripsi := in.Deskripsi
	if len(deskripsi) > MaxDeskripsiBytes {
		return nil, fmt.Errorf("%w: deskripsi exceeds %d bytes", ErrDeskripsiTooLong, MaxDeskripsiBytes)
	}
	if in.PenaltyPersen < 0 || in.PenaltyPersen > 100 {
		return nil, fmt.Errorf("%w: penalty_persen must be between 0 and 100", ErrInvalidInput)
	}
	status := StatusDraft
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: status must be draft|published|archived", ErrInvalidInput)
		}
		status = *in.Status
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

	t := &Tugas{
		KelasID:         kelasID,
		BabID:           in.BabID,
		Judul:           judul,
		Deskripsi:       deskripsi,
		Deadline:        in.Deadline,
		IzinkanLate:     in.IzinkanLate,
		PenaltyPersen:   in.PenaltyPersen,
		WajibAttachment: in.WajibAttachment,
		Status:          status,
		Version:         1,
		CreatedByID:     callerID,
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, fmt.Errorf("tugas create: %w", err)
	}

	s.logAudit(ctx, "tugas_created", &callerID, callerRole, &t.ID, &kelasID, ip, userAgent, map[string]any{
		"tugas_id":         t.ID.String(),
		"bab_id":           babIDStr(in.BabID),
		"judul":            t.Judul,
		"status":           string(t.Status),
		"deadline":         deadlineStr(t.Deadline),
		"izinkan_late":     t.IzinkanLate,
		"penalty_persen":   t.PenaltyPersen,
		"wajib_attachment": t.WajibAttachment,
	})
	return t, nil
}

// ---------- List ----------

// ListInput narrows ListByKelas results. Mirror pengumuman semantics.
//
// BabID:
//   - nil:                   no bab filter
//   - non-nil + zero UUID:   pin bab_id IS NULL (kelas-wide)
//   - non-nil + real UUID:   pin bab_id = BabID
//
// Status: nil → guru sees all; siswa always pinned to &StatusPublished.
type ListInput struct {
	BabID  *uuid.UUID
	Status *Status
	Limit  int
}

// ListByKelas returns tugas in a kelas. Authorization branches by role:
// guru/admin see all + can filter status; siswa enrolled forced to
// status='published'.
func (s *Service) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Tugas, error) {
	if callerRole == string(auth.Siswa) {
		if _, err := s.kelas.FindByID(ctx, kelasID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrForbidden
			}
			return nil, fmt.Errorf("tugas list kelas: %w", err)
		}
		if err := s.assertEnrolled(ctx, kelasID, callerID); err != nil {
			return nil, err
		}
		pub := StatusPublished
		f := ListFilter{
			Status: &pub,
			Bab:    babFilterFromList(in.BabID),
			Limit:  in.Limit,
		}
		return s.repo.ListByKelas(ctx, kelasID, f)
	}

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

// Get returns a tugas by id with role-based visibility.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Tugas, error) {
	t, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas get: %w", err)
	}

	if callerRole == string(auth.Siswa) {
		if t.Status != StatusPublished {
			return nil, ErrNotFound
		}
		if err := s.assertEnrolled(ctx, t.KelasID, callerID); err != nil {
			return nil, err
		}
		return t, nil
	}

	if _, err := s.findKelasOrForbidden(ctx, t.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return t, nil
}

// ---------- Update ----------

// UpdateInput is the PATCH payload. Pointer fields are optional. ExpectedVersion
// required.
type UpdateInput struct {
	ExpectedVersion int
	Judul           *string
	Deskripsi       *string
	BabID           *uuid.UUID // nil = leave unchanged; *uuid.Nil = clear (kelas-wide)
	BabIDExplicit   bool       // distinguish "field absent" vs "explicit null"
	Deadline        *time.Time
	DeadlineExplicit bool
	IzinkanLate     *bool
	PenaltyPersen   *int16
	WajibAttachment *bool
	Status          *Status
}

// Update applies a partial update with optimistic concurrency. Owner-only.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Tugas, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas update find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	fields := map[string]any{}

	if in.Judul != nil {
		j := strings.TrimSpace(*in.Judul)
		if j == "" {
			return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
		}
		if len(j) > MaxJudulBytes {
			return nil, fmt.Errorf("%w: judul exceeds %d chars", ErrInvalidInput, MaxJudulBytes)
		}
		if j != existing.Judul {
			fields["judul"] = j
		}
	}
	if in.Deskripsi != nil {
		if len(*in.Deskripsi) > MaxDeskripsiBytes {
			return nil, fmt.Errorf("%w: deskripsi exceeds %d bytes", ErrDeskripsiTooLong, MaxDeskripsiBytes)
		}
		if *in.Deskripsi != existing.Deskripsi {
			fields["deskripsi"] = *in.Deskripsi
		}
	}
	if in.BabIDExplicit {
		// Validate bab→kelas only if non-nil.
		if in.BabID != nil {
			if err := s.assertBabInKelas(ctx, *in.BabID, existing.KelasID); err != nil {
				return nil, err
			}
		}
		// Detect change.
		oldStr := ""
		if existing.BabID != nil {
			oldStr = existing.BabID.String()
		}
		newStr := ""
		if in.BabID != nil {
			newStr = in.BabID.String()
		}
		if oldStr != newStr {
			if in.BabID == nil {
				fields["bab_id"] = nil
			} else {
				fields["bab_id"] = *in.BabID
			}
		}
	}
	if in.DeadlineExplicit {
		oldT := time.Time{}
		if existing.Deadline != nil {
			oldT = *existing.Deadline
		}
		newT := time.Time{}
		if in.Deadline != nil {
			newT = *in.Deadline
		}
		if !oldT.Equal(newT) {
			if in.Deadline == nil {
				fields["deadline"] = nil
			} else {
				fields["deadline"] = *in.Deadline
			}
		}
	}
	if in.IzinkanLate != nil && *in.IzinkanLate != existing.IzinkanLate {
		fields["izinkan_late"] = *in.IzinkanLate
	}
	if in.PenaltyPersen != nil {
		if *in.PenaltyPersen < 0 || *in.PenaltyPersen > 100 {
			return nil, fmt.Errorf("%w: penalty_persen must be between 0 and 100", ErrInvalidInput)
		}
		if *in.PenaltyPersen != existing.PenaltyPersen {
			fields["penalty_persen"] = *in.PenaltyPersen
		}
	}
	if in.WajibAttachment != nil && *in.WajibAttachment != existing.WajibAttachment {
		fields["wajib_attachment"] = *in.WajibAttachment
	}
	newStatus := existing.Status
	if in.Status != nil {
		if !in.Status.Valid() {
			return nil, fmt.Errorf("%w: status must be draft|published|archived", ErrInvalidInput)
		}
		if *in.Status != existing.Status {
			fields["status"] = *in.Status
			newStatus = *in.Status
		}
	}

	if len(fields) == 0 {
		// No-op PATCH: return existing without bumping version.
		return existing, nil
	}

	if err := s.repo.UpdateBasic(ctx, id, in.ExpectedVersion, fields); err != nil {
		return nil, mapRepoErr(err)
	}
	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("tugas update refetch: %w", err)
	}

	action := "tugas_updated"
	if existing.Status != newStatus {
		action = "tugas_status_changed"
	}
	s.logAudit(ctx, action, &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"tugas_id":    id.String(),
		"old_status":  string(existing.Status),
		"new_status":  string(fresh.Status),
		"old_version": existing.Version,
		"new_version": fresh.Version,
		"changed":     fieldKeys(fields),
	})
	return fresh, nil
}

// ---------- Delete ----------

// Delete hard-deletes a tugas + cascade attachment rows. Returns the
// orphan ObjectKeys for compensating R2 cleanup (caller responsibility).
// Owner-only. Audit-logged.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Tugas, []string, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("tugas delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, nil, err
	}

	keys, err := s.repo.Delete(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("tugas delete: %w", err)
	}

	s.logAudit(ctx, "tugas_deleted", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"tugas_id":         id.String(),
		"judul":            existing.Judul,
		"status":           string(existing.Status),
		"attachment_count": len(keys),
	})
	return existing, keys, nil
}

// ---------- Helpers ----------

func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("tugas kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

func (s *Service) assertEnrolled(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	if s.enroll == nil {
		return fmt.Errorf("tugas: enrollment lookup not configured")
	}
	enr, err := s.enroll.FindEnrollment(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("tugas enrollment lookup: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return ErrForbidden
	}
	return nil
}

func (s *Service) assertBabInKelas(ctx context.Context, babID, kelasID uuid.UUID) error {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: bab not found", ErrInvalidInput)
	}
	if err != nil {
		return fmt.Errorf("tugas bab lookup: %w", err)
	}
	if b.KelasID != kelasID {
		return ErrBabNotInKelas
	}
	return nil
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

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	default:
		return fmt.Errorf("tugas repo: %w", err)
	}
}

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

func deadlineStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func fieldKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
	targetType := "tugas"
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
