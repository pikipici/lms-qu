// Service layer untuk materi: input validation, ownership guard via kelas
// (and bab when bab_id is supplied), tipe-immutable PATCH, optimistic
// concurrency, audit logging. Handler stays thin.
//
// Scope of Task 3.C.2: youtube + markdown only. PDF upload + R2 delete
// compensating dipasang di Task 3.C.3 (separate Upload service method).
// MarkRead siswa flow di Task 3.C.4.
package materi

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
	"github.com/pikip/lms/backend/internal/storage"
)

// Sentinel errors yang di-mapping ke HTTP status di handler. ErrVersionConflict
// + ErrInvalidTipe diekspor dari repo.go dan dipakai handler langsung.
var (
	ErrInvalidInput   = errors.New("materi: invalid input")
	ErrNotFound       = errors.New("materi: not found")
	ErrForbidden      = errors.New("materi: forbidden")
	ErrKelasArchived  = errors.New("materi: kelas archived")
	ErrBabNotInKelas  = errors.New("materi: bab does not belong to this kelas")
	ErrTipeImmutable  = errors.New("materi: tipe is immutable; delete + recreate")
	ErrTipeUnsupported = errors.New("materi: tipe not supported in this endpoint")
	ErrKontenTooLong  = errors.New("materi: konten exceeds size limit")
)

// MaxMarkdownBytes caps the markdown body persisted in materi.konten. The
// app-level limit (50KB per locked roadmap §3.C.2) is enforced here so the
// handler stays purely transport.
const MaxMarkdownBytes = 50 * 1024

// materiRepo subset yang dipakai service. Interface dipisah supaya gampang
// di-mock di test.
type materiRepo interface {
	Create(ctx context.Context, m *Materi) error
	FindByID(ctx context.Context, id uuid.UUID) (*Materi, error)
	MaxUrutan(ctx context.Context, kelasID uuid.UUID, f BabFilter) (int, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f BabFilter) ([]Materi, error)
	UpdateBasic(ctx context.Context, id uuid.UUID, expectedVersion int, judul, konten string, urutan int) error
	Delete(ctx context.Context, id uuid.UUID) (*string, error)
}

// kelasLookup hydrates kelas ownership/lifecycle. We don't import *kelas.Repo
// directly to keep the dep narrow; the production wiring passes *kelas.Repo
// which satisfies the interface.
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// babLookup verifies bab→kelas ownership when materi.BabID is supplied.
type babLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
}

// auditLogger lets the service write audit rows without a hard auth dep.
// Implemented by *auth.Repo.
type auditLogger interface {
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// Service handles materi business logic.
type Service struct {
	repo  materiRepo
	kelas kelasLookup
	bab   babLookup
	audit auditLogger
	store storage.Storage
	now   func() time.Time
}

// NewService wires materi Repo + kelas + bab lookup + audit logger +
// optional object store. The store is used by Upload (Task 3.C.3) for
// PutObject and by Delete for compensating R2 cleanup of tipe='pdf'
// rows (locked #69). Pass nil to disable upload/cleanup paths — used in
// 3.C.2-only test fixtures and main.go before R2 is configured.
func NewService(repo materiRepo, kelas kelasLookup, bab babLookup, audit auditLogger, store storage.Storage) *Service {
	return &Service{repo: repo, kelas: kelas, bab: bab, audit: audit, store: store, now: time.Now}
}

// CreateInput holds fields for POST /kelas/:id/materi (youtube + markdown).
//
// PDF upload uses a separate path (Task 3.C.3 — multipart, not JSON).
type CreateInput struct {
	BabID  *uuid.UUID
	Judul  string
	Tipe   Tipe
	Konten string // youtube: URL (parsed → video_id) | markdown: body
}

// Create membuat materi baru. Owner-only + kelas active. BabID kalau dikasih
// wajib milik kelas yang sama. Urutan auto = max(urutan)+1 dalam scope
// (kelas, bab_id) — kalau BabID nil scope = bab_id IS NULL.
//
// Tipe yang di-handle di sini: youtube + markdown. PDF wajib lewat upload
// endpoint (Task 3.C.3).
func (s *Service) Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Materi, error) {
	judul := strings.TrimSpace(in.Judul)
	if judul == "" {
		return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
	}
	if !in.Tipe.Valid() {
		return nil, fmt.Errorf("%w: tipe must be pdf|youtube|markdown", ErrInvalidInput)
	}
	if in.Tipe == TipePDF {
		return nil, fmt.Errorf("%w: pdf must be uploaded via multipart endpoint", ErrTipeUnsupported)
	}

	k, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole)
	if err != nil {
		return nil, err
	}
	if k.ArchivedAt != nil {
		return nil, ErrKelasArchived
	}

	// Validate bab ownership when BabID supplied.
	if in.BabID != nil {
		if err := s.assertBabInKelas(ctx, *in.BabID, kelasID); err != nil {
			return nil, err
		}
	}

	// Resolve konten per tipe.
	var konten string
	switch in.Tipe {
	case TipeYouTube:
		id, perr := parseYouTubeID(in.Konten)
		if perr != nil {
			return nil, fmt.Errorf("%w: invalid youtube url", ErrInvalidInput)
		}
		konten = id
	case TipeMarkdown:
		konten = in.Konten
		if len(konten) > MaxMarkdownBytes {
			return nil, fmt.Errorf("%w: markdown body exceeds %d bytes", ErrKontenTooLong, MaxMarkdownBytes)
		}
	}

	babFilter := babFilterFrom(in.BabID)
	maxUrutan, err := s.repo.MaxUrutan(ctx, kelasID, babFilter)
	if err != nil {
		return nil, fmt.Errorf("materi create max urutan: %w", err)
	}

	m := &Materi{
		KelasID: kelasID,
		BabID:   in.BabID,
		Judul:   judul,
		Tipe:    in.Tipe,
		Konten:  konten,
		Urutan:  maxUrutan + 1,
		Version: 1,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("materi create: %w", err)
	}

	s.logAudit(ctx, "materi_created", &callerID, callerRole, &m.ID, &kelasID, ip, userAgent, map[string]any{
		"materi_id": m.ID.String(),
		"bab_id":    babIDStr(in.BabID),
		"judul":     m.Judul,
		"tipe":      string(m.Tipe),
		"urutan":    m.Urutan,
	})
	return m, nil
}

// ListInput narrows ListByKelas results.
//
// BabID semantics:
//   - nil:                   no bab filter (return all materi in kelas)
//   - non-nil + zero UUID:   pin bab_id IS NULL (materi berdiri bebas)
//   - non-nil + real UUID:   pin bab_id = BabID
type ListInput struct {
	BabID *uuid.UUID
}

// ListByKelas returns materi rows in a kelas owned by caller (or admin),
// optionally scoped by bab_id.
func (s *Service) ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Materi, error) {
	if _, err := s.findKelasOrForbidden(ctx, kelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	// If a real (non-nil) bab id is supplied, verify it belongs to this kelas.
	if in.BabID != nil && *in.BabID != uuid.Nil {
		if err := s.assertBabInKelas(ctx, *in.BabID, kelasID); err != nil {
			return nil, err
		}
	}
	rows, err := s.repo.ListByKelas(ctx, kelasID, babFilterFromList(in.BabID))
	if err != nil {
		return nil, fmt.Errorf("materi list: %w", err)
	}
	return rows, nil
}

// Get returns a materi by id with ownership guard via its kelas.
func (s *Service) Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Materi, error) {
	m, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("materi get: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, m.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}
	return m, nil
}

// UpdateInput is the PATCH payload. Pointer fields are optional ("nil =
// leave unchanged"). ExpectedVersion is required. Tipe is intentionally
// NOT updatable — caller must delete + recreate.
type UpdateInput struct {
	ExpectedVersion int
	Judul           *string
	Konten          *string
	Urutan          *int
}

// Update applies a partial update with optimistic concurrency.
//
// Konten validation per existing tipe:
//   - youtube: parsed ulang via parseYouTubeID → simpan video_id
//   - markdown: cap MaxMarkdownBytes
//   - pdf: konten editing tidak diizinkan via PATCH (PDF konten = "" by
//     invariant; payload editing dilakukan via re-upload di 3.C.3)
func (s *Service) Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Materi, error) {
	if in.ExpectedVersion <= 0 {
		return nil, fmt.Errorf("%w: version must be positive", ErrInvalidInput)
	}

	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("materi update find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, err
	}

	// Resolve final values.
	judul := existing.Judul
	if in.Judul != nil {
		judul = strings.TrimSpace(*in.Judul)
		if judul == "" {
			return nil, fmt.Errorf("%w: judul is required", ErrInvalidInput)
		}
	}

	konten := existing.Konten
	if in.Konten != nil {
		raw := *in.Konten
		switch existing.Tipe {
		case TipeYouTube:
			vid, perr := parseYouTubeID(raw)
			if perr != nil {
				return nil, fmt.Errorf("%w: invalid youtube url", ErrInvalidInput)
			}
			konten = vid
		case TipeMarkdown:
			if len(raw) > MaxMarkdownBytes {
				return nil, fmt.Errorf("%w: markdown body exceeds %d bytes", ErrKontenTooLong, MaxMarkdownBytes)
			}
			konten = raw
		case TipePDF:
			return nil, fmt.Errorf("%w: konten editing not allowed for pdf — re-upload required", ErrTipeImmutable)
		}
	}

	urutan := existing.Urutan
	if in.Urutan != nil {
		urutan = *in.Urutan
		if urutan < 0 {
			return nil, fmt.Errorf("%w: urutan must be >= 0", ErrInvalidInput)
		}
	}

	if judul == existing.Judul && konten == existing.Konten && urutan == existing.Urutan {
		// No-op PATCH: return existing without bumping version.
		return existing, nil
	}

	if err := s.repo.UpdateBasic(ctx, id, in.ExpectedVersion, judul, konten, urutan); err != nil {
		return nil, mapRepoErr(err)
	}

	fresh, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("materi update refetch: %w", err)
	}
	s.logAudit(ctx, "materi_updated", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
		"materi_id":   id.String(),
		"old_judul":   existing.Judul,
		"new_judul":   fresh.Judul,
		"old_urutan":  existing.Urutan,
		"new_urutan":  fresh.Urutan,
		"konten_changed": existing.Konten != fresh.Konten,
		"new_version": fresh.Version,
	})
	return fresh, nil
}

// Delete hard-deletes a materi row. For PDF tipe the R2 ObjectKey is also
// removed via store.DeleteObject (locked #69). Compensating semantics:
//   - DB delete failure → no R2 call, return error.
//   - DB delete success + R2 delete failure → log audit.materi_r2_orphan,
//     return success (DB row already gone; R2 orphan toleransi per #69).
//
// For non-pdf tipe (youtube/markdown) ObjectKey is nil and no R2 call is
// made. Returns ErrNotFound if the row is missing.
func (s *Service) Delete(ctx context.Context, id, callerID uuid.UUID, callerRole, ip, userAgent string) (*Materi, *string, error) {
	existing, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("materi delete find: %w", err)
	}
	if _, err := s.findKelasOrForbidden(ctx, existing.KelasID, callerID, callerRole); err != nil {
		return nil, nil, err
	}

	objectKey, err := s.repo.Delete(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("materi delete: %w", err)
	}

	// Compensating R2 cleanup for tipe='pdf' (locked #69). Best-effort —
	// DB row is already gone; an R2 orphan is acceptable, audit logs the
	// drift so an operator can purge later.
	r2OrphanKey := ""
	if existing.Tipe == TipePDF && objectKey != nil && *objectKey != "" && s.store != nil {
		if derr := s.store.DeleteObject(ctx, *objectKey); derr != nil {
			r2OrphanKey = *objectKey
			s.logAudit(ctx, "materi_r2_orphan", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, map[string]any{
				"materi_id":  id.String(),
				"object_key": *objectKey,
				"reason":     "delete_object_failed",
				"err":        derr.Error(),
			})
		}
	}

	meta := map[string]any{
		"materi_id":  id.String(),
		"judul":      existing.Judul,
		"tipe":       string(existing.Tipe),
		"object_key": objectKeyStr(objectKey),
	}
	if r2OrphanKey != "" {
		meta["r2_orphan"] = true
	}
	s.logAudit(ctx, "materi_deleted", &callerID, callerRole, &id, &existing.KelasID, ip, userAgent, meta)
	return existing, objectKey, nil
}

// findKelasOrForbidden hydrates kelas + asserts callerID/role can manage it.
// Returns ErrNotFound if the kelas is missing, ErrForbidden if caller isn't
// owner/admin.
func (s *Service) findKelasOrForbidden(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string) (*kelas.Kelas, error) {
	k, err := s.kelas.FindByID(ctx, kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("materi kelas find: %w", err)
	}
	if !canManageKelas(k, callerID, callerRole) {
		return nil, ErrForbidden
	}
	return k, nil
}

// assertBabInKelas verifies the bab exists and belongs to the same kelas.
// Returns ErrInvalidInput when the bab is missing, ErrBabNotInKelas when
// it lives under a different kelas.
func (s *Service) assertBabInKelas(ctx context.Context, babID, kelasID uuid.UUID) error {
	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("%w: bab not found", ErrInvalidInput)
	}
	if err != nil {
		return fmt.Errorf("materi bab lookup: %w", err)
	}
	if b.KelasID != kelasID {
		return ErrBabNotInKelas
	}
	return nil
}

// canManageKelas mirrors bab.canManageKelas: admin manages all; guru manages
// their own; siswa rejected.
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

// mapRepoErr converts repo-level sentinels into service-level sentinels so
// handler code can match on a single layer.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, ErrVersionConflict):
		return ErrVersionConflict
	case errors.Is(err, ErrInvalidTipe):
		return ErrInvalidTipe
	default:
		return fmt.Errorf("materi repo: %w", err)
	}
}

// logAudit is best-effort: a logging failure must not poison the user-facing
// success response, so we swallow the error here (matches kelas + bab + admin).
func (s *Service) logAudit(ctx context.Context, action string, actorID *uuid.UUID, actorRole string, targetID, targetKelasID *uuid.UUID, ip, userAgent string, meta map[string]any) {
	if s.audit == nil {
		return
	}
	targetType := "materi"
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

// babFilterFrom converts an optional BabID (Create input) into a repo
// BabFilter. nil BabID maps to "scope by bab_id IS NULL" — distinct rows
// per (kelas, bab_id IS NULL) urutan stream.
func babFilterFrom(bid *uuid.UUID) BabFilter {
	if bid == nil {
		return BabFilter{Mode: BabFilterNull}
	}
	return BabFilter{Mode: BabFilterEq, BabID: *bid}
}

// babFilterFromList differs from babFilterFrom in semantics for List:
//   - nil:               no filter (return all in kelas)
//   - non-nil zero UUID: pin bab_id IS NULL (handler maps query "?bab_id=null")
//   - non-nil real UUID: pin bab_id = id
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

func objectKeyStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
