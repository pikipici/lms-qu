// HTTP handlers untuk ujian.
//
// Endpoints (Task 6.C.1 — CRUD + duplicate):
//   - POST   /api/v1/kelas/:id/ujian       (admin/guru owner)
//   - GET    /api/v1/kelas/:id/ujian       (admin/guru owner; siswa published-only)
//   - GET    /api/v1/ujian/:id             (admin/guru owner; siswa published-only)
//   - PATCH  /api/v1/ujian/:id             (admin/guru owner)
//   - DELETE /api/v1/ujian/:id             (admin/guru owner)
//   - POST   /api/v1/ujian/:id/duplicate   (admin/guru owner)
//
// Task 6.C.2 — source dispatch + preview:
//   - POST   /api/v1/ujian/:id/source/preview  (admin/guru owner)
//
// Source change actually persisted via PATCH body { source: {...} }.
package ujian

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/middleware"
)

// ujianService is the subset of *Service the handler depends on.
type ujianService interface {
	Create(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in CreateInput, ip, userAgent string) (*Ujian, error)
	ListByKelas(ctx context.Context, kelasID, callerID uuid.UUID, callerRole string, in ListInput) ([]Ujian, error)
	Get(ctx context.Context, id, callerID uuid.UUID, callerRole string) (*Ujian, error)
	Update(ctx context.Context, id, callerID uuid.UUID, callerRole string, in UpdateInput, ip, userAgent string) (*Ujian, error)
	Delete(ctx context.Context, id, callerID uuid.UUID, callerRole string, expectedVersion int, ip, userAgent string) (*Ujian, error)
	Duplicate(ctx context.Context, srcID, callerID uuid.UUID, callerRole string, in DuplicateInput, ip, userAgent string) (*Ujian, error)
	PreviewSource(ctx context.Context, ujianID, callerID uuid.UUID, callerRole string, in SourceInput) (*SourcePreview, error)
}

// Handler wires HTTP routes to ujian Service.
type Handler struct {
	svc ujianService
}

// NewHandler returns an ujian HTTP handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// ListResponse shape for GET ujian list.
type ListResponse struct {
	Items  []Ujian `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

// DefaultListLimit caps the result set when caller doesn't specify limit.
const DefaultListLimit = 50

// MaxListLimit clamps the upper bound.
const MaxListLimit = 200

// sourceRequest is the discriminated source DTO used in create/update/preview.
type sourceRequest struct {
	Mode       string       `json:"mode"`
	SoalIDs    []string     `json:"soal_ids,omitempty"`
	Filter     *filterDTO   `json:"filter,omitempty"`
	JumlahSoal int          `json:"jumlah_soal,omitempty"`
}

type filterDTO struct {
	Mapel   string `json:"mapel,omitempty"`
	Tingkat string `json:"tingkat,omitempty"`
	Topik   string `json:"topik,omitempty"`
}

// parseSource decodes a sourceRequest into the service-side SourceInput.
// Returns nil pointer kalau req nil/empty.
func parseSource(req *sourceRequest) (*SourceInput, error) {
	if req == nil {
		return nil, nil
	}
	switch SourceMode(req.Mode) {
	case SourceManual:
		ids := make([]uuid.UUID, 0, len(req.SoalIDs))
		for _, raw := range req.SoalIDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil {
				return nil, errInvalidSourceSoalID
			}
			ids = append(ids, id)
		}
		return &SourceInput{Manual: &ManualSourceConfig{
			Mode:    SourceManual,
			SoalIDs: ids,
		}}, nil
	case SourceRandom:
		f := RandomFilter{}
		if req.Filter != nil {
			f = RandomFilter{
				Mapel:   req.Filter.Mapel,
				Tingkat: req.Filter.Tingkat,
				Topik:   req.Filter.Topik,
			}
		}
		return &SourceInput{Random: &RandomSourceConfig{
			Mode:       SourceRandom,
			Filter:     f,
			JumlahSoal: req.JumlahSoal,
		}}, nil
	case "":
		return nil, errSourceMissingMode
	default:
		return nil, errSourceInvalidMode
	}
}

// Sentinel handler-side errors for parsing.
var (
	errInvalidSourceSoalID = errors.New("source.soal_ids contains invalid uuid")
	errSourceMissingMode   = errors.New("source.mode required (manual|random)")
	errSourceInvalidMode   = errors.New("source.mode must be manual|random")
)

// timestampDTO captures a *time.Time field with explicit-null semantics.
// FE sends `null` to clear (column → NULL), omits to leave unchanged.
type timestampDTO = json.RawMessage

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

type createRequest struct {
	Judul                      string         `json:"judul"`
	Deskripsi                  string         `json:"deskripsi"`
	DurasiMenit                int16          `json:"durasi_menit"`
	WaktuMulai                 *string        `json:"waktu_mulai,omitempty"`
	WaktuSelesai               *string        `json:"waktu_selesai,omitempty"`
	IzinkanReviewSetelahSubmit bool           `json:"izinkan_review_setelah_submit"`
	WaktuBukaReview            *string        `json:"waktu_buka_review,omitempty"`
	Status                     *string        `json:"status,omitempty"`
	Source                     *sourceRequest `json:"source,omitempty"`
}

// Create handles POST /api/v1/kelas/:id/ujian.
func (h *Handler) Create(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	mulaiAt, err := parseOptionalRFC3339(req.WaktuMulai)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_mulai must be RFC3339", "invalid_body")
	}
	selesaiAt, err := parseOptionalRFC3339(req.WaktuSelesai)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_selesai must be RFC3339", "invalid_body")
	}
	bukaReview, err := parseOptionalRFC3339(req.WaktuBukaReview)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_buka_review must be RFC3339", "invalid_body")
	}

	source, perr := parseSource(req.Source)
	if perr != nil {
		return errResp(c, fiber.StatusBadRequest, perr.Error(), "invalid_source")
	}

	in := CreateInput{
		Judul:                      req.Judul,
		Deskripsi:                  req.Deskripsi,
		DurasiMenit:                req.DurasiMenit,
		WaktuMulai:                 mulaiAt,
		WaktuSelesai:               selesaiAt,
		IzinkanReviewSetelahSubmit: req.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            bukaReview,
		Source:                     source,
	}
	if req.Status != nil {
		st := Status(strings.TrimSpace(*req.Status))
		in.Status = &st
	}

	u, err := h.svc.Create(c.UserContext(), kelasID, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ujian": u})
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

// ListByKelas handles GET /api/v1/kelas/:id/ujian.
func (h *Handler) ListByKelas(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	in := ListInput{Limit: DefaultListLimit}
	if rawStatus := strings.TrimSpace(c.Query("status")); rawStatus != "" {
		st := Status(rawStatus)
		if !st.Valid() {
			return errResp(c, fiber.StatusBadRequest, "status must be draft|published|archived", "invalid_status")
		}
		in.Status = &st
	}
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		n, perr := strconv.Atoi(rawLimit)
		if perr != nil || n <= 0 {
			return errResp(c, fiber.StatusBadRequest, "limit must be a positive integer", "invalid_limit")
		}
		if n > MaxListLimit {
			n = MaxListLimit
		}
		in.Limit = n
	}
	if rawOffset := strings.TrimSpace(c.Query("offset")); rawOffset != "" {
		n, perr := strconv.Atoi(rawOffset)
		if perr != nil || n < 0 {
			return errResp(c, fiber.StatusBadRequest, "offset must be a non-negative integer", "invalid_offset")
		}
		in.Offset = n
	}

	rows, err := h.svc.ListByKelas(c.UserContext(), kelasID, callerID, role, in)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(ListResponse{
		Items:  rows,
		Total:  len(rows),
		Limit:  in.Limit,
		Offset: in.Offset,
	})
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// Get handles GET /api/v1/ujian/:id.
func (h *Handler) Get(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	u, err := h.svc.Get(c.UserContext(), id, callerID, role)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ujian": u})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// updateRequest mirrors UpdateInput. *string for nullable fields.
// Use json.RawMessage rules: omitted vs explicit null distinguished
// via Has* booleans driven from raw map.
type updateRequest struct {
	Version                    int            `json:"version"`
	Judul                      *string        `json:"judul,omitempty"`
	Deskripsi                  *string        `json:"deskripsi,omitempty"`
	DurasiMenit                *int16         `json:"durasi_menit,omitempty"`
	WaktuMulai                 *string        `json:"waktu_mulai,omitempty"`
	WaktuSelesai               *string        `json:"waktu_selesai,omitempty"`
	IzinkanReviewSetelahSubmit *bool          `json:"izinkan_review_setelah_submit,omitempty"`
	WaktuBukaReview            *string        `json:"waktu_buka_review,omitempty"`
	Status                     *string        `json:"status,omitempty"`
	Source                     *sourceRequest `json:"source,omitempty"`
}

// Update handles PATCH /api/v1/ujian/:id.
//
// Catatan: explicit-null semantics untuk waktu_* fields didetect lewat
// raw map probe karena Fiber BodyParser tidak distinguish absent vs
// null. Body harus literal `"waktu_mulai": null` untuk clear.
func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	rawBody := c.Body()
	var req updateRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req.Version <= 0 {
		return errResp(c, fiber.StatusBadRequest, "version must be positive", "invalid_version")
	}

	// Probe raw map for explicit-null fields (timestamp clears).
	var probe map[string]json.RawMessage
	_ = json.Unmarshal(rawBody, &probe)
	hasMulai := keyPresent(probe, "waktu_mulai")
	hasSelesai := keyPresent(probe, "waktu_selesai")
	hasBuka := keyPresent(probe, "waktu_buka_review")

	mulaiAt, err := parseOptionalRFC3339(req.WaktuMulai)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_mulai must be RFC3339", "invalid_body")
	}
	selesaiAt, err := parseOptionalRFC3339(req.WaktuSelesai)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_selesai must be RFC3339", "invalid_body")
	}
	bukaReview, err := parseOptionalRFC3339(req.WaktuBukaReview)
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "waktu_buka_review must be RFC3339", "invalid_body")
	}

	source, perr := parseSource(req.Source)
	if perr != nil {
		return errResp(c, fiber.StatusBadRequest, perr.Error(), "invalid_source")
	}

	in := UpdateInput{
		ExpectedVersion:            req.Version,
		Judul:                      req.Judul,
		Deskripsi:                  req.Deskripsi,
		DurasiMenit:                req.DurasiMenit,
		WaktuMulai:                 mulaiAt,
		WaktuMulaiExplicit:         hasMulai,
		WaktuSelesai:               selesaiAt,
		WaktuSelesaiExplicit:       hasSelesai,
		IzinkanReviewSetelahSubmit: req.IzinkanReviewSetelahSubmit,
		WaktuBukaReview:            bukaReview,
		WaktuBukaReviewExplicit:    hasBuka,
		Source:                     source,
	}
	if req.Status != nil {
		st := Status(strings.TrimSpace(*req.Status))
		in.Status = &st
	}

	u, err := h.svc.Update(c.UserContext(), id, callerID, role, in, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"ujian": u})
}

func keyPresent(probe map[string]json.RawMessage, key string) bool {
	if probe == nil {
		return false
	}
	_, ok := probe[key]
	return ok
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

type deleteRequest struct {
	Version int `json:"version"`
}

// Delete handles DELETE /api/v1/ujian/:id.
//
// Body: { "version": <int> } untuk optimistic concurrency. Caller boleh
// taruh ?version= di query string juga (DELETE body optional di clients).
func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	expectedVersion := 0
	var req deleteRequest
	if err := c.BodyParser(&req); err == nil {
		expectedVersion = req.Version
	}
	if expectedVersion <= 0 {
		if rawV := strings.TrimSpace(c.Query("version")); rawV != "" {
			if n, perr := strconv.Atoi(rawV); perr == nil {
				expectedVersion = n
			}
		}
	}
	if expectedVersion <= 0 {
		return errResp(c, fiber.StatusBadRequest, "version must be positive (body or ?version=)", "invalid_version")
	}

	u, err := h.svc.Delete(c.UserContext(), id, callerID, role, expectedVersion, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"ujian_id": u.ID,
		"deleted":  true,
	})
}

// ---------------------------------------------------------------------------
// Duplicate
// ---------------------------------------------------------------------------

type duplicateRequest struct {
	Judul string `json:"judul"`
}

// Duplicate handles POST /api/v1/ujian/:id/duplicate.
func (h *Handler) Duplicate(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req duplicateRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
		}
	}

	u, err := h.svc.Duplicate(c.UserContext(), id, callerID, role, DuplicateInput{Judul: req.Judul}, c.IP(), string(c.Request().Header.UserAgent()))
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ujian": u})
}

// ---------------------------------------------------------------------------
// Source preview (Task 6.C.2)
// ---------------------------------------------------------------------------

type previewRequest struct {
	Source sourceRequest `json:"source"`
}

// PreviewSource handles POST /api/v1/ujian/:id/source/preview.
//
// Returns { mode, pool_size, jumlah_soal, soal_ids[≤50] } so guru can
// confirm pool size + sample sebelum save.
func (h *Handler) PreviewSource(c *fiber.Ctx) error {
	id, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid ujian id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	var req previewRequest
	if err := c.BodyParser(&req); err != nil {
		return errResp(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	source, perr := parseSource(&req.Source)
	if perr != nil {
		return errResp(c, fiber.StatusBadRequest, perr.Error(), "invalid_source")
	}
	if source == nil {
		return errResp(c, fiber.StatusBadRequest, "source required", "invalid_source")
	}

	preview, err := h.svc.PreviewSource(c.UserContext(), id, callerID, role, *source)
	if err != nil {
		return mapServiceErr(c, err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"preview": preview})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mapServiceErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrSoalNotInBank):
		return errResp(c, fiber.StatusBadRequest, "salah satu soal tidak ada di bank guru ini", "soal_not_in_bank")
	case errors.Is(err, ErrSoalEmpty):
		return errResp(c, fiber.StatusBadRequest, "filter random tidak menghasilkan soal", "source_pool_empty")
	case errors.Is(err, ErrSourceMissing):
		return errResp(c, fiber.StatusBadRequest, "source.mode required (manual|random)", "source_missing")
	case errors.Is(err, ErrInvalidInput):
		return errResp(c, fiber.StatusBadRequest, friendlyMessage(err, "invalid input"), "invalid_body")
	case errors.Is(err, ErrAttemptsExist):
		return errResp(c, fiber.StatusConflict, "ujian sudah dipakai siswa; tidak bisa dihapus, archive saja", "attempts_exist")
	case errors.Is(err, ErrActiveAttemptsBlock):
		return errResp(c, fiber.StatusConflict, "ada attempt aktif; cancel attempt dulu sebelum ubah timing/source", "active_attempts_block")
	case errors.Is(err, ErrKelasArchived):
		return errResp(c, fiber.StatusConflict, "kelas archived; ujian tidak bisa diubah", "kelas_archived")
	case errors.Is(err, ErrNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		return errResp(c, fiber.StatusNotFound, "ujian not found", "not_found")
	case errors.Is(err, ErrForbidden):
		return errResp(c, fiber.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, ErrVersionConflict):
		return errResp(c, fiber.StatusConflict, "ujian has been modified by another request; please refresh", "version_conflict")
	default:
		slog.Error("ujian handler", slog.String("err", err.Error()))
		return errResp(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func friendlyMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	msg := err.Error()
	const sep = ": "
	if idx := strings.Index(msg, sep); idx >= 0 {
		rest := msg[idx+len(sep):]
		if idx2 := strings.Index(rest, sep); idx2 >= 0 {
			return rest[idx2+len(sep):]
		}
		return rest
	}
	return fallback
}

func errResp(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
