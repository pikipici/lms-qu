package chat

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/auth"
	"gorm.io/gorm"
)

const (
	messageMinLen = 1
	messageMaxLen = 4000
	previewMaxLen = 160
	defaultLimit  = 80
	maxLimit      = 100
)

var (
	ErrForbidden     = errors.New("chat: forbidden")
	ErrInvalidBody   = errors.New("chat: invalid body")
	ErrInvalidStatus = errors.New("chat: invalid status")
)

type Service struct{ repo *Repo }

func NewService(repo *Repo) *Service { return &Service{repo: repo} }

type ConversationWithMessages struct {
	Conversation *Conversation `json:"conversation"`
	Messages     []Message     `json:"messages"`
}

type ListResult struct {
	Data  []Conversation `json:"data"`
	Total int64          `json:"total"`
}

func (s *Service) GetSiswaConversation(ctx context.Context, kelasID, siswaID uuid.UUID, limit int) (*ConversationWithMessages, error) {
	conv, err := s.getOrCreateSiswaConversation(ctx, kelasID, siswaID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListMessages(ctx, conv.ID, clampLimit(limit, defaultLimit, maxLimit))
	if err != nil {
		return nil, err
	}
	return &ConversationWithMessages{Conversation: conv, Messages: msgs}, nil
}

func (s *Service) SendSiswaMessage(ctx context.Context, kelasID, siswaID uuid.UUID, body string) (*Message, error) {
	conv, err := s.getOrCreateSiswaConversation(ctx, kelasID, siswaID)
	if err != nil {
		return nil, err
	}
	body, err = normalizeBody(body)
	if err != nil {
		return nil, err
	}
	return s.repo.SendMessageTx(ctx, conv.ID, siswaID, SenderSiswa, body, makePreview(body))
}

func (s *Service) MarkSiswaRead(ctx context.Context, kelasID, siswaID uuid.UUID) error {
	conv, err := s.repo.FindConversationByKelasSiswa(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, conv.ID, SenderSiswa)
}

func (s *Service) ListGuruConversations(ctx context.Context, kelasID, guruID uuid.UUID, status string, unread bool, limit, offset int) (*ListResult, error) {
	k, err := s.repo.FindKelas(ctx, kelasID)
	if err != nil {
		return nil, err
	}
	if k.GuruID != guruID {
		return nil, ErrForbidden
	}
	if status != "" && status != string(StatusOpen) && status != string(StatusClosed) {
		return nil, ErrInvalidStatus
	}
	rows, total, err := s.repo.ListGuruConversations(ctx, kelasID, guruID, status, unread, clampLimit(limit, 20, 100), max(offset, 0))
	if err != nil {
		return nil, err
	}
	return &ListResult{Data: rows, Total: total}, nil
}

func (s *Service) ListAdminConversations(ctx context.Context, kelasID uuid.UUID, status string, unread bool, limit, offset int) (*ListResult, error) {
	if status != "" && status != string(StatusOpen) && status != string(StatusClosed) {
		return nil, ErrInvalidStatus
	}
	rows, total, err := s.repo.ListAdminConversations(ctx, kelasID, status, unread, clampLimit(limit, 20, 100), max(offset, 0))
	if err != nil {
		return nil, err
	}
	return &ListResult{Data: rows, Total: total}, nil
}

func (s *Service) GetAdminMessages(ctx context.Context, conversationID uuid.UUID, limit int) (*ConversationWithMessages, error) {
	conv, err := s.authorizeAdminClassConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListMessages(ctx, conversationID, clampLimit(limit, defaultLimit, maxLimit))
	if err != nil {
		return nil, err
	}
	return &ConversationWithMessages{Conversation: conv, Messages: msgs}, nil
}

func (s *Service) SendAdminMessage(ctx context.Context, adminID, conversationID uuid.UUID, body string) (*Message, error) {
	if _, err := s.authorizeAdminClassConversation(ctx, conversationID); err != nil {
		return nil, err
	}
	body, err := normalizeBody(body)
	if err != nil {
		return nil, err
	}
	return s.repo.SendMessageTx(ctx, conversationID, adminID, SenderAdmin, body, makePreview(body))
}

func (s *Service) MarkAdminRead(ctx context.Context, conversationID uuid.UUID) error {
	if _, err := s.authorizeAdminClassConversation(ctx, conversationID); err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, conversationID, SenderAdmin)
}

func (s *Service) SetAdminStatus(ctx context.Context, conversationID uuid.UUID, status ConversationStatus, version int) (*Conversation, error) {
	if status != StatusOpen && status != StatusClosed {
		return nil, ErrInvalidStatus
	}
	if _, err := s.authorizeAdminClassConversation(ctx, conversationID); err != nil {
		return nil, err
	}
	return s.repo.SetStatus(ctx, conversationID, status, version)
}

func (s *Service) ListSiswaUnreadByKelas(ctx context.Context, siswaID uuid.UUID) ([]UnreadSummary, error) {
	return s.repo.ListSiswaUnreadByKelas(ctx, siswaID)
}

func (s *Service) ListGuruUnreadByKelas(ctx context.Context, guruID uuid.UUID) ([]UnreadSummary, error) {
	return s.repo.ListGuruUnreadByKelas(ctx, guruID)
}

func (s *Service) GetGuruMessages(ctx context.Context, kelasID, guruID, conversationID uuid.UUID, limit int) (*ConversationWithMessages, error) {
	conv, err := s.authorizeGuruConversation(ctx, kelasID, guruID, conversationID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListMessages(ctx, conversationID, clampLimit(limit, defaultLimit, maxLimit))
	if err != nil {
		return nil, err
	}
	return &ConversationWithMessages{Conversation: conv, Messages: msgs}, nil
}

func (s *Service) SendGuruMessage(ctx context.Context, kelasID, guruID, conversationID uuid.UUID, body string) (*Message, error) {
	if _, err := s.authorizeGuruConversation(ctx, kelasID, guruID, conversationID); err != nil {
		return nil, err
	}
	body, err := normalizeBody(body)
	if err != nil {
		return nil, err
	}
	return s.repo.SendMessageTx(ctx, conversationID, guruID, SenderGuru, body, makePreview(body))
}

func (s *Service) MarkGuruRead(ctx context.Context, kelasID, guruID, conversationID uuid.UUID) error {
	if _, err := s.authorizeGuruConversation(ctx, kelasID, guruID, conversationID); err != nil {
		return err
	}
	return s.repo.MarkRead(ctx, conversationID, SenderGuru)
}

func (s *Service) SetGuruStatus(ctx context.Context, kelasID, guruID, conversationID uuid.UUID, status ConversationStatus, version int) (*Conversation, error) {
	if status != StatusOpen && status != StatusClosed {
		return nil, ErrInvalidStatus
	}
	if _, err := s.authorizeGuruConversation(ctx, kelasID, guruID, conversationID); err != nil {
		return nil, err
	}
	return s.repo.SetStatus(ctx, conversationID, status, version)
}

func (s *Service) getOrCreateSiswaConversation(ctx context.Context, kelasID, siswaID uuid.UUID) (*Conversation, error) {
	k, err := s.repo.FindKelas(ctx, kelasID)
	if err != nil {
		return nil, err
	}
	ok, err := s.repo.IsSiswaEnrolled(ctx, kelasID, siswaID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}
	conv, err := s.repo.FindConversationByKelasSiswa(ctx, kelasID, siswaID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		conv = &Conversation{Scope: ScopeKelas, KelasID: kelasID, SekolahID: k.SekolahID, SiswaID: siswaID, GuruID: k.GuruID, Status: StatusOpen}
		if createErr := s.repo.CreateConversation(ctx, conv); createErr != nil {
			return nil, createErr
		}
		return s.repo.GetConversation(ctx, conv.ID)
	}
	if err != nil {
		return nil, err
	}
	if conv.GuruID != k.GuruID || !sameUUIDPtr(conv.SekolahID, k.SekolahID) {
		if err := s.repo.UpdateConversationClassSnapshot(ctx, conv.ID, k.GuruID, k.SekolahID); err != nil {
			return nil, err
		}
	}
	return s.repo.GetConversation(ctx, conv.ID)
}

func (s *Service) authorizeGuruConversation(ctx context.Context, kelasID, guruID, conversationID uuid.UUID) (*Conversation, error) {
	k, err := s.repo.FindKelas(ctx, kelasID)
	if err != nil {
		return nil, err
	}
	if k.GuruID != guruID {
		return nil, ErrForbidden
	}
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.Scope != ScopeKelas || conv.KelasID != kelasID || conv.GuruID != guruID {
		return nil, ErrForbidden
	}
	return conv, nil
}

func (s *Service) authorizeAdminClassConversation(ctx context.Context, conversationID uuid.UUID) (*Conversation, error) {
	conv, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.Scope != ScopeKelas {
		return nil, ErrForbidden
	}
	return conv, nil
}

func normalizeBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	n := utf8.RuneCountInString(body)
	if n < messageMinLen || n > messageMaxLen {
		return "", ErrInvalidBody
	}
	return body, nil
}

func makePreview(body string) string {
	r := []rune(strings.TrimSpace(body))
	if len(r) <= previewMaxLen {
		return string(r)
	}
	return string(r[:previewMaxLen])
}

func clampLimit(v, def, maxV int) int {
	if v <= 0 {
		return def
	}
	if v > maxV {
		return maxV
	}
	return v
}

func sameUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func senderRoleFromAuthRole(role string) SenderRole {
	switch auth.UserRole(role) {
	case auth.Siswa:
		return SenderSiswa
	case auth.Guru:
		return SenderGuru
	case auth.Admin:
		return SenderAdmin
	default:
		return ""
	}
}
