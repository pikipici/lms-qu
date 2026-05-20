// HTTP handler untuk siswa-side endpoint list kelas yang sudah dijoin.
// Mounted GET /api/v1/siswa/kelas. Tetap di kelas package biar share Service
// + audit plumbing dgn enrollment_handler.go.
package kelas

import (
	"log/slog"
	"math"

	"github.com/gofiber/fiber/v2"

	"github.com/pikip/lms/backend/internal/middleware"
)

type myKelasResponse struct {
	Items      []MyKelasItem `json:"items"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	Total      int64         `json:"total"`
	TotalPages int           `json:"total_pages"`
}

// ListMyKelas handles GET /api/v1/siswa/kelas — siswa lists their own active
// enrollments hydrated with kelas detail. Pagination via ?page=&page_size=
// (defaults match other list endpoints; max 100). Removed enrollments are
// hidden so the FE shows only kelas the siswa can still access.
func (h *Handler) ListMyKelas(c *fiber.Ctx) error {
	siswaID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", defaultPageSize)
	if pageSize < 1 {
		pageSize = 1
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	res, err := h.svc.ListMyKelas(c.UserContext(), siswaID, ListInput{
		Limit:  pageSize,
		Offset: (page - 1) * pageSize,
	})
	if err != nil {
		slog.Error("kelas list my failed", slog.String("err", err.Error()))
		return kelasError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	totalPages := 0
	if res.Total > 0 {
		totalPages = int(math.Ceil(float64(res.Total) / float64(pageSize)))
	}
	return c.Status(fiber.StatusOK).JSON(myKelasResponse{
		Items:      res.Items,
		Page:       page,
		PageSize:   pageSize,
		Total:      res.Total,
		TotalPages: totalPages,
	})
}
