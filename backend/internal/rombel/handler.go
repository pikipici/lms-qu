package rombel

import (
	"errors"
	"log/slog"
	"math"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/middleware"
	"gorm.io/gorm"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type listResponse struct {
	Items      []Rombel `json:"items"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
	Total      int64    `json:"total"`
	TotalPages int      `json:"total_pages"`
}
type createRequest struct {
	Nama      string `json:"nama"`
	Deskripsi string `json:"deskripsi"`
}
type updateRequest struct {
	Version   int    `json:"version"`
	Nama      string `json:"nama"`
	Deskripsi string `json:"deskripsi"`
}

func (h *Handler) ListBySekolah(c *fiber.Ctx) error {
	sekolahID, err := uuid.Parse(c.Params("sekolah_id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid sekolah id", "invalid_sekolah_id")
	}
	page, pageSize := pagination(c)
	res, err := h.svc.ListBySekolah(c.UserContext(), sekolahID, ListInput{IncludeArchived: strings.EqualFold(c.Query("include_archived"), "true"), Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		slog.Error("rombel list failed", slog.String("err", err.Error()))
		return rombelError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.JSON(listResponse{Items: res.Items, Page: page, PageSize: pageSize, Total: res.Total, TotalPages: int(math.Ceil(float64(res.Total) / float64(pageSize)))})
}

func (h *Handler) ListPublicBySekolah(c *fiber.Ctx) error {
	sekolahID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid sekolah id", "invalid_sekolah_id")
	}
	rows, err := h.svc.ListPublicBySekolah(c.UserContext(), sekolahID)
	if err != nil {
		slog.Error("public rombel list failed", slog.String("err", err.Error()))
		return rombelError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.JSON(fiber.Map{"data": rows})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	sekolahID, err := uuid.Parse(c.Params("sekolah_id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid sekolah id", "invalid_sekolah_id")
	}
	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	row, err := h.svc.Create(c.UserContext(), sekolahID, CreateInput{Nama: req.Nama, Deskripsi: req.Deskripsi})
	if err != nil {
		return mapErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"rombel": row})
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	var req updateRequest
	if err := c.BodyParser(&req); err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	row, err := h.svc.Update(c.UserContext(), id, UpdateInput{Version: req.Version, Nama: req.Nama, Deskripsi: req.Deskripsi})
	if err != nil {
		return mapErr(c, err)
	}
	return c.JSON(fiber.Map{"rombel": row})
}

func (h *Handler) Archive(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	row, err := h.svc.Archive(c.UserContext(), id)
	if err != nil {
		return mapErr(c, err)
	}
	return c.JSON(fiber.Map{"rombel": row})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return rombelError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	if err := h.svc.DeleteIfEmpty(c.UserContext(), id); err != nil {
		return mapErr(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func pagination(c *fiber.Ctx) (int, int) {
	p := c.QueryInt("page", 1)
	ps := c.QueryInt("page_size", 20)
	if p < 1 {
		p = 1
	}
	if ps < 1 {
		ps = 20
	}
	if ps > 100 {
		ps = 100
	}
	return p, ps
}

func mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return rombelError(c, fiber.StatusBadRequest, "invalid rombel input", "invalid_input")
	case errors.Is(err, ErrVersionConflict):
		return rombelError(c, fiber.StatusConflict, "rombel version conflict", "version_conflict")
	case errors.Is(err, ErrNotEmpty):
		return rombelError(c, fiber.StatusConflict, "rombel is not empty", "rombel_not_empty")
	case errors.Is(err, gorm.ErrRecordNotFound):
		return rombelError(c, fiber.StatusNotFound, "rombel not found", "not_found")
	case strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "SQLSTATE 23505"):
		return rombelError(c, fiber.StatusConflict, "rombel name already exists", "duplicate_name")
	default:
		slog.Error("rombel request failed", slog.String("err", err.Error()))
		return rombelError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func rombelError(c *fiber.Ctx, status int, msg, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg, "code": code, "request_id": middleware.RequestIDFromFiber(c)})
}
