// Package admin exposes admin-only account management handlers.
package admin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"github.com/pikip/lms/backend/internal/config"
	"github.com/pikip/lms/backend/internal/middleware"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	generatedLength = 16
	passwordCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789"
)

type userRepo interface {
	ListUsers(ctx context.Context, f auth.UserListFilter, limit, offset int) ([]auth.User, int64, error)
	FindUserByEmail(ctx context.Context, email string) (*auth.User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error)
	CreateUser(ctx context.Context, u *auth.User) error
	UpdateUserName(ctx context.Context, id uuid.UUID, name string) error
	UpdateUserRole(ctx context.Context, id uuid.UUID, role auth.UserRole) error
	SuspendUser(ctx context.Context, id uuid.UUID) error
	AdminResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error
	UnsuspendUser(ctx context.Context, id uuid.UUID) error
	UnlockUser(ctx context.Context, id uuid.UUID) error
	RevokeAllRefreshByUser(ctx context.Context, userID uuid.UUID, reason auth.RevokedReason) (int64, error)
	CountAdmins(ctx context.Context) (int64, error)
	LogAudit(ctx context.Context, entry *auth.AuditLog) error
}

type passwordVerifier func(hashed, plain string) error

type Handler struct {
	repo           userRepo
	cfg            *config.Config
	verifyPassword passwordVerifier
}

func NewHandler(repo userRepo, cfg *config.Config) *Handler {
	return &Handler{repo: repo, cfg: cfg, verifyPassword: auth.VerifyPassword}
}

type listUsersResponse struct {
	Users      []auth.User `json:"users"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
	Total      int64       `json:"total"`
	TotalPages int         `json:"total_pages"`
}

// ListUsers handles GET /api/v1/admin/users.
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	role := strings.ToLower(strings.TrimSpace(c.Query("role")))
	if role != "" && !validRole(role) {
		return adminError(c, fiber.StatusBadRequest, "invalid role", "invalid_role")
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status != "" && !validStatus(status) {
		return adminError(c, fiber.StatusBadRequest, "invalid status", "invalid_status")
	}

	page := c.QueryInt("page", 1)
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("page_size", defaultPageSize)
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	q := strings.TrimSpace(c.Query("q"))
	users, total, err := h.repo.ListUsers(c.UserContext(), auth.UserListFilter{
		Role:        role,
		Status:      status,
		SearchEmail: q,
		SearchName:  q,
	}, pageSize, (page-1)*pageSize)
	if err != nil {
		slog.Error("admin list users failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return c.Status(fiber.StatusOK).JSON(listUsersResponse{
		Users:      users,
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		TotalPages: totalPages,
	})
}

type createUserRequest struct {
	Name             string `json:"name"`
	Email            string `json:"email"`
	Role             string `json:"role"`
	PasswordStrategy string `json:"password_strategy"`
	Password         string `json:"password"`
}

type createUserResponse struct {
	User              *auth.User `json:"user"`
	GeneratedPassword *string    `json:"generated_password"`
}

type resetUserPasswordRequest struct {
	PasswordStrategy string `json:"password_strategy"`
	Password         string `json:"password"`
}

type resetUserPasswordResponse struct {
	User              *auth.User `json:"user"`
	GeneratedPassword *string    `json:"generated_password"`
}

type suspendUserRequest struct {
	Reason string `json:"reason"`
}

// CreateUser handles POST /api/v1/admin/users.
func (h *Handler) CreateUser(c *fiber.Ctx) error {
	var req createUserRequest
	if err := c.BodyParser(&req); err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return adminError(c, fiber.StatusBadRequest, "name is required", "invalid_body")
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		return adminError(c, fiber.StatusBadRequest, "email is required", "invalid_body")
	}

	role := strings.ToLower(strings.TrimSpace(req.Role))
	if !validRole(role) {
		return adminError(c, fiber.StatusBadRequest, "invalid role", "invalid_role")
	}

	strategy := strings.ToLower(strings.TrimSpace(req.PasswordStrategy))
	switch strategy {
	case "manual":
		if len(req.Password) < 8 {
			return adminError(c, fiber.StatusBadRequest, "password must be at least 8 characters", "weak_password")
		}
	case "generate":
		if req.Password != "" {
			return adminError(c, fiber.StatusBadRequest, "password must be empty when generating", "conflicting_password")
		}
	default:
		return adminError(c, fiber.StatusBadRequest, "invalid password strategy", "invalid_strategy")
	}

	if _, err := h.repo.FindUserByEmail(c.UserContext(), email); err == nil {
		return adminError(c, fiber.StatusConflict, "email already exists", "email_already_exists")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		slog.Error("admin find user by email failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	password := req.Password
	var generated *string
	if strategy == "generate" {
		plain, err := generatePassword(generatedLength)
		if err != nil {
			slog.Error("admin generate password failed", slog.String("err", err.Error()))
			return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		password = plain
		generated = &plain
	}

	cost := 0
	if h.cfg != nil {
		cost = h.cfg.JWT.BcryptCost
	}
	hash, err := auth.HashPassword(password, cost)
	if err != nil {
		slog.Error("admin hash password failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	newUser := &auth.User{
		ID:                 uuid.New(),
		Name:               name,
		Email:              email,
		PasswordHash:       hash,
		Role:               auth.UserRole(role),
		Status:             auth.Active,
		MustChangePassword: true,
		CreatedByID:        &adminID,
	}
	if err := h.repo.CreateUser(c.UserContext(), newUser); err != nil {
		slog.Error("admin create user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	h.logAudit(c, "admin_user_created", adminID, newUser.ID, auditMeta(map[string]any{
		"role":              role,
		"password_strategy": strategy,
	}))

	return c.Status(fiber.StatusCreated).JSON(createUserResponse{
		User:              newUser,
		GeneratedPassword: generated,
	})
}

type updateUserRequest struct {
	Name string `json:"name"`
}

type changeRoleRequest struct {
	NewRole         string `json:"new_role"`
	CurrentPassword string `json:"current_password"`
}

// UpdateUser handles PATCH /api/v1/admin/users/:id.
func (h *Handler) UpdateUser(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	var req updateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return adminError(c, fiber.StatusBadRequest, "name is required", "invalid_body")
	}

	existing, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if err := h.repo.UpdateUserName(c.UserContext(), targetID, name); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin update user name failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	h.logAudit(c, "admin_user_name_updated", adminID, targetID, auditMeta(map[string]any{
		"old_name": existing.Name,
		"new_name": name,
	}))

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": fresh})
}

// ChangeUserRole handles POST /api/v1/admin/users/:id/role.
func (h *Handler) ChangeUserRole(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	var req changeRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}
	if req == (changeRoleRequest{}) {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	newRole := strings.ToLower(strings.TrimSpace(req.NewRole))
	if !validRole(newRole) {
		return adminError(c, fiber.StatusBadRequest, "invalid role", "invalid_role")
	}
	if strings.TrimSpace(req.CurrentPassword) == "" {
		return adminError(c, fiber.StatusBadRequest, "current password is required", "invalid_body")
	}

	requesterID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	requester, err := h.repo.FindUserByID(c.UserContext(), requesterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusUnauthorized, "invalid current password", "invalid_current_password")
	}
	if err != nil {
		slog.Error("admin find requester failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	if err := h.verifyPassword(requester.PasswordHash, req.CurrentPassword); err != nil {
		return adminError(c, fiber.StatusUnauthorized, "invalid current password", "invalid_current_password")
	}

	target, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	role := auth.UserRole(newRole)
	if target.Role == role {
		return adminError(c, fiber.StatusBadRequest, "user already has role", "same_role")
	}

	if target.Role == auth.Admin && role != auth.Admin {
		adminCount, err := h.repo.CountAdmins(c.UserContext())
		if err != nil {
			slog.Error("admin count admins failed", slog.String("err", err.Error()))
			return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		if adminCount == 1 {
			return adminError(c, fiber.StatusBadRequest, "cannot demote the last admin", "last_admin_protected")
		}
	}

	if target.ID == requesterID && role != auth.Admin {
		return adminError(c, fiber.StatusBadRequest, "cannot demote self", "cannot_demote_self")
	}

	oldRole := target.Role
	if err := h.repo.UpdateUserRole(c.UserContext(), targetID, role); err != nil {
		slog.Error("admin update user role failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if _, err := h.repo.RevokeAllRefreshByUser(c.UserContext(), targetID, auth.AdminReset); err != nil {
		slog.Warn("admin revoke refresh tokens failed",
			slog.String("user_id", targetID.String()),
			slog.String("err", err.Error()),
		)
	}

	h.logAudit(c, "admin_user_role_changed", requesterID, targetID, auditMeta(map[string]any{
		"old_role": string(oldRole),
		"new_role": string(role),
	}))

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": fresh})
}

// ResetUserPassword handles POST /api/v1/admin/users/:id/reset-password.
func (h *Handler) ResetUserPassword(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	var req resetUserPasswordRequest
	if err := c.BodyParser(&req); err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
	}

	strategy := strings.ToLower(strings.TrimSpace(req.PasswordStrategy))
	switch strategy {
	case "manual":
		if len(req.Password) < 8 {
			return adminError(c, fiber.StatusBadRequest, "password must be at least 8 characters", "weak_password")
		}
	case "generate":
		if req.Password != "" {
			return adminError(c, fiber.StatusBadRequest, "password must be empty when generating", "conflicting_password")
		}
	default:
		return adminError(c, fiber.StatusBadRequest, "invalid password strategy", "invalid_strategy")
	}

	if _, err := h.repo.FindUserByID(c.UserContext(), targetID); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	password := req.Password
	var generated *string
	if strategy == "generate" {
		plain, err := generatePassword(generatedLength)
		if err != nil {
			slog.Error("admin generate password failed", slog.String("err", err.Error()))
			return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		password = plain
		generated = &plain
	}

	cost := 0
	if h.cfg != nil {
		cost = h.cfg.JWT.BcryptCost
	}
	hash, err := auth.HashPassword(password, cost)
	if err != nil {
		slog.Error("admin hash password failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if err := h.repo.AdminResetUserPassword(c.UserContext(), targetID, hash); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin reset user password failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if _, err := h.repo.RevokeAllRefreshByUser(c.UserContext(), targetID, auth.AdminReset); err != nil {
		slog.Warn("admin revoke refresh tokens failed",
			slog.String("user_id", targetID.String()),
			slog.String("err", err.Error()),
		)
	}

	h.logAudit(c, "admin_user_password_reset", adminID, targetID, auditMeta(map[string]any{
		"password_strategy": strategy,
		"must_change":       true,
	}))

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	return c.Status(fiber.StatusOK).JSON(resetUserPasswordResponse{
		User:              fresh,
		GeneratedPassword: generated,
	})
}

// SuspendUser handles POST /api/v1/admin/users/:id/suspend.
func (h *Handler) SuspendUser(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	var req suspendUserRequest
	if body := strings.TrimSpace(string(c.Body())); body != "" {
		if err := c.BodyParser(&req); err != nil {
			return adminError(c, fiber.StatusBadRequest, "invalid request body", "invalid_body")
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 200 {
		return adminError(c, fiber.StatusBadRequest, "reason must be at most 200 characters", "invalid_body")
	}

	target, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if target.Role == auth.Admin {
		adminCount, err := h.repo.CountAdmins(c.UserContext())
		if err != nil {
			slog.Error("admin count admins failed", slog.String("err", err.Error()))
			return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		if adminCount == 1 {
			return adminError(c, fiber.StatusBadRequest, "cannot suspend the last admin", "last_admin_protected")
		}
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	if target.ID == adminID {
		return adminError(c, fiber.StatusBadRequest, "cannot suspend self", "cannot_suspend_self")
	}

	if target.Status == auth.Suspended {
		return adminError(c, fiber.StatusBadRequest, "user already suspended", "already_suspended")
	}

	if err := h.repo.SuspendUser(c.UserContext(), targetID); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin suspend user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if _, err := h.repo.RevokeAllRefreshByUser(c.UserContext(), targetID, auth.AdminReset); err != nil {
		slog.Warn("admin revoke refresh tokens failed",
			slog.String("user_id", targetID.String()),
			slog.String("err", err.Error()),
		)
	}

	h.logAudit(c, "admin_user_suspended", adminID, targetID, auditMeta(map[string]any{
		"previous_status": string(target.Status),
		"reason":          reason,
	}))

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": fresh})
}

// UnsuspendUser handles POST /api/v1/admin/users/:id/unsuspend.
func (h *Handler) UnsuspendUser(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	target, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if target.Status != auth.Suspended {
		return adminError(c, fiber.StatusBadRequest, "user is not suspended", "not_suspended")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if err := h.repo.UnsuspendUser(c.UserContext(), targetID); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin unsuspend user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	h.logAudit(c, "admin_user_unsuspended", adminID, targetID, auditMeta(map[string]any{
		"previous_status": string(auth.Suspended),
	}))

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": fresh})
}

// UnlockUser handles POST /api/v1/admin/users/:id/unlock.
func (h *Handler) UnlockUser(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	target, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if target.Status != auth.Locked {
		return adminError(c, fiber.StatusBadRequest, "user is not locked", "not_locked")
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if err := h.repo.UnlockUser(c.UserContext(), targetID); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin unlock user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	h.logAudit(c, "admin_user_unlocked", adminID, targetID, auditMeta(map[string]any{
		"previous_status": string(auth.Locked),
	}))

	fresh, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin refetch user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"user": fresh})
}

// DeleteUser handles DELETE /api/v1/admin/users/:id.
func (h *Handler) DeleteUser(c *fiber.Ctx) error {
	targetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return adminError(c, fiber.StatusBadRequest, "invalid id", "invalid_id")
	}

	target, err := h.repo.FindUserByID(c.UserContext(), targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	}
	if err != nil {
		slog.Error("admin find user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if target.Role == auth.Admin {
		adminCount, err := h.repo.CountAdmins(c.UserContext())
		if err != nil {
			slog.Error("admin count admins failed", slog.String("err", err.Error()))
			return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
		}
		if adminCount == 1 {
			return adminError(c, fiber.StatusBadRequest, "cannot suspend the last admin", "last_admin_protected")
		}
	}

	adminID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}
	if target.ID == adminID {
		return adminError(c, fiber.StatusBadRequest, "cannot delete self", "cannot_delete_self")
	}

	if err := h.repo.SuspendUser(c.UserContext(), targetID); errors.Is(err, gorm.ErrRecordNotFound) {
		return adminError(c, fiber.StatusNotFound, "user not found", "user_not_found")
	} else if err != nil {
		slog.Error("admin suspend user failed", slog.String("err", err.Error()))
		return adminError(c, fiber.StatusInternalServerError, "internal server error", "internal")
	}

	if _, err := h.repo.RevokeAllRefreshByUser(c.UserContext(), targetID, auth.AdminReset); err != nil {
		slog.Warn("admin revoke refresh tokens failed",
			slog.String("user_id", targetID.String()),
			slog.String("err", err.Error()),
		)
	}

	h.logAudit(c, "admin_user_suspended", adminID, targetID, auditMeta(map[string]any{
		"previous_status": string(target.Status),
	}))

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) logAudit(c *fiber.Ctx, action string, actorID, targetID uuid.UUID, meta datatypes.JSON) {
	actorRole := string(auth.Admin)
	targetType := "user"
	ip := c.IP()
	ua := string(c.Request().Header.UserAgent())
	entry := &auth.AuditLog{
		ActorID:    &actorID,
		ActorRole:  &actorRole,
		Action:     action,
		TargetType: &targetType,
		TargetID:   &targetID,
		Meta:       meta,
		IP:         strPtr(ip),
		UserAgent:  strPtr(ua),
		At:         time.Now(),
	}
	if err := h.repo.LogAudit(c.UserContext(), entry); err != nil {
		slog.Warn("admin audit log failed",
			slog.String("action", action),
			slog.String("target_id", targetID.String()),
			slog.String("err", err.Error()),
		)
	}
}

func validRole(role string) bool {
	switch auth.UserRole(role) {
	case auth.Admin, auth.Guru, auth.Siswa:
		return true
	default:
		return false
	}
}

func validStatus(status string) bool {
	switch auth.UserStatus(status) {
	case auth.Active, auth.Suspended, auth.Locked:
		return true
	default:
		return false
	}
}

func generatePassword(length int) (string, error) {
	var b strings.Builder
	b.Grow(length)
	max := big.NewInt(int64(len(passwordCharset)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordCharset[n.Int64()])
	}
	return b.String(), nil
}

func auditMeta(fields map[string]any) datatypes.JSON {
	if len(fields) == 0 {
		return nil
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return nil
	}
	return datatypes.JSON(b)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	v := s
	return &v
}

func adminError(c *fiber.Ctx, status int, message, code string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":      message,
		"code":       code,
		"request_id": middleware.RequestIDFromFiber(c),
	})
}
