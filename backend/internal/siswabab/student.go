// Package siswabab houses the siswa-only bab list + detail endpoints
// (Task 3.E.1). Kept in its own package to avoid an import cycle: the
// materi package already depends on bab + kelas, so the siswa flow —
// which needs both bab + materi together — sits outside bab.
//
// Endpoints:
//   - GET /api/v1/siswa/kelas/:id/bab → list bab status='published' di
//     kelas yang siswa enroll, dengan progress per bab. Progress
//     fase-3-partial = materi_read_count / materi_total × 100 (locked #68
//     + Section 6.4). materi_total=0 → progress 0 + bab_kosong=true.
//   - GET /api/v1/siswa/bab/:id → detail bab + materi list (urutan ASC)
//     + read state per materi (boolean sudah_dibaca).
//
// Authorization:
//   - Caller role MUST be siswa (handler enforces with RoleGuard, service
//     re-checks defensively so unit tests don't need a fiber app).
//   - Siswa MUST have an active enrollment in the kelas (Repo.FindEnrollment;
//     status='removed' atau missing → 403 forbidden).
//   - Bab MUST be status='published' (404 not_found di siswa scope kalau
//     status draft/archived — hindari leak bab ke siswa).
//
// Components yang lain di Fase 4-7 (latihan, ulangan bab, tugas, hasil)
// di-flag breakdown dengan pct=null + w=0 di Fase 3 — FE skip render
// section terkait.
package siswabab

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/bab"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/materi"
	"github.com/pikip/lms/backend/internal/middleware"
)

// babLookup narrowly types the bab data the siswa flow needs. Implemented
// by *bab.Repo. We only depend on read-only methods.
type babLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*bab.Bab, error)
	ListByKelas(ctx context.Context, kelasID uuid.UUID, f bab.ListFilter) ([]bab.Bab, error)
}

// kelasLookup hydrates kelas existence (no ownership check — siswa
// authorization is enrollment-based, not ownership-based).
type kelasLookup interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
}

// enrollmentLookup verifies the siswa is enrolled in the kelas.
// Implemented by *kelas.Repo (FindEnrollment).
type enrollmentLookup interface {
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
}

// materiLookup narrowly types the materi data the siswa flow needs.
// Implemented by *materi.Repo. We only depend on read-only methods.
type materiLookup interface {
	ListByBab(ctx context.Context, babID uuid.UUID) ([]materi.Materi, error)
	CountByBabBatch(ctx context.Context, babIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	CountReadByBabBatch(ctx context.Context, babIDs []uuid.UUID, siswaID uuid.UUID) (map[uuid.UUID]int64, error)
	ListReadIDsByBabSiswa(ctx context.Context, babID, siswaID uuid.UUID) ([]uuid.UUID, error)
}

// Service wraps siswa-only bab queries.
type Service struct {
	bab    babLookup
	kelas  kelasLookup
	enroll enrollmentLookup
	materi materiLookup
}

// NewService wires bab repo + kelas lookup + enrollment guard + materi
// repo for siswa endpoints.
func NewService(b babLookup, k kelasLookup, e enrollmentLookup, m materiLookup) *Service {
	return &Service{bab: b, kelas: k, enroll: e, materi: m}
}

// ProgressBreakdownItem captures a single component of the bab progress.
// Pct is nullable — Fase 3 only ships the materi component; latihan/
// ulangan/tugas land later (Fase 4-7) so we surface them upfront with
// pct=nil + w=0 so FE can render placeholders without changing the API.
type ProgressBreakdownItem struct {
	Pct    *float64 `json:"pct"`
	Weight float64  `json:"w"`
}

// Progress is a per-bab progress record returned by ListSiswa.
type Progress struct {
	Persen     float64                          `json:"persen"`
	Breakdown  map[string]ProgressBreakdownItem `json:"breakdown"`
	BabKosong  bool                             `json:"bab_kosong"`
	MateriRead int                              `json:"materi_read"`
	MateriAll  int                              `json:"materi_total"`
}

// SiswaBabItem is one row in the siswa bab list.
type SiswaBabItem struct {
	ID        uuid.UUID  `json:"id"`
	Nomor     int        `json:"nomor"`
	Judul     string     `json:"judul"`
	Deskripsi string     `json:"deskripsi"`
	Urutan    int        `json:"urutan"`
	Status    bab.Status `json:"status"`
	Progress  Progress   `json:"progress"`
}

// SiswaMateriCard is a per-materi card on the bab detail (siswa view).
// Strips guru-only fields (object_key, mime_type, size_bytes — siswa
// downloads via presigned URL endpoint anyway, locked #62).
type SiswaMateriCard struct {
	ID          uuid.UUID  `json:"id"`
	BabID       *uuid.UUID `json:"bab_id"`
	Judul       string     `json:"judul"`
	Tipe        string     `json:"tipe"`
	Konten      string     `json:"konten"`
	Urutan      int        `json:"urutan"`
	SudahDibaca bool       `json:"sudah_dibaca"`
}

// SiswaBabDetail is the response body for GET /siswa/bab/:id.
type SiswaBabDetail struct {
	Bab    SiswaBabItem      `json:"bab"`
	Materi []SiswaMateriCard `json:"materi"`
}

// ListSiswa returns bab status='published' in kelas, with progress per bab.
//
// Authorization (defensive): callerRole must be siswa, and siswaID must
// have an active enrollment in kelasID. Returns ErrForbidden in either
// case (no leak whether the kelas exists or whether siswa is enrolled).
//
// Performance: single batched query per metric (CountByBabBatch +
// CountReadByBabBatch) — avoid N+1 over bab ids.
func (s *Service) ListSiswa(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) ([]SiswaBabItem, error) {
	if callerRole != string(auth.Siswa) {
		return nil, bab.ErrForbidden
	}

	// Verify kelas exists. Missing → collapse to ErrForbidden so siswa
	// cannot probe kelas existence by toggling 404 vs 403.
	if _, err := s.kelas.FindByID(ctx, kelasID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, bab.ErrForbidden
		}
		return nil, fmt.Errorf("siswa list bab kelas: %w", err)
	}

	if err := s.assertEnrollment(ctx, kelasID, siswaID); err != nil {
		return nil, err
	}

	pub := bab.StatusPublished
	rows, err := s.bab.ListByKelas(ctx, kelasID, bab.ListFilter{Status: &pub})
	if err != nil {
		return nil, fmt.Errorf("siswa list bab: %w", err)
	}

	babIDs := make([]uuid.UUID, len(rows))
	for i, b := range rows {
		babIDs[i] = b.ID
	}
	totals, err := s.materi.CountByBabBatch(ctx, babIDs)
	if err != nil {
		return nil, fmt.Errorf("siswa list bab totals: %w", err)
	}
	reads, err := s.materi.CountReadByBabBatch(ctx, babIDs, siswaID)
	if err != nil {
		return nil, fmt.Errorf("siswa list bab reads: %w", err)
	}

	out := make([]SiswaBabItem, len(rows))
	for i, b := range rows {
		out[i] = SiswaBabItem{
			ID:        b.ID,
			Nomor:     b.Nomor,
			Judul:     b.Judul,
			Deskripsi: b.Deskripsi,
			Urutan:    b.Urutan,
			Status:    b.Status,
			Progress:  computeProgress(totals[b.ID], reads[b.ID]),
		}
	}
	return out, nil
}

// GetSiswa returns a single bab + materi list with read state. Bab MUST
// be status='published' (404/not_found di siswa scope kalau draft/archived).
func (s *Service) GetSiswa(ctx context.Context, babID, siswaID uuid.UUID, callerRole string) (*SiswaBabDetail, error) {
	if callerRole != string(auth.Siswa) {
		return nil, bab.ErrForbidden
	}

	b, err := s.bab.FindByID(ctx, babID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, bab.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("siswa get bab find: %w", err)
	}
	if b.Status != bab.StatusPublished {
		return nil, bab.ErrNotFound
	}

	if err := s.assertEnrollment(ctx, b.KelasID, siswaID); err != nil {
		return nil, err
	}

	mats, err := s.materi.ListByBab(ctx, babID)
	if err != nil {
		return nil, fmt.Errorf("siswa get bab materi: %w", err)
	}
	readIDs, err := s.materi.ListReadIDsByBabSiswa(ctx, babID, siswaID)
	if err != nil {
		return nil, fmt.Errorf("siswa get bab reads: %w", err)
	}
	readSet := make(map[uuid.UUID]struct{}, len(readIDs))
	for _, id := range readIDs {
		readSet[id] = struct{}{}
	}

	cards := make([]SiswaMateriCard, len(mats))
	for i, m := range mats {
		_, sudah := readSet[m.ID]
		cards[i] = SiswaMateriCard{
			ID:          m.ID,
			BabID:       m.BabID,
			Judul:       m.Judul,
			Tipe:        string(m.Tipe),
			Konten:      m.Konten,
			Urutan:      m.Urutan,
			SudahDibaca: sudah,
		}
	}

	totalMateri := int64(len(mats))
	readMateri := int64(len(readIDs))

	detail := &SiswaBabDetail{
		Bab: SiswaBabItem{
			ID:        b.ID,
			Nomor:     b.Nomor,
			Judul:     b.Judul,
			Deskripsi: b.Deskripsi,
			Urutan:    b.Urutan,
			Status:    b.Status,
			Progress:  computeProgress(totalMateri, readMateri),
		},
		Materi: cards,
	}
	return detail, nil
}

// assertEnrollment verifies the siswa has an active enrollment in kelas.
// Missing row OR status≠active → ErrForbidden.
func (s *Service) assertEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	enr, err := s.enroll.FindEnrollment(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return bab.ErrForbidden
	}
	if err != nil {
		return fmt.Errorf("siswa enrollment lookup: %w", err)
	}
	if enr.Status != kelas.EnrollmentActive {
		return bab.ErrForbidden
	}
	return nil
}

// computeProgress applies the Fase-3-partial formula. Materi-only weight
// goes 100% in Fase 3; future components (latihan/ulangan/tugas) land
// pct=nil + w=0 so FE can render placeholders without API churn.
func computeProgress(total, read int64) Progress {
	breakdown := map[string]ProgressBreakdownItem{
		"materi": {Pct: nil, Weight: 1.0},
	}
	prog := Progress{
		Persen:     0,
		Breakdown:  breakdown,
		BabKosong:  total == 0,
		MateriRead: int(read),
		MateriAll:  int(total),
	}
	if total > 0 {
		pct := float64(read) / float64(total) * 100.0
		// Round to 2 decimals to keep response stable across DB providers.
		pct = float64(int64(pct*100+0.5)) / 100.0
		prog.Persen = pct
		item := breakdown["materi"]
		item.Pct = &pct
		breakdown["materi"] = item
	}
	return prog
}

// ---------- HTTP handlers ----------

// Handler wires HTTP routes to Service.
type Handler struct {
	svc serviceAPI
}

type serviceAPI interface {
	ListSiswa(ctx context.Context, kelasID, siswaID uuid.UUID, callerRole string) ([]SiswaBabItem, error)
	GetSiswa(ctx context.Context, babID, siswaID uuid.UUID, callerRole string) (*SiswaBabDetail, error)
}

// NewHandler returns a siswa-side bab HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// SiswaListResponse is the body of GET /siswa/kelas/:id/bab.
type SiswaListResponse struct {
	Items []SiswaBabItem `json:"items"`
	Total int            `json:"total"`
}

// ListSiswa handles GET /api/v1/siswa/kelas/:id/bab.
//
// 200 → { items: [SiswaBabItem...], total: int }
// 400 invalid_id   → :id bukan UUID
// 403 forbidden    → caller bukan siswa atau siswa tidak enroll
// 500 internal     → unexpected error
func (h *Handler) ListSiswa(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	rows, err := h.svc.ListSiswa(c.UserContext(), kelasID, callerID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(SiswaListResponse{Items: rows, Total: len(rows)})
}

// GetSiswa handles GET /api/v1/siswa/bab/:id.
//
// 200 → SiswaBabDetail
// 400 invalid_id   → :id bukan UUID
// 403 forbidden    → caller bukan siswa atau siswa tidak enroll
// 404 not_found    → bab missing OR status≠published
// 500 internal     → unexpected error
func (h *Handler) GetSiswa(c *fiber.Ctx) error {
	babID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid bab id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	detail, err := h.svc.GetSiswa(c.UserContext(), babID, callerID, role)
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(detail)
}

// mapErr translates Service sentinel errors to HTTP responses.
func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, bab.ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, bab.ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "bab not found", "not_found")
	default:
		slog.Error("siswabab handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
