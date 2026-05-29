package chat

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/middleware"
	"gorm.io/gorm"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

type sendMessageReq struct {
	Body string `json:"body"`
}

type setStatusReq struct {
	Status  ConversationStatus `json:"status"`
	Version int                `json:"version"`
}

func (h *Handler) GetSiswaChat(c *fiber.Ctx) error {
	kelasID, err := parseUUIDParam(c, "kelas_id")
	if err != nil {
		return badRequest(c, "invalid_kelas_id", "invalid kelas id")
	}
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	res, err := h.svc.GetSiswaConversation(c.UserContext(), kelasID, userID, queryInt(c, "limit", defaultLimit))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": res})
}

func (h *Handler) SendSiswaMessage(c *fiber.Ctx) error {
	kelasID, err := parseUUIDParam(c, "kelas_id")
	if err != nil {
		return badRequest(c, "invalid_kelas_id", "invalid kelas id")
	}
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	msg, err := h.svc.SendSiswaMessage(c.UserContext(), kelasID, userID, req.Body)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": msg})
}

func (h *Handler) GetSiswaAdminChat(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	res, err := h.svc.GetSiswaAdminConversation(c.UserContext(), userID, queryInt(c, "limit", defaultLimit))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": res})
}

func (h *Handler) SendSiswaAdminMessage(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	msg, err := h.svc.SendSiswaAdminMessage(c.UserContext(), userID, req.Body)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": msg})
}

func (h *Handler) MarkSiswaAdminRead(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	if err := h.svc.MarkSiswaAdminRead(c.UserContext(), userID); err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) ListSiswaUnread(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	rows, err := h.svc.ListSiswaUnreadByKelas(c.UserContext(), userID)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": rows})
}

func (h *Handler) ListGuruUnread(c *fiber.Ctx) error {
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	rows, err := h.svc.ListGuruUnreadByKelas(c.UserContext(), userID)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": rows})
}

func (h *Handler) MarkSiswaRead(c *fiber.Ctx) error {
	kelasID, err := parseUUIDParam(c, "kelas_id")
	if err != nil {
		return badRequest(c, "invalid_kelas_id", "invalid kelas id")
	}
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	if err := h.svc.MarkSiswaRead(c.UserContext(), kelasID, userID); err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) ListGuruConversations(c *fiber.Ctx) error {
	kelasID, guruID, ok := h.kelasGuruParams(c)
	if !ok {
		return nil
	}
	res, err := h.svc.ListGuruConversations(c.UserContext(), kelasID, guruID, strings.TrimSpace(c.Query("status")), c.Query("unread") == "true", queryInt(c, "limit", 20), queryInt(c, "offset", 0))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(res)
}

func (h *Handler) GetGuruMessages(c *fiber.Ctx) error {
	kelasID, guruID, ok := h.kelasGuruParams(c)
	if !ok {
		return nil
	}
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	res, err := h.svc.GetGuruMessages(c.UserContext(), kelasID, guruID, conversationID, queryInt(c, "limit", defaultLimit))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": res})
}

func (h *Handler) SendGuruMessage(c *fiber.Ctx) error {
	kelasID, guruID, ok := h.kelasGuruParams(c)
	if !ok {
		return nil
	}
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	msg, err := h.svc.SendGuruMessage(c.UserContext(), kelasID, guruID, conversationID, req.Body)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": msg})
}

func (h *Handler) MarkGuruRead(c *fiber.Ctx) error {
	kelasID, guruID, ok := h.kelasGuruParams(c)
	if !ok {
		return nil
	}
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	if err := h.svc.MarkGuruRead(c.UserContext(), kelasID, guruID, conversationID); err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) SetGuruStatus(c *fiber.Ctx) error {
	kelasID, guruID, ok := h.kelasGuruParams(c)
	if !ok {
		return nil
	}
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	var req setStatusReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	conv, err := h.svc.SetGuruStatus(c.UserContext(), kelasID, guruID, conversationID, req.Status, req.Version)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": conv})
}

func (h *Handler) ListAdminConversations(c *fiber.Ctx) error {
	kelasID := uuid.Nil
	if raw := strings.TrimSpace(c.Query("kelas_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return badRequest(c, "invalid_kelas_id", "invalid kelas id")
		}
		kelasID = id
	}
	scope := ScopeKelas
	if strings.TrimSpace(c.Query("scope")) == string(ScopeAdmin) {
		scope = ScopeAdmin
	}
	res, err := h.svc.ListAdminConversations(c.UserContext(), scope, kelasID, strings.TrimSpace(c.Query("status")), c.Query("unread") == "true", queryInt(c, "limit", 20), queryInt(c, "offset", 0))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(res)
}

func (h *Handler) GetAdminMessages(c *fiber.Ctx) error {
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	res, err := h.svc.GetAdminMessages(c.UserContext(), conversationID, queryInt(c, "limit", defaultLimit))
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": res})
}

func (h *Handler) SendAdminMessage(c *fiber.Ctx) error {
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		return unauthorized(c)
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	msg, err := h.svc.SendAdminMessage(c.UserContext(), userID, conversationID, req.Body)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": msg})
}

func (h *Handler) MarkAdminRead(c *fiber.Ctx) error {
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	if err := h.svc.MarkAdminRead(c.UserContext(), conversationID); err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *Handler) SetAdminStatus(c *fiber.Ctx) error {
	conversationID, err := parseUUIDParam(c, "conversation_id")
	if err != nil {
		return badRequest(c, "invalid_conversation_id", "invalid conversation id")
	}
	var req setStatusReq
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "invalid_json", "invalid json")
	}
	conv, err := h.svc.SetAdminStatus(c.UserContext(), conversationID, req.Status, req.Version)
	if err != nil {
		return h.mapErr(c, err)
	}
	return c.JSON(fiber.Map{"data": conv})
}

func (h *Handler) kelasGuruParams(c *fiber.Ctx) (uuid.UUID, uuid.UUID, bool) {
	kelasID, err := parseUUIDParam(c, "kelas_id")
	if err != nil {
		_ = badRequest(c, "invalid_kelas_id", "invalid kelas id")
		return uuid.Nil, uuid.Nil, false
	}
	userID, err := middleware.UserIDFromCtx(c)
	if err != nil {
		_ = unauthorized(c)
		return uuid.Nil, uuid.Nil, false
	}
	return kelasID, userID, true
}

func (h *Handler) mapErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(errBody(c, "forbidden", "forbidden"))
	case errors.Is(err, ErrInvalidBody):
		return badRequest(c, "invalid_body", "message body must be 1-4000 characters")
	case errors.Is(err, ErrInvalidStatus):
		return badRequest(c, "invalid_status", "invalid status")
	case errors.Is(err, ErrVersionConflict):
		return c.Status(fiber.StatusConflict).JSON(errBody(c, "version_conflict", "version conflict"))
	case errors.Is(err, gorm.ErrRecordNotFound):
		return c.Status(fiber.StatusNotFound).JSON(errBody(c, "not_found", "not found"))
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(errBody(c, "internal_error", "internal error"))
	}
}

func parseUUIDParam(c *fiber.Ctx, name string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(c.Params(name)))
}

func queryInt(c *fiber.Ctx, name string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(c.Query(name)))
	if err != nil {
		return def
	}
	return v
}

func badRequest(c *fiber.Ctx, code, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(errBody(c, code, msg))
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(errBody(c, "unauthorized", "unauthorized"))
}

func errBody(c *fiber.Ctx, code, msg string) fiber.Map {
	return fiber.Map{"error": msg, "code": code, "request_id": middleware.RequestIDFromFiber(c)}
}
