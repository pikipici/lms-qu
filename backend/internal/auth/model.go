// Package auth holds authentication, session, and audit-trail models for LMS.
package auth

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type UserRole string

const (
	Admin UserRole = "admin"
	Guru  UserRole = "guru"
	Siswa UserRole = "siswa"
)

type UserStatus string

const (
	Active    UserStatus = "active"
	Suspended UserStatus = "suspended"
	Locked    UserStatus = "locked"
)

type RevokedReason string

const (
	Logout          RevokedReason = "logout"
	Rotate          RevokedReason = "rotate"
	PasswordChanged RevokedReason = "password_changed"
	AdminReset      RevokedReason = "admin_reset"
	UserLocked      RevokedReason = "user_locked"
	UserSuspended   RevokedReason = "user_suspended"
	ReuseDetected   RevokedReason = "reuse_detected"
)

type User struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Name               string     `gorm:"not null" json:"name"`
	Email              string     `gorm:"type:citext;uniqueIndex;not null" json:"email"`
	PasswordHash       string     `gorm:"not null" json:"-"`
	Role               UserRole   `gorm:"type:user_role;not null" json:"role"`
	Status             UserStatus `gorm:"type:user_status;not null;default:active" json:"status"`
	MustChangePassword bool       `gorm:"not null;default:true" json:"must_change_password"`
	FailedLoginCount   int        `gorm:"not null;default:0" json:"-"`
	LastFailedLoginAt  *time.Time `json:"-"`
	CreatedByID        *uuid.UUID `gorm:"type:uuid" json:"created_by_id,omitempty"`
	LastLoginAt         *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

type RefreshToken struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	JTI           uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null" json:"jti"`
	UserID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_refresh_tokens_user_id_revoked_at,priority:1" json:"user_id"`
	IssuedAt      time.Time  `gorm:"not null;default:now()" json:"issued_at"`
	ExpiresAt     time.Time  `gorm:"not null;index" json:"expires_at"`
	RevokedAt     *time.Time `gorm:"index:idx_refresh_tokens_user_id_revoked_at,priority:2" json:"revoked_at,omitempty"`
	RevokedReason *string    `json:"revoked_reason,omitempty"`
	IP            *string    `gorm:"type:inet" json:"ip,omitempty"`
	UserAgent     *string    `json:"user_agent,omitempty"`
	ReplacedByJTI *uuid.UUID `gorm:"type:uuid" json:"replaced_by_jti,omitempty"`
}

func (RefreshToken) TableName() string {
	return "refresh_tokens"
}

type LoginAttempt struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	Email     string     `gorm:"type:citext;not null;index:idx_login_attempts_email_at,priority:1" json:"email"`
	IP        *string    `gorm:"type:inet;index:idx_login_attempts_ip_at,priority:1" json:"ip,omitempty"`
	UserAgent *string    `json:"user_agent,omitempty"`
	Success   bool       `gorm:"not null" json:"success"`
	Reason    *string    `json:"reason,omitempty"`
	At        time.Time  `gorm:"not null;default:now();index:idx_login_attempts_email_at,priority:2,sort:desc;index:idx_login_attempts_ip_at,priority:2,sort:desc" json:"at"`
}

func (LoginAttempt) TableName() string {
	return "login_attempts"
}

type AuditLog struct {
	ID            uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	ActorID       *uuid.UUID     `gorm:"type:uuid;index:idx_audit_logs_actor_id_at,priority:1" json:"actor_id,omitempty"`
	ActorRole     *string        `json:"actor_role,omitempty"`
	Action        string         `gorm:"not null" json:"action"`
	TargetType    *string        `json:"target_type,omitempty"`
	TargetID      *uuid.UUID     `gorm:"type:uuid" json:"target_id,omitempty"`
	TargetKelasID *uuid.UUID     `gorm:"type:uuid;index:idx_audit_logs_target_kelas_id_at,priority:1" json:"target_kelas_id,omitempty"`
	Meta          datatypes.JSON `gorm:"type:jsonb" json:"meta,omitempty"`
	IP            *string        `gorm:"type:inet" json:"ip,omitempty"`
	UserAgent     *string        `json:"user_agent,omitempty"`
	At            time.Time      `gorm:"not null;default:now();index:idx_audit_logs_actor_id_at,priority:2,sort:desc;index:idx_audit_logs_target_kelas_id_at,priority:2,sort:desc" json:"at"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
