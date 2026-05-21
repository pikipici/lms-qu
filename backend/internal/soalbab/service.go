// Service layer untuk soalbab: input validation, ownership/enrollment guards,
// optimistic concurrency, audit logging. Handler stays thin.
//
// Authorization (Task 5.B.1):
//
//   - Create/Update/Delete: guru pemilik kelas atau admin.
//   - List/Get (siswa-direct): siswa enrolled BLOCKED — siswa view soal harus
//     lewat flow Latihan/Ulangan endpoint (Task 5.C/5.D); list direct return
//     403 untuk siswa supaya gak bisa enumerate jawaban tanpa attempt.
//   - List/Get untuk guru/admin: full visibility incl. pemilik soal di
//     bab archived (untuk audit).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #69 hard delete + R2 cleanup compensating (image cleanup di Task 5.B.2;
//     service.Delete returns ObjectKeys keluar untuk Task 5.B.2 wiring).
//   - #76 sub-fase split.
//   - #78 image upload constraints (validation deferred ke Task 5.B.2).
//   - #82 coverage gate 70%.
package soalbab

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
	ErrInvalidInput       = errors.New("soalbab: invalid input")
	ErrNotFound           = errors.New("soalbab: not found")
	ErrForbidden          = errors.New("soalbab: forbidden")
	ErrBabArchived        = errors.New("soalbab: bab archived")
	ErrJawabanInvalid     = errors.New("soalbab: jawaban points to empty option")
)

// Length caps for soal text fields.
const (
	MaxPertanyaanBytes = 5 * 1024 // 5KB per pertanyaan body
	MaxOpsiBytes       = 2 * 1024 // 2KB per opsi text
)

// repoAPI is the subset of *Repo the service depends on.
type repoAPI interface {
	CreateSoal(ctx context.Context, s *SoalBab) error
	FindSoalByID(ctx context.Context, id uuid.UUID) (*SoalBab, error)
	ListSoalByBab(ctx context.Context, babID uuid.UUID, f SoalListFilter) ([]SoalBab, error)
	UpdateSoalBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]interface{}) error
	DeleteSoal(ctx context.Context, id uuid.UUID) ([]string, error)
}

// kelasLookup hydrates kelas ownership/lifecycle.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// babLookup hydrates bab→kelas + status.
type babLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
}

// auditLogger lets the service write audit rows without a hard auth dep.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles soalbab business logic.
type Service struct {
	repo  repoAPI
	kelas kelasLookup
	bab   babLookup
	audit auditLogger
	now   func() time.Time
}

// NewService wires soalbab Repo + kelas/bab lookups + audit logger.
func NewService(repo repoAPI, kelas kelasLookup, bab babLookup, audit auditLogger) *Service {
	return &Service{repo: repo, kelas: kelas, bab: bab, audit: audit, now: time.Now}
}

// ---------- Create ----------

// CreateInput holds fields for POST /api/v1/bab/:id/soal.
type CreateInput struct {
	Pertanyaan string
	OpsiA      string
	OpsiB      string
	OpsiC      string
	OpsiD      string
	OpsiE      string
	Jawaban    Jawaban
	Poin       int16 // 0 → default 1
	Mode       Mode  // empty → default keduanya
	Urutan     int   // 0 → default 0 (no auto-bump)
}

// Create publishes a soal under a bab. Owner-only + bab not archived.
func (s *Service) Create(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*SoalBab, error) {
	b, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
	}

	soal := &SoalBab{
		BabID:       b.ID,
		KelasID:     b.KelasID,
		Pertanyaan:  strings.TrimRight(in.Pertanyaan, " \t\r\n"),
		OpsiA:       in.OpsiA,
		OpsiB:       in.OpsiB,
		OpsiC:       in.OpsiC,
		OpsiD:       in.OpsiD,
		OpsiE:       in.OpsiE,
		Jawaban:     in.Jawaban,
		Poin:        in.Poin,
		Mode:        in.Mode,
		Urutan:      in.Urutan,
		Version:     1,
		CreatedByID: callerID,
	}
	if err := s.validateSoalFields(soal); err != nil {
		return nil, err
	}
	if soal.Poin == 0 {
		soal.Poin = 1
	}
	if soal.Mode == "" {
		soal.Mode = ModeKeduanya
	}

	if err := s.repo.CreateSoal(ctx, soal); err != nil {
		return nil, fmt.Errorf("soalbab create: %w", err)
	}

	s.logAudit(ctx, "soalbab_created", &callerID, callerRole, &soal.ID, &b.KelasID, ip, userAgent, map[string]any{
		"soal_id": soal.ID.String(),
		"bab_id":  b.ID.String(),
		"mode":    string(soal.Mode),
		"jawaban": string(soal.Jawaban),
		"poin":    soal.Poin,
	})
	return soal, nil
}

// ---------- List ----------

// ListInput narrows ListByBab results.
type ListInput struct {
	Mode  Mode
	Limit int
}

// ListByBab returns soal in a bab. Siswa-direct list is forbidden by design
// (locked #76 — siswa view soal lewat flow Latihan/Ulangan endpoint).
func (s *Service) ListByBab(ctx context.Context, babID, callerID uuid.UUID, callerRole string, in ListInput) ([]SoalBab, error) {
	if callerRole == string(auth.Siswa) {
		return nil, ErrForbidden
	}
	if _, err := s.findBabAndOwnership(ctx, babID, callerID, callerRole); err != nil {
		return nil, err
	}
	f := SoalListFilter{Mode: in.Mode, Limit: in.Limit}
	return s.repo.ListSoalByBab(ctx, babID, f)
}

// ---------- Get ----------

// Get returns a soal by id. Guru/admin owner only — siswa lewat flow.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*SoalBab, error) {
	if callerRole == string(auth.Siswa) {
		return nil, ErrForbidden
	}
	soal, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab get: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, soal.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return soal, nil
}

// ---------- Update ----------

// UpdateInput is the PATCH payload.
type UpdateInput struct {
	ExpectedVersion int
	Pertanyaan      *string
	OpsiA           *string
	OpsiB           *string
	OpsiC           *string
	OpsiD           *string
	OpsiE           *string
	Jawaban         *Jawaban
	Poin            *int16
	Mode            *Mode
	Urutan          *int
}

// Update applies a partial update with optimistic concurrency. Owner-only.
// Bab archived → reject (consistency dengan Create).
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*SoalBab, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab update find: %w", err)
	}
	b, err := s.findBabAndOwnership(ctx, existing.BabID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if b.Status == bab.StatusArchived {
		return nil, ErrBabArchived
	}

	// Compute resolved values then validate the full row before issuing UPDATE.
	merged := *existing
	fields := map[string]any{}

	if in.Pertanyaan != nil {
		v := strings.TrimRight(*in.Pertanyaan, " \t\r\n")
		if v != merged.Pertanyaan {
			merged.Pertanyaan = v
			fields["pertanyaan"] = v
		}
	}
	for _, p := range []struct {
		col string
		new *string
		cur *string
	}{
		{"opsi_a", in.OpsiA, &merged.OpsiA},
		{"opsi_b", in.OpsiB, &merged.OpsiB},
		{"opsi_c", in.OpsiC, &merged.OpsiC},
		{"opsi_d", in.OpsiD, &merged.OpsiD},
		{"opsi_e", in.OpsiE, &merged.OpsiE},
	} {
		if p.new != nil && *p.new != *p.cur {
			*p.cur = *p.new
			fields[p.col] = *p.new
		}
	}
	if in.Jawaban != nil && *in.Jawaban != existing.Jawaban {
		merged.Jawaban = *in.Jawaban
		fields["jawaban"] = *in.Jawaban
	}
	if in.Poin != nil && *in.Poin != existing.Poin {
		merged.Poin = *in.Poin
		fields["poin"] = *in.Poin
	}
	if in.Mode != nil && *in.Mode != existing.Mode {
		merged.Mode = *in.Mode
		fields["mode"] = *in.Mode
	}
	if in.Urutan != nil && *in.Urutan != existing.Urutan {
		merged.Urutan = *in.Urutan
		fields["urutan"] = *in.Urutan
	}

	if err := s.validateSoalFields(&merged); err != nil {
		return nil, err
	}

	if len(fields) == 0 {
		return existing, nil
	}

	if err := s.repo.UpdateSoalBasic(ctx, id, in.ExpectedVersion, fields); err != nil {
		return nil, mapRepoErr(err)
	}
	fresh, err := s.repo.FindSoalByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("soalbab update refetch: %w", err)
	}

	s.logAudit(ctx, "soalbab_updated", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"soal_id":     id.String(),
		"old_version": existing.Version,
		"new_version": fresh.Version,
		"changed":     fieldKeys(fields),
	})
	return fresh, nil
}

// ---------- Delete ----------

// Delete hard-deletes a soal_bab row. Returns the orphan ObjectKeys for
// compensating R2 cleanup (caller responsibility, locked #69). Owner-only.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*SoalBab, []string, error) {
	existing, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("soalbab delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, nil, err
	}

	keys, err := s.repo.DeleteSoal(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("soalbab delete: %w", err)
	}

	s.logAudit(ctx, "soalbab_deleted", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"soal_id":         id.String(),
		"image_key_count": len(keys),
		"mode":            string(existing.Mode),
	})
	return existing, keys, nil
}

// ---------- Helpers ----------

func (s *Service) findBabAndOwnership(ctx context.Context, babID, callerID uuid.UUID, callerRole string) (*bab.Bab, error) {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab bab find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, b.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("soalbab kelas find: %w", err)
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

// validateSoalFields enforces field constraints across both Create + Update.
// Caller must have already merged any partial PATCH onto the row.
func (s *Service) validateSoalFields(soal *SoalBab) error {
	if len(soal.Pertanyaan) > MaxPertanyaanBytes {
		return fmt.Errorf("%w: pertanyaan exceeds %d bytes", ErrInvalidInput, MaxPertanyaanBytes)
	}
	for label, v := range map[string]string{
		"opsi_a": soal.OpsiA, "opsi_b": soal.OpsiB,
		"opsi_c": soal.OpsiC, "opsi_d": soal.OpsiD, "opsi_e": soal.OpsiE,
	} {
		if len(v) > MaxOpsiBytes {
			return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidInput, label, MaxOpsiBytes)
		}
	}
	if !soal.Jawaban.Valid() {
		return fmt.Errorf("%w: jawaban must be a|b|c|d|e", ErrInvalidInput)
	}
	// Pertanyaan must have either text or image — empty both is invalid.
	if strings.TrimSpace(soal.Pertanyaan) == "" && (soal.PertanyaanObjectKey == nil || *soal.PertanyaanObjectKey == "") {
		return fmt.Errorf("%w: pertanyaan must have text or image", ErrInvalidInput)
	}
	// Jawaban must point to an option that has either text or image.
	answerOpt := optionTextFor(soal, soal.Jawaban)
	answerImg := optionImageFor(soal, soal.Jawaban)
	if strings.TrimSpace(answerOpt) == "" && (answerImg == nil || *answerImg == "") {
		return fmt.Errorf("%w: %s", ErrJawabanInvalid, "selected option is empty")
	}
	if soal.Poin != 0 && (soal.Poin < 1 || soal.Poin > 100) {
		return fmt.Errorf("%w: poin must be between 1 and 100", ErrInvalidInput)
	}
	if soal.Mode != "" && !soal.Mode.Valid() {
		return fmt.Errorf("%w: mode must be latihan|ulangan|keduanya", ErrInvalidInput)
	}
	return nil
}

func optionTextFor(s *SoalBab, j Jawaban) string {
	switch j {
	case JawabanA:
		return s.OpsiA
	case JawabanB:
		return s.OpsiB
	case JawabanC:
		return s.OpsiC
	case JawabanD:
		return s.OpsiD
	case JawabanE:
		return s.OpsiE
	}
	return ""
}

func optionImageFor(s *SoalBab, j Jawaban) *string {
	switch j {
	case JawabanA:
		return s.OpsiAObjectKey
	case JawabanB:
		return s.OpsiBObjectKey
	case JawabanC:
		return s.OpsiCObjectKey
	case JawabanD:
		return s.OpsiDObjectKey
	case JawabanE:
		return s.OpsiEObjectKey
	}
	return nil
}

func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	default:
		return fmt.Errorf("soalbab repo: %w", err)
	}
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
	targetType := "soalbab"
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
