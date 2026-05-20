// HTTP handler untuk endpoint guru/admin list enrollment per kelas.
// Mounted GET /api/v1/kelas/:id/enrollments. Stays di kelas package biar share
// Service + audit plumbing (Task 2.C.4).
package kelas

import (
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/pikip/lms/backend/internal/middleware"
)

// enrollmentListResponse is the envelope for GET /kelas/:id/enrollments.
type enrollmentListResponse struct {
	Items      []EnrollmentItem `json:"items"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	Total      int64            `json:"total"`
	TotalPages int              `json:"total_pages"`
}

// ListEnrollments handles GET /api/v1/kelas/:id/enrollments — guru-owner OR
// admin lists the active enrollment roster of a kelas, hydrated dengan nama
// + email siswa. Pagination via ?page=&page_size= (max 100). Removed
// enrollments hidden by default; admin can opt in via ?include_removed=true.
//
// Locked decision (v0.7.2 Task 2.C.4): guru read-only di MVP — handler ini
// gak punya endpoint remove. Bulk remove + admin-side roster management
// dipindah ke Fase 2 backlog atau v0.9.
func (h *Handler) ListEnrollments(c *fiber.Ctx) error {
	kelasID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return kelasError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	callerID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	role, _ := c.Locals(middleware.LocalsUserRole).(string)

	page, pageSize := pagination(c)
	includeRemoved := strings.EqualFold(strings.TrimSpace(c.Query("include_removed")), "true")

	res, err := h.svc.ListEnrollmentsByKelas(c.UserContext(), kelasID, callerID, role, EnrollmentListInput{
		IncludeRemoved: includeRemoved,
		Limit:          pageSize,
		Offset:         (page - 1) * pageSize,
	})
	if err != nil {
		return mapServiceErr(c, err)
	}

	totalPagesCount := 0
	if res.Total > 0 {
		totalPagesCount = int(math.Ceil(float64(res.Total) / float64(pageSize)))
	}
	return c.Status(fiber.StatusOK).JSON(enrollmentListResponse{
		Items:      res.Items,
		Page:       page,
		PageSize:   pageSize,
		Total:      res.Total,
		TotalPages: totalPagesCount,
	})
}
