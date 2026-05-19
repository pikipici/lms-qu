// Repository for the auth domain: users, refresh tokens, login attempts, audit logs.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repo struct {
	db *gorm.DB
}

// NewRepo creates an auth repository backed by GORM.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// UserListFilter narrows the user list query. All fields are optional.
type UserListFilter struct {
	Role        string // "admin"|"guru"|"siswa"|""=any
	Status      string // "active"|"suspended"|"locked"|""=any
	SearchEmail string // ILIKE match on email; empty=no filter
	SearchName  string // ILIKE match on name; empty=no filter
}

// FindUserByEmail returns a user by email.
func (r *Repo) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindUserByID returns a user by ID.
func (r *Repo) FindUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser inserts a new user.
func (r *Repo) CreateUser(ctx context.Context, u *User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// ListUsers returns a page of users + total count matching filter, ordered by created_at DESC.
// limit must be >0; offset >=0.
func (r *Repo) ListUsers(ctx context.Context, f UserListFilter, limit, offset int) ([]User, int64, error) {
	query := r.db.WithContext(ctx).Model(&User{})
	if f.Role != "" {
		query = query.Where("role = ?", f.Role)
	}
	if f.Status != "" {
		query = query.Where("status = ?", f.Status)
	}

	searchEmail := strings.TrimSpace(f.SearchEmail)
	searchName := strings.TrimSpace(f.SearchName)
	switch {
	case searchEmail != "" && searchName != "":
		query = query.Where("(email ILIKE ? OR name ILIKE ?)", "%"+searchEmail+"%", "%"+searchName+"%")
	case searchEmail != "":
		query = query.Where("email ILIKE ?", "%"+searchEmail+"%")
	case searchName != "":
		query = query.Where("name ILIKE ?", "%"+searchName+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []User
	if err := query.Order("created_at DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// UpdateUserName updates only the name column. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) UpdateUserName(ctx context.Context, id uuid.UUID, name string) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"name":       name,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserRole sets the role column. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) UpdateUserRole(ctx context.Context, id uuid.UUID, role UserRole) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"role":       role,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// SuspendUser sets status='suspended'. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) SuspendUser(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"status":     Suspended,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// AdminResetUserPassword updates the password and forces a change on next login.
// Status is not changed. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) AdminResetUserPassword(ctx context.Context, id uuid.UUID, newHash string) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"password_hash":        newHash,
			"must_change_password": true,
			"failed_login_count":   0,
			"updated_at":           gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UnsuspendUser sets status='active'. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) UnsuspendUser(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"status":     Active,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UnlockUser sets status='active' and failed_login_count=0. Returns gorm.ErrRecordNotFound if no row.
func (r *Repo) UnlockUser(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", id).
		UpdateColumns(map[string]any{
			"status":             Active,
			"failed_login_count": 0,
			"updated_at":         gorm.Expr("now()"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateUserPassword updates a user's password and clears force-change state.
func (r *Repo) UpdateUserPassword(ctx context.Context, userID uuid.UUID, newHash string) error {
	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		UpdateColumns(map[string]any{
			"password_hash":        newHash,
			"must_change_password": false,
			"updated_at":           gorm.Expr("now()"),
		}).Error
}

// IncFailedLogin increments a user's failed login counter.
func (r *Repo) IncFailedLogin(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		UpdateColumns(map[string]any{
			"failed_login_count":   gorm.Expr("failed_login_count + 1"),
			"last_failed_login_at": gorm.Expr("now()"),
		}).Error
}

// ResetFailedLogin clears failed login state and records last login time.
func (r *Repo) ResetFailedLogin(ctx context.Context, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Model(&User{}).
		Where("id = ?", userID).
		UpdateColumns(map[string]any{
			"failed_login_count":   0,
			"last_failed_login_at": nil,
			"last_login_at":        gorm.Expr("now()"),
		}).Error
}

// LockUser locks a user and revokes all active refresh tokens.
func (r *Repo) LockUser(ctx context.Context, userID uuid.UUID, reason string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).
			Where("id = ?", userID).
			Update("status", Locked).Error; err != nil {
			return err
		}

		_, err := (&Repo{db: tx}).RevokeAllRefreshByUser(ctx, userID, UserLocked)
		return err
	})
}

// IssueRefresh inserts a refresh token session.
func (r *Repo) IssueRefresh(ctx context.Context, t *RefreshToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// FindRefreshByJTI returns a refresh token by JTI.
func (r *Repo) FindRefreshByJTI(ctx context.Context, jti uuid.UUID) (*RefreshToken, error) {
	var token RefreshToken
	if err := r.db.WithContext(ctx).Where("jti = ?", jti).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// RotateRefresh revokes an old refresh token and inserts its replacement.
func (r *Repo) RotateRefresh(ctx context.Context, oldJTI uuid.UUID, newToken *RefreshToken) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		db := tx.WithContext(ctx)
		res := db.Model(&RefreshToken{}).
			Where("jti = ? AND revoked_at IS NULL", oldJTI).
			UpdateColumns(map[string]any{
				"revoked_at":      gorm.Expr("now()"),
				"revoked_reason":  string(Rotate),
				"replaced_by_jti": newToken.JTI,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var old RefreshToken
			if err := db.Select("revoked_at").Where("jti = ?", oldJTI).First(&old).Error; err != nil {
				return err
			}
			if old.RevokedAt != nil {
				return errors.New("refresh: token already revoked")
			}
			return gorm.ErrRecordNotFound
		}

		return db.Create(newToken).Error
	})
}

// RevokeRefresh revokes one active refresh token.
func (r *Repo) RevokeRefresh(ctx context.Context, jti uuid.UUID, reason RevokedReason) error {
	return r.db.WithContext(ctx).
		Model(&RefreshToken{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		UpdateColumns(map[string]any{
			"revoked_at":     gorm.Expr("now()"),
			"revoked_reason": string(reason),
		}).Error
}

// RevokeAllRefreshByUser revokes all active refresh tokens for a user.
func (r *Repo) RevokeAllRefreshByUser(ctx context.Context, userID uuid.UUID, reason RevokedReason) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		UpdateColumns(map[string]any{
			"revoked_at":     gorm.Expr("now()"),
			"revoked_reason": string(reason),
		})
	return res.RowsAffected, res.Error
}

// RevokeRefreshChain revokes all refresh tokens owned by a reused token's user.
func (r *Repo) RevokeRefreshChain(ctx context.Context, jti uuid.UUID) error {
	var token RefreshToken
	if err := r.db.WithContext(ctx).Select("user_id").Where("jti = ?", jti).First(&token).Error; err != nil {
		return err
	}

	_, err := r.RevokeAllRefreshByUser(ctx, token.UserID, ReuseDetected)
	return err
}

// ListUserSessions returns active, unexpired refresh sessions for a user.
func (r *Repo) ListUserSessions(ctx context.Context, userID uuid.UUID) ([]RefreshToken, error) {
	var tokens []RefreshToken
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > now()", userID).
		Order("issued_at DESC").
		Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// LogLoginAttempt inserts a login attempt event.
func (r *Repo) LogLoginAttempt(ctx context.Context, attempt *LoginAttempt) error {
	return r.db.WithContext(ctx).Create(attempt).Error
}

// CountRecentFailedAttempts counts recent failed attempts by email or IP.
func (r *Repo) CountRecentFailedAttempts(ctx context.Context, email string, ip *string, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&LoginAttempt{}).
		Where("(email = ? OR ip = ?) AND success = ? AND at >= ?", email, ip, false, since).
		Count(&count).Error
	return count, err
}

// LogAudit inserts an audit log entry.
func (r *Repo) LogAudit(ctx context.Context, entry *AuditLog) error {
	return r.db.WithContext(ctx).Create(entry).Error
}

// CountAdmins returns the number of admin users in the database.
// Used by cmd/seed-admin to enforce single-bootstrap policy.
func (r *Repo) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&User{}).Where("role = ?", Admin).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}
