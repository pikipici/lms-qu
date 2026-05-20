// Admin bulk-enroll handler (Phase 2.C.2): admin assigns multiple siswa
// directly into a kelas without going through kode invite. Kept in a separate
// file/struct so existing admin.Handler tests stay untouched.
package admin

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/kelas"
	"github.com/pikip/lms/backend/internal/middleware"
)

// MaxBulkEnrollSize caps how many siswa_ids one bulk enroll request may carry.
// 100 is generous for typical class sizes while keeping the per-siswa fan-out
// (find user + find enrollment + enroll + audit row) bounded.
const MaxBulkEnrollSize = 100

// kelasEnrollUserRepo is the slim user-side surface BulkEnroll needs.
// Implemented by *auth.Repo; the small interface keeps this handler easy to
// mock without dragging in unrelated admin user-management methods.
type kelasEnrollUserRepo interface {
	FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

// kelasEnrollKelasRepo is the slim kelas-side surface BulkEnroll needs.
// Implemented by *kelas.Repo.
type kelasEnrollKelasRepo interface {
	FindByID(ctx context.Context, id uuid.UUID) (*kelas.Kelas, error)
	FindEnrollment(ctx context.Context, kelasID, siswaID uuid.UUID) (*kelas.Enrollment, error)
	Enroll(ctx context.Context, kelasID, siswaID uuid.UUID, via kelas.JoinedVia) (bool, error)
}

// KelasEnrollHandler owns admin bulk-enroll endpoints. Wired in main.go
// alongside the regular admin user-management handler.
type KelasEnrollHandler struct {
	users kelasEnrollUserRepo
	kelas kelasEnrollKelasRepo
	now   func() time.Time
}

// NewKelasEnrollHandler wires the user + kelas repo dependencies.
func NewKelasEnrollHandler(users kelasEnrollUserRepo, k kelasEnrollKelasRepo) *KelasEnrollHandler {
	return &KelasEnrollHandler{users: users, kelas: k, now: time.Now}
}

type bulkEnrollRequest struct {
	SiswaIDs []string `json:"siswa_ids"`
}

// BulkEnrollItem is one siswa entry in the response, optionally annotated with
// a reason (only populated for items in the "invalid" bucket).
type BulkEnrollItem struct {
	SiswaID string `json:"siswa_id"`
	Reason  string `json:"reason,omitempty"`
}

// BulkEnrollResponse is the admin bulk-enroll summary. Slices are always
// non-nil so the JSON response is consistent for clients.
type BulkEnrollResponse struct {
	Enrolled        []BulkEnrollItem `json:"enrolled"`
	AlreadyEnrolled []BulkEnrollItem `json:"already_enrolled"`
	Invalid         []BulkEnrollItem `json:"invalid"`
}

// Reason codes for the invalid bucket. Stable keys so the FE can map to UI
// copy without parsing free-form strings.
const (
	ReasonInvalidUUID         = "invalid_uuid"
	ReasonDuplicateInRequest  = "duplicate_in_request"
	ReasonUserNotFound        = "user_not_found"
	ReasonNotSiswa            = "not_siswa"
	ReasonUserInactive        = "user_inactive"
	ReasonEnrollmentRemoved   = "enrollment_removed"
	ReasonInternal            = "internal"
)

// Audit result values written into the per-siswa audit log meta.
const (
	auditResultEnrolled        = "enrolled"
	auditResultAlreadyEnrolled = "already_enrolled"
	auditActionAssign          = "admin_assigned_siswa_to_kelas"
)

// BulkEnroll handles POST /api/v1/admin/kelas/:id/enroll. Body: {siswa_ids:[]}.
//
// The endpoint is idempotent and partial-success: each siswa is classified
// independently into one of the three buckets and a 200 is returned even when
// some entries fail. Hard preconditions (invalid kelas id, kelas not found,
// kelas archived, missing/oversize body) short-circuit with 4xx before any
// per-siswa work happens.
func (h *KelasEnrollHandler) BulkEnroll(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_id")
	}

	var req bulkEnrollRequest
	if err := c.BodyParser(&req); err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if len(req.SiswaIDs) == 0 {
		return adminError(c, fiber.StatusBadRequest, "siswa_ids is required", "siswa_ids_required")
	}
	if len(req.SiswaIDs) > MaxBulkEnrollSize {
		return adminError(c, fiber.StatusBadRequest, "too many siswa_ids (max 100 per request)", "too_many")
	}

	k, err := h.kelas.FindByID(c.UserContext(), kelasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "kelas not found", "kelas_not_found")
	}
	if err != nil {
		slog.Error("admin bulk enroll find kelas failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	if k.ArchivedAt != nil {
		return adminError(c, fiber.StatusConflict, "kelas is archived", "kelas_archived")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	ip := c.IP()
	ua := string(c.Request().Header.UserAgent())

	resp := BulkEnrollResponse{
		Enrolled:        []BulkEnrollItem{},
		AlreadyEnrolled: []BulkEnrollItem{},
		Invalid:         []BulkEnrollItem{},
	}
	seen := make(map[uuid.UUID]struct{}, len(req.SiswaIDs))

	for _, raw := range req.SiswaIDs {
		trimmed := strings.TrimSpace(raw)
		sid, perr := uuid.Parse(trimmed)
		if perr != nil {
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: trimmed, Reason: ReasonInvalidUUID})
			continue
		}
		if _, dup := seen[sid]; dup {
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonDuplicateInRequest})
			continue
		}
		seen[sid] = struct{}{}

		u, ferr := h.users.FindUserByID(c.UserContext(), sid)
		if errors.Is(ferr, gorm.ErrRecordNotFound) {
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonUserNotFound})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, "invalid_"+ReasonUserNotFound)
			continue
		}
		if ferr != nil {
			slog.Error("admin bulk enroll find user failed",
				slog.String("siswa_id", sid.String()), slog.String("err", ferr.Error()))
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonInternal})
			continue
		}
		if u.Role != auth.Siswa {
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonNotSiswa})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, "invalid_"+ReasonNotSiswa)
			continue
		}
		if u.Status != auth.Active {
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonUserInactive})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, "invalid_"+ReasonUserInactive)
			continue
		}

		existing, eerr := h.kelas.FindEnrollment(c.UserContext(), kelasID, sid)
		if eerr == nil {
			if existing.Status == kelas.EnrollmentRemoved {
				resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonEnrollmentRemoved})
				h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, "invalid_"+ReasonEnrollmentRemoved)
				continue
			}
			// Active prior enrollment.
			resp.AlreadyEnrolled = append(resp.AlreadyEnrolled, BulkEnrollItem{SiswaID: sid.String()})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, auditResultAlreadyEnrolled)
			continue
		}
		if !errors.Is(eerr, gorm.ErrRecordNotFound) {
			slog.Error("admin bulk enroll find enrollment failed",
				slog.String("siswa_id", sid.String()), slog.String("err", eerr.Error()))
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonInternal})
			continue
		}

		inserted, ierr := h.kelas.Enroll(c.UserContext(), kelasID, sid, kelas.JoinedViaAdmin)
		if ierr != nil {
			slog.Error("admin bulk enroll insert failed",
				slog.String("siswa_id", sid.String()), slog.String("err", ierr.Error()))
			resp.Invalid = append(resp.Invalid, BulkEnrollItem{SiswaID: sid.String(), Reason: ReasonInternal})
			continue
		}
		if inserted {
			resp.Enrolled = append(resp.Enrolled, BulkEnrollItem{SiswaID: sid.String()})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, auditResultEnrolled)
		} else {
			// Race: another caller enrolled between FindEnrollment and Enroll.
			// Classify as already_enrolled to keep response semantics stable.
			resp.AlreadyEnrolled = append(resp.AlreadyEnrolled, BulkEnrollItem{SiswaID: sid.String()})
			h.logEnrollAudit(c, adminID, sid, kelasID, ip, ua, auditResultAlreadyEnrolled)
		}
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

// logEnrollAudit writes one audit row per siswa attempt. Failures only emit a
// warning so the response stays authoritative even when the audit writer is
// flaky.
func (h *KelasEnrollHandler) logEnrollAudit(c *fiber.Ctx, adminID, siswaID, kelasID uuid.UUID, ip, ua, result string) {
	actorRole := string(auth.Admin)
	targetType := "user"
	entry := &auth.AuditLog{
		ActorID:       &adminID,
		ActorRole:     &actorRole,
		Action:        auditActionAssign,
		TargetType:    &targetType,
		TargetID:      &siswaID,
		TargetKelasID: &kelasID,
		Meta:          auditMeta(map[string]any{"result": result}),
		IP:            strPtr(ip),
		UserAgent:     strPtr(ua),
		At:            h.now(),
	}
	if err := h.users.LogAudit(c.UserContext(), entry); err != nil {
		slog.Warn("admin bulk enroll audit log failed",
			slog.String("siswa_id", siswaID.String()),
			slog.String("kelas_id", kelasID.String()),
			slog.String("result", result),
			slog.String("err", err.Error()))
	}
}
