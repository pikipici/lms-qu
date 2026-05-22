// Service layer untuk banksoal: input validation, ownership guard
// (per-guru pribadi locked #84), optimistic concurrency, audit logging.
// Handler stays thin.
//
// Authorization (Task 6.B.1):
//
//   - Create/Update/Delete: guru pemilik (owner_guru_id == caller) atau
//     admin override.
//   - List/Get: guru pemilik OR admin. Siswa BLOCKED — siswa tidak punya
//     direct view ke Bank Soal; akses lewat HasilUjian (nanti di Fase 6.D).
//
// Locked decisions referenced:
//   - #56 optimistic concurrency: PATCH wajib `version`.
//   - #69 hard delete + R2 cleanup compensating (image cleanup di Task 6.B.2).
//   - #76 sub-fase split (mirror struktur Fase 5).
//   - #78 image upload constraints (validation deferred ke Task 6.B.2).
//   - #84 Bank Soal scope per-guru pribadi (no share antar-guru).
//   - #88 coverage gate 70%.
package banksoal

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
	"github.com/pikip/lms/backend/internal/storage"
)

// Sentinel errors mapped to HTTP status di handler.
var (
	ErrInvalidInput   = errors.New("banksoal: invalid input")
	ErrNotFound       = errors.New("banksoal: not found")
	ErrForbidden      = errors.New("banksoal: forbidden")
	ErrJawabanInvalid = errors.New("banksoal: jawaban points to empty option")
)

// Length caps for soal text fields (mirror soalbab).
const (
	MaxPertanyaanBytes = 5 * 1024 // 5KB per pertanyaan body
	MaxOpsiBytes       = 2 * 1024 // 2KB per opsi text
	MaxTagBytes        = 256      // 256B per mapel/tingkat/topik tag
)

// repoAPI is the subset of *Repo the service depends on.
type repoAPI interface {
	CreateSoal(ctx context.Context, s *BankSoal) error
	FindSoalByID(ctx context.Context, id uuid.UUID) (*BankSoal, error)
	ListByOwner(ctx context.Context, guruID uuid.UUID, f ListFilter) ([]BankSoal, error)
	CountByOwner(ctx context.Context, guruID uuid.UUID, f ListFilter) (int64, error)
	UpdateSoalBasic(ctx context.Context, id uuid.UUID, expectedVersion int, fields map[string]interface{}) error
	SoftDeleteSoal(ctx context.Context, id uuid.UUID, expectedVersion int) error
}

// auditLogger lets the service write audit rows without a hard auth dep.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles banksoal business logic.
type Service struct {
	repo  repoAPI
	audit auditLogger
	store storage.Storage
	now   func() time.Time
}

// NewService wires banksoal Repo + audit logger.
// Storage is optional — pass nil untuk disable image upload (Task 6.B.2).
// Pass *storage.R2Client untuk production.
func NewService(repo repoAPI, audit auditLogger, store storage.Storage) *Service {
	return &Service{repo: repo, audit: audit, store: store, now: time.Now}
}

// ---------- Create ----------

// CreateInput holds fields for POST /api/v1/bank-soal.
type CreateInput struct {
	Mapel      string
	Tingkat    string
	Topik      string
	Pertanyaan string
	OpsiA      string
	OpsiB      string
	OpsiC      string
	OpsiD      string
	OpsiE      string
	Jawaban    Jawaban
	Poin       int16 // 0 → default 1
}

// Create publishes a soal under caller's bank. Guru bind owner_guru_id =
// caller; admin must specify ownerOverride lewat input separate (TBD).
func (s *Service) Create(ctx context.Context, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*BankSoal, error) {
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}

	soal := &BankSoal{
		OwnerGuruID: callerID,
		Mapel:       strings.TrimSpace(in.Mapel),
		Tingkat:     strings.TrimSpace(in.Tingkat),
		Topik:       strings.TrimSpace(in.Topik),
		Pertanyaan:  strings.TrimRight(in.Pertanyaan, " \t\r\n"),
		OpsiA:       in.OpsiA,
		OpsiB:       in.OpsiB,
		OpsiC:       in.OpsiC,
		OpsiD:       in.OpsiD,
		OpsiE:       in.OpsiE,
		Jawaban:     in.Jawaban,
		Poin:        in.Poin,
		Version:     1,
	}
	if err := s.validateSoalFields(soal); err != nil {
		return nil, err
	}
	if soal.Poin == 0 {
		soal.Poin = 1
	}

	if err := s.repo.CreateSoal(ctx, soal); err != nil {
		return nil, fmt.Errorf("banksoal create: %w", err)
	}

	s.logAudit(ctx, "banksoal_created", &callerID, callerRole, &soal.ID, nil, ip, userAgent, map[string]any{
		"soal_id": soal.ID.String(),
		"mapel":   soal.Mapel,
		"tingkat": soal.Tingkat,
		"topik":   soal.Topik,
		"jawaban": string(soal.Jawaban),
		"poin":    soal.Poin,
	})
	return soal, nil
}

// ---------- List ----------

// ListInput narrows List results.
type ListInput struct {
	Mapel   string
	Tingkat string
	Topik   string
	Limit   int
	Offset  int
}

// ListResult bundles items + pagination metadata.
type ListResult struct {
	Items  []BankSoal
	Total  int64
	Limit  int
	Offset int
}

// List returns soal owned by caller (or admin sees specified guru via
// ownerOverride — admin route deferred). Siswa BLOCKED.
func (s *Service) List(ctx context.Context, callerID uuid.UUID, callerRole string, in ListInput) (*ListResult, error) {
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}
	f := ListFilter{
		Mapel:   strings.TrimSpace(in.Mapel),
		Tingkat: strings.TrimSpace(in.Tingkat),
		Topik:   strings.TrimSpace(in.Topik),
		Limit:   in.Limit,
		Offset:  in.Offset,
	}
	rows, err := s.repo.ListByOwner(ctx, callerID, f)
	if err != nil {
		return nil, fmt.Errorf("banksoal list: %w", err)
	}
	total, err := s.repo.CountByOwner(ctx, callerID, f)
	if err != nil {
		return nil, fmt.Errorf("banksoal count: %w", err)
	}
	return &ListResult{Items: rows, Total: total, Limit: in.Limit, Offset: in.Offset}, nil
}

// ---------- Get ----------

// Get returns a soal by id. Owner-only (admin override allowed).
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*BankSoal, error) {
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}
	soal, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal get: %w", err)
	}
	if !canManageSoal(soal, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return soal, nil
}

// ---------- Update ----------

// UpdateInput is the PATCH payload.
type UpdateInput struct {
	ExpectedVersion int
	Mapel           *string
	Tingkat         *string
	Topik           *string
	Pertanyaan      *string
	OpsiA           *string
	OpsiB           *string
	OpsiC           *string
	OpsiD           *string
	OpsiE           *string
	Jawaban         *Jawaban
	Poin            *int16
}

// Update applies a partial update with optimistic concurrency. Owner-only.
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*BankSoal, error) {
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal update find: %w", err)
	}
	if !canManageSoal(existing, callerID, callerRole) {
		return nil, ErrForbidden
	}

	merged := *existing
	fields := map[string]any{}

	if in.Mapel != nil {
		v := strings.TrimSpace(*in.Mapel)
		if v != merged.Mapel {
			merged.Mapel = v
			fields["mapel"] = v
		}
	}
	if in.Tingkat != nil {
		v := strings.TrimSpace(*in.Tingkat)
		if v != merged.Tingkat {
			merged.Tingkat = v
			fields["tingkat"] = v
		}
	}
	if in.Topik != nil {
		v := strings.TrimSpace(*in.Topik)
		if v != merged.Topik {
			merged.Topik = v
			fields["topik"] = v
		}
	}
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
		return nil, fmt.Errorf("banksoal update refetch: %w", err)
	}

	s.logAudit(ctx, "banksoal_updated", &callerID, callerRole, &id, nil, ip, userAgent, map[string]any{
		"soal_id":     id.String(),
		"old_version": existing.Version,
		"new_version": fresh.Version,
		"changed":     fieldKeys(fields),
	})
	return fresh, nil
}

// ---------- Delete ----------

// Delete soft-deletes a bank_soal row (locked #84 — soft delete agar
// HasilUjian referensi tetap valid). Owner-only.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole string, expectedVersion int, ip, userAgent string) (*BankSoal, error) {
	if !canWriteBank(callerRole) {
		return nil, ErrForbidden
	}
	if expectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindSoalByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("banksoal delete find: %w", err)
	}
	if !canManageSoal(existing, callerID, callerRole) {
		return nil, ErrForbidden
	}

	if err := s.repo.SoftDeleteSoal(ctx, id, expectedVersion); err != nil {
		return nil, mapRepoErr(err)
	}

	s.logAudit(ctx, "banksoal_deleted", &callerID, callerRole, &id, nil, ip, userAgent, map[string]any{
		"soal_id": id.String(),
		"mapel":   existing.Mapel,
		"tingkat": existing.Tingkat,
		"topik":   existing.Topik,
	})
	return existing, nil
}

// ---------- Helpers ----------

// canWriteBank: hanya guru / admin yang punya akses ke Bank Soal write.
// Siswa BLOCKED at handler/service boundary (locked #84).
func canWriteBank(role string) bool {
	return role == string(auth.Admin) || role == string(auth.Guru)
}

// canManageSoal: guru hanya bisa manage soal miliknya sendiri (locked
// #84 — no share antar-guru). Admin override allowed (untuk audit).
func canManageSoal(s *BankSoal, callerID uuid.UUID, callerRole string) bool {
	if s == nil {
		return false
	}
	if callerRole == string(auth.Admin) {
		return true
	}
	if callerRole == string(auth.Guru) && s.OwnerGuruID == callerID {
		return true
	}
	return false
}

// validateSoalFields enforces field constraints across both Create + Update.
// Caller must have already merged any partial PATCH onto the row.
func (s *Service) validateSoalFields(soal *BankSoal) error {
	if len(soal.Mapel) > MaxTagBytes {
		return fmt.Errorf("%w: mapel exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}
	if len(soal.Tingkat) > MaxTagBytes {
		return fmt.Errorf("%w: tingkat exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}
	if len(soal.Topik) > MaxTagBytes {
		return fmt.Errorf("%w: topik exceeds %d bytes", ErrInvalidInput, MaxTagBytes)
	}
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
	return nil
}

func optionTextFor(s *BankSoal, j Jawaban) string {
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

func optionImageFor(s *BankSoal, j Jawaban) *string {
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
		return fmt.Errorf("banksoal repo: %w", err)
	}
}

func fieldKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// logAudit best-effort: a logging failure must not poison success response.
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "banksoal"
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
