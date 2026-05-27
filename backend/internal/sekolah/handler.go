package sekolah

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

type Handler struct {
	repo *Repo
}

func NewHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

type listResponse struct {
	Items      []Sekolah `json:"items"`
	Page       int       `json:"page"`
	PageSize   int       `json:"page_size"`
	Total      int64     `json:"total"`
	TotalPages int       `json:"total_pages"`
}

type upsertRequest struct {
	Nama                     string `json:"nama"`
	NPSN                     string `json:"npsn"`
	Alamat                   string `json:"alamat"`
	SiswaRegistrationEnabled *bool  `json:"siswa_registration_enabled"`
	SiswaRegistrationMode    string `json:"siswa_registration_mode"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	page, pageSize := pagination(c)
	rows, total, err := h.repo.List(c.UserContext(), c.Query("q"), pageSize, (page-1)*pageSize)
	if err != nil {
		slog.Error("sekolah list failed", slog.String("err", err.Error()))
		return sekolahError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return c.Status(fiber.StatusOK).JSON(listResponse{Items: rows, Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var req upsertRequest
	if err := c.BodyParser(&req); err != nil {
		return sekolahError(c, fiber.StatusBadRequest, "invalid json body", "invalid_json")
	}
	row, err := normalize(req)
	if err != nil {
		return sekolahError(c, fiber.StatusBadRequest, err.Error(), "invalid_input")
	}
	if err := h.repo.Create(c.UserContext(), row); err != nil {
		slog.Error("sekolah create failed", slog.String("err", err.Error()))
		return sekolahError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"sekolah": row})
}

func (h *Handler) Update(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return sekolahError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	var req upsertRequest
	if err := c.BodyParser(&req); err != nil {
		return sekolahError(c, fiber.StatusBadRequest, "invalid json body", "invalid_json")
	}
	row, err := normalize(req)
	if err != nil {
		return sekolahError(c, fiber.StatusBadRequest, err.Error(), "invalid_input")
	}
	updated, err := h.repo.Update(c.UserContext(), id, row.Nama, row.NPSN, row.Alamat, row.SiswaRegistrationEnabled, row.SiswaRegistrationMode)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return sekolahError(c, fiber.StatusNotFound, "sekolah not found", "not_found")
	}
	if err != nil {
		slog.Error("sekolah update failed", slog.String("err", err.Error()))
		return sekolahError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"sekolah": updated})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return sekolahError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}
	if err := h.repo.Delete(c.UserContext(), id); errors.Is(err, gorm.ErrRecordNotFound) {
		return sekolahError(c, fiber.StatusNotFound, "sekolah not found", "not_found")
	} else if err != nil {
		slog.Error("sekolah delete failed", slog.String("err", err.Error()))
		return sekolahError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func normalize(req upsertRequest) (*Sekolah, error) {
	nama := strings.TrimSpace(req.Nama)
	if nama == "" {
		return nil, errors.New("nama is required")
	}
	npsn := strings.TrimSpace(req.NPSN)
	var npsnPtr *string
	if npsn != "" {
		npsnPtr = &npsn
	}
	mode := strings.TrimSpace(req.SiswaRegistrationMode)
	if mode == "" {
		mode = "approval_required"
	}
	if mode != "approval_required" && mode != "auto_approve" {
		return nil, errors.New("invalid siswa registration mode")
	}
	enabled := false
	if req.SiswaRegistrationEnabled != nil {
		enabled = *req.SiswaRegistrationEnabled
	}
	return &Sekolah{
		Nama:                     nama,
		NPSN:                     npsnPtr,
		Alamat:                   strings.TrimSpace(req.Alamat),
		SiswaRegistrationEnabled: enabled,
		SiswaRegistrationMode:    mode,
	}, nil
}

func pagination(c *fiber.Ctx) (int, int) {
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
	return page, pageSize
}

func sekolahError(c *fiber.Ctx, status int, msg, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg, "code": code})
}
