package chat

import (
	"time"

	"github.com/google/uuid"
)

type ConversationStatus string

const (
	StatusOpen   ConversationStatus = "open"
	StatusClosed ConversationStatus = "closed"
)

type SenderRole string

const (
	SenderSiswa SenderRole = "siswa"
	SenderGuru  SenderRole = "guru"
	SenderAdmin SenderRole = "admin"
)

type Conversation struct {
	ID                 uuid.UUID          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	KelasID            uuid.UUID          `gorm:"type:uuid;not null;index" json:"kelas_id"`
	SiswaID            uuid.UUID          `gorm:"type:uuid;not null;index" json:"siswa_id"`
	GuruID             uuid.UUID          `gorm:"type:uuid;not null;index" json:"guru_id"`
	Status             ConversationStatus `gorm:"not null;default:open" json:"status"`
	LastMessageAt      *time.Time         `json:"last_message_at,omitempty"`
	LastMessagePreview string             `gorm:"not null;default:''" json:"last_message_preview"`
	SiswaUnreadCount   int                `gorm:"not null;default:0" json:"siswa_unread_count"`
	GuruUnreadCount    int                `gorm:"not null;default:0" json:"guru_unread_count"`
	AdminUnreadCount   int                `gorm:"not null;default:0" json:"admin_unread_count"`
	Version            int                `gorm:"not null;default:1" json:"version"`
	CreatedAt          time.Time          `json:"created_at"`
	UpdatedAt          time.Time          `json:"updated_at"`
	DeletedAt          *time.Time         `json:"deleted_at,omitempty"`
	SiswaName          string             `gorm:"->" json:"siswa_name,omitempty"`
	GuruName           string             `gorm:"->" json:"guru_name,omitempty"`
	KelasNama          string             `gorm:"->" json:"kelas_nama,omitempty"`
}

func (Conversation) TableName() string { return "chat_conversations" }

type Message struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ConversationID uuid.UUID  `gorm:"type:uuid;not null;index" json:"conversation_id"`
	SenderID       uuid.UUID  `gorm:"type:uuid;not null;index" json:"sender_id"`
	SenderRole     SenderRole `gorm:"not null" json:"sender_role"`
	Body           string     `gorm:"not null" json:"body"`
	CreatedAt      time.Time  `json:"created_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	SenderName     string     `gorm:"->" json:"sender_name,omitempty"`
}

func (Message) TableName() string { return "chat_messages" }
