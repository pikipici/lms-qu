package chat

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/kelas"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

func (r *Repo) FindKelas(ctx context.Context, kelasID uuid.UUID) (*kelas.Kelas, error) {
	var k kelas.Kelas
	if err := r.db.WithContext(ctx).Where("id = ? AND archived_at IS NULL", kelasID).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *Repo) IsSiswaEnrolled(ctx context.Context, kelasID, siswaID uuid.UUID) (bool, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&kelas.Enrollment{}).
		Where("kelas_id = ? AND siswa_id = ? AND status = ?", kelasID, siswaID, kelas.EnrollmentActive).
		Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *Repo) FindConversationByKelasSiswa(ctx context.Context, kelasID, siswaID uuid.UUID) (*Conversation, error) {
	var c Conversation
	if err := r.db.WithContext(ctx).
		Where("kelas_id = ? AND siswa_id = ? AND deleted_at IS NULL", kelasID, siswaID).
		First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repo) CreateConversation(ctx context.Context, c *Conversation) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *Repo) UpdateConversationGuru(ctx context.Context, conversationID, guruID uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&Conversation{}).
		Where("id = ?", conversationID).
		UpdateColumns(map[string]any{"guru_id": guruID, "version": gorm.Expr("version + 1")}).Error
}

func (r *Repo) GetConversation(ctx context.Context, id uuid.UUID) (*Conversation, error) {
	var c Conversation
	if err := r.db.WithContext(ctx).
		Select("chat_conversations.*, siswa.name AS siswa_name, guru.name AS guru_name, kelas.nama AS kelas_nama").
		Joins("LEFT JOIN users siswa ON siswa.id = chat_conversations.siswa_id").
		Joins("LEFT JOIN users guru ON guru.id = chat_conversations.guru_id").
		Joins("LEFT JOIN kelas ON kelas.id = chat_conversations.kelas_id").
		Where("chat_conversations.id = ? AND chat_conversations.deleted_at IS NULL", id).
		First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repo) ListMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]Message, error) {
	var rows []Message
	// Load newest N first, then reverse in Go for chat rendering old -> new.
	if err := r.db.WithContext(ctx).
		Select("chat_messages.*, users.name AS sender_name").
		Joins("LEFT JOIN users ON users.id = chat_messages.sender_id").
		Where("chat_messages.conversation_id = ? AND chat_messages.deleted_at IS NULL", conversationID).
		Order("chat_messages.created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

func (r *Repo) ListGuruConversations(ctx context.Context, kelasID, guruID uuid.UUID, status string, unread bool, limit, offset int) ([]Conversation, int64, error) {
	q := r.db.WithContext(ctx).Model(&Conversation{}).
		Where("chat_conversations.kelas_id = ? AND chat_conversations.guru_id = ? AND chat_conversations.deleted_at IS NULL", kelasID, guruID)
	if status != "" {
		q = q.Where("chat_conversations.status = ?", status)
	}
	if unread {
		q = q.Where("chat_conversations.guru_unread_count > 0")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Conversation
	if err := q.Select("chat_conversations.*, siswa.name AS siswa_name, guru.name AS guru_name, kelas.nama AS kelas_nama").
		Joins("LEFT JOIN users siswa ON siswa.id = chat_conversations.siswa_id").
		Joins("LEFT JOIN users guru ON guru.id = chat_conversations.guru_id").
		Joins("LEFT JOIN kelas ON kelas.id = chat_conversations.kelas_id").
		Order("chat_conversations.last_message_at DESC NULLS LAST, chat_conversations.created_at DESC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) ListAdminConversations(ctx context.Context, kelasID uuid.UUID, status string, unread bool, limit, offset int) ([]Conversation, int64, error) {
	q := r.db.WithContext(ctx).Model(&Conversation{}).
		Where("chat_conversations.deleted_at IS NULL")
	if kelasID != uuid.Nil {
		q = q.Where("chat_conversations.kelas_id = ?", kelasID)
	}
	if status != "" {
		q = q.Where("chat_conversations.status = ?", status)
	}
	if unread {
		q = q.Where("chat_conversations.admin_unread_count > 0")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []Conversation
	if err := q.Select("chat_conversations.*, siswa.name AS siswa_name, guru.name AS guru_name, kelas.nama AS kelas_nama").
		Joins("LEFT JOIN users siswa ON siswa.id = chat_conversations.siswa_id").
		Joins("LEFT JOIN users guru ON guru.id = chat_conversations.guru_id").
		Joins("LEFT JOIN kelas ON kelas.id = chat_conversations.kelas_id").
		Order("chat_conversations.last_message_at DESC NULLS LAST, chat_conversations.created_at DESC").
		Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *Repo) SendMessageTx(ctx context.Context, conversationID, senderID uuid.UUID, role SenderRole, body, preview string) (*Message, error) {
	var out *Message
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", conversationID).First(&conv).Error; err != nil {
			return err
		}
		msg := &Message{ConversationID: conversationID, SenderID: senderID, SenderRole: role, Body: body}
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"last_message_at":      gorm.Expr("now()"),
			"last_message_preview": preview,
			"version":              gorm.Expr("version + 1"),
		}
		switch role {
		case SenderSiswa:
			updates["status"] = StatusOpen
			updates["guru_unread_count"] = gorm.Expr("guru_unread_count + 1")
			updates["admin_unread_count"] = gorm.Expr("admin_unread_count + 1")
		case SenderGuru, SenderAdmin:
			updates["siswa_unread_count"] = gorm.Expr("siswa_unread_count + 1")
		default:
			return errors.New("invalid sender role")
		}
		if err := tx.Model(&Conversation{}).Where("id = ?", conversationID).UpdateColumns(updates).Error; err != nil {
			return err
		}
		out = msg
		return nil
	})
	return out, err
}

type UnreadSummary struct {
	KelasID uuid.UUID `json:"kelas_id"`
	Unread  int       `json:"unread"`
}

func (r *Repo) ListSiswaUnreadByKelas(ctx context.Context, siswaID uuid.UUID) ([]UnreadSummary, error) {
	var rows []UnreadSummary
	if err := r.db.WithContext(ctx).Model(&Conversation{}).
		Select("kelas_id, siswa_unread_count AS unread").
		Where("siswa_id = ? AND siswa_unread_count > 0 AND deleted_at IS NULL", siswaID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) ListGuruUnreadByKelas(ctx context.Context, guruID uuid.UUID) ([]UnreadSummary, error) {
	var rows []UnreadSummary
	if err := r.db.WithContext(ctx).Model(&Conversation{}).
		Select("kelas_id, SUM(guru_unread_count) AS unread").
		Where("guru_id = ? AND guru_unread_count > 0 AND deleted_at IS NULL", guruID).
		Group("kelas_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *Repo) MarkRead(ctx context.Context, conversationID uuid.UUID, role SenderRole) error {
	field := ""
	switch role {
	case SenderSiswa:
		field = "siswa_unread_count"
	case SenderGuru:
		field = "guru_unread_count"
	case SenderAdmin:
		field = "admin_unread_count"
	default:
		return errors.New("invalid reader role")
	}
	return r.db.WithContext(ctx).Model(&Conversation{}).
		Where("id = ?", conversationID).
		UpdateColumns(map[string]any{field: 0, "version": gorm.Expr("version + 1")}).Error
}

var ErrVersionConflict = errors.New("chat: version conflict")

func (r *Repo) SetStatus(ctx context.Context, conversationID uuid.UUID, status ConversationStatus, version int) (*Conversation, error) {
	res := r.db.WithContext(ctx).Model(&Conversation{}).
		Where("id = ? AND version = ? AND deleted_at IS NULL", conversationID, version).
		UpdateColumns(map[string]any{"status": status, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		var probe Conversation
		if err := r.db.WithContext(ctx).Select("id").Where("id = ? AND deleted_at IS NULL", conversationID).First(&probe).Error; err != nil {
			return nil, err
		}
		return nil, ErrVersionConflict
	}
	return r.GetConversation(ctx, conversationID)
}
