package registration

import (
	"errors"
	"log/slog"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/middleware"
	"gorm.io/gorm"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]{3,32}$`)

type Handler struct {
	repo *Repo
	cfg  *config.Config
}

func NewHandler(repo *Repo, cfg *config.Config) *Handler { return &Handler{repo: repo, cfg: cfg} }

type registerRequest struct {
	Nama            string `json:"nama"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	PasswordConfirm string `json:"password_confirm"`
	SekolahID       string `json:"sekolah_id"`
	KelasID         string `json:"kelas_id"`
	RombelID        string `json:"rombel_id"`
}

func (h *Handler) ListPublicSekolah(c *fiber.Ctx) error {
	rows, err := h.repo.ListPublicSekolah(c.UserContext())
	if err != nil {
		slog.Error("registration list sekolah failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.JSON(fiber.Map{"data": rows})
}

func (h *Handler) ListPublicKelas(c *fiber.Ctx) error {
	sekolahID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return regError(c, fiber.StatusBadRequest, "invalid sekolah id", "invalid_sekolah_id")
	}
	rows, err := h.repo.ListPublicKelas(c.UserContext(), sekolahID)
	if err != nil {
		slog.Error("registration list kelas failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.JSON(fiber.Map{"data": rows})
}

func (h *Handler) RegisterSiswa(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return regError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	name := strings.TrimSpace(req.Nama)
	if len(name) < 2 || len(name) > 100 {
		return regError(c, fiber.StatusBadRequest, "nama must be 2-100 characters", "invalid_name")
	}
	username := strings.ToLower(strings.TrimSpace(req.Username))
	if !usernamePattern.MatchString(username) {
		return regError(c, fiber.StatusBadRequest, "username must be 3-32 chars and use a-z, 0-9, dot, dash, or underscore", "invalid_username")
	}
	if len(req.Password) < 8 {
		return regError(c, fiber.StatusBadRequest, "password must be at least 8 characters", "weak_password")
	}
	if req.Password != req.PasswordConfirm {
		return regError(c, fiber.StatusBadRequest, "password confirmation does not match", "password_mismatch")
	}
	sekolahID, err := uuid.Parse(req.SekolahID)
	if err != nil {
		return regError(c, fiber.StatusBadRequest, "invalid sekolah id", "invalid_sekolah_id")
	}
	var kelasID *uuid.UUID
	if strings.TrimSpace(req.KelasID) != "" {
		parsed, err := uuid.Parse(req.KelasID)
		if err != nil {
			return regError(c, fiber.StatusBadRequest, "invalid kelas id", "invalid_kelas_id")
		}
		kelasID = &parsed
	}
	var rombelID *uuid.UUID
	if strings.TrimSpace(req.RombelID) != "" {
		parsed, err := uuid.Parse(req.RombelID)
		if err != nil {
			return regError(c, fiber.StatusBadRequest, "invalid rombel id", "invalid_rombel_id")
		}
		rombelID = &parsed
	}
	if kelasID == nil && rombelID == nil {
		return regError(c, fiber.StatusBadRequest, "rombel_id is required", "rombel_required")
	}

	cost := 0
	if h.cfg != nil {
		cost = h.cfg.JWT.BcryptCost
	}
	hash, err := auth.HashPassword(req.Password, cost)
	if err != nil {
		slog.Error("registration hash password failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	user := &auth.User{
		ID:                 uuid.New(),
		Name:               name,
		Email:              username,
		PasswordHash:       hash,
		Role:               auth.Siswa,
		Status:             auth.Active,
		MustChangePassword: false,
	}
	mode, err := h.repo.RegisterSiswa(c.UserContext(), user, sekolahID, kelasID, rombelID)
	if err != nil {
		return h.mapRegisterError(c, err)
	}
	status := "pending"
	message := "Pendaftaran berhasil. Menunggu persetujuan admin/guru."
	if mode == ModeAutoApprove {
		status = "active"
		message = "Pendaftaran berhasil. Kamu sudah masuk kelas."
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"status": "registered", "enrollment_status": status, "message": message})
}

func (h *Handler) ListRequests(c *fiber.Ctx) error {
	status := strings.TrimSpace(c.Query("status"))
	if status == "" {
		status = RequestPending
	}
	if status != RequestPending && status != RequestApproved && status != RequestRejected {
		return regError(c, fiber.StatusBadRequest, "invalid status", "invalid_status")
	}
	var guruID *uuid.UUID
	if c.Locals(middleware.LocalsUserRole) == string(auth.Guru) {
		id, err := middleware.UserIDFromCtx(c)
		if err != nil {
			return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		guruID = &id
	}
	rows, err := h.repo.ListRequests(c.UserContext(), status, guruID)
	if err != nil {
		slog.Error("registration list requests failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	return c.JSON(fiber.Map{"items": rows})
}

type rejectRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) Approve(c *fiber.Ctx) error {
	return h.decide(c, true)
}

func (h *Handler) Reject(c *fiber.Ctx) error {
	return h.decide(c, false)
}

func (h *Handler) decide(c *fiber.Ctx, approve bool) error {
	requestID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return regError(c, fiber.StatusBadRequest, "invalid request id", "invalid_id")
	}
	actorID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	var guruID *uuid.UUID
	if c.Locals(middleware.LocalsUserRole) == string(auth.Guru) {
		guruID = &actorID
	}
	if approve {
		err = h.repo.Approve(c.UserContext(), requestID, actorID, guruID)
	} else {
		var req rejectRequest
		if len(c.Body()) > 0 {
			if parseErr := c.BodyParser(&req); parseErr != nil {
				return regError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
			}
		}
		err = h.repo.Reject(c.UserContext(), requestID, actorID, strings.TrimSpace(req.Reason), guruID)
	}
	if err != nil {
		return h.mapDecisionError(c, err)
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *Handler) mapRegisterError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrRegistrationDisabled):
		return regError(c, fiber.StatusForbidden, "registration disabled", "registration_disabled")
	case errors.Is(err, ErrKelasNotInSekolah):
		return regError(c, fiber.StatusBadRequest, "kelas does not belong to selected sekolah", "kelas_not_in_sekolah")
	case errors.Is(err, ErrRombelNotInSekolah):
		return regError(c, fiber.StatusBadRequest, "rombel does not belong to selected sekolah", "rombel_not_in_sekolah")
	case errors.Is(err, gorm.ErrRecordNotFound):
		return regError(c, fiber.StatusBadRequest, "invalid sekolah or kelas", "invalid_reference")
	case strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "SQLSTATE 23505"):
		return regError(c, fiber.StatusConflict, "username already exists or join request already exists", "username_taken")
	default:
		slog.Error("registration failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func (h *Handler) mapDecisionError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return regError(c, fiber.StatusNotFound, "join request not found", "join_request_not_found")
	case errors.Is(err, ErrRequestNotPending):
		return regError(c, fiber.StatusConflict, "join request is not pending", "join_request_not_pending")
	case errors.Is(err, ErrForbiddenScope):
		return regError(c, fiber.StatusForbidden, "forbidden kelas scope", "forbidden_kelas_scope")
	default:
		slog.Error("registration decision failed", slog.String("err", err.Error()))
		return regError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
}

func regError(c *fiber.Ctx, status int, msg, code string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg, "code": code, "request_id": middleware.RequestIDFromFiber(c)})
}
