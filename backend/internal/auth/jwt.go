package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/config"
)

const jwtIssuer = "lms-api"

// ErrInvalidSigningMethod is returned when a token does not use HS256.
var ErrInvalidSigningMethod = errors.New("jwt: invalid signing method")

// AccessClaims are the JWT claims carried by access tokens.
type AccessClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   UserRole  `json:"role"`
	jwt.RegisteredClaims
}

// RefreshClaims are the JWT claims carried by refresh tokens.
type RefreshClaims struct {
	UserID uuid.UUID `json:"user_id"`
	jwt.RegisteredClaims
}

// IssueAccess signs an HS256 access token for the given user.
func IssueAccess(cfg config.JWTConfig, userID uuid.UUID, role UserRole) (token string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	expiresAt = now.Add(time.Duration(cfg.AccessTTLMin) * time.Minute)

	claims := AccessClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, &claims).SignedString([]byte(cfg.SecretKey))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("jwt: issue access token: %w", err)
	}
	return token, expiresAt, nil
}

// IssueRefresh signs an HS256 refresh token with a new JTI.
func IssueRefresh(cfg config.JWTConfig, userID uuid.UUID) (jti string, token string, expiresAt time.Time, err error) {
	now := time.Now().UTC()
	jti = uuid.NewString()
	expiresAt = now.Add(time.Duration(cfg.RefreshTTLDays) * 24 * time.Hour)

	claims := RefreshClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        jti,
		},
	}

	token, err = jwt.NewWithClaims(jwt.SigningMethodHS256, &claims).SignedString([]byte(cfg.SecretKey))
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("jwt: issue refresh token: %w", err)
	}
	return jti, token, expiresAt, nil
}

// VerifyAccess parses and validates an access token.
func VerifyAccess(cfg config.JWTConfig, raw string) (*AccessClaims, error) {
	claims := &AccessClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, jwtKeyFunc(cfg.SecretKey))
	if err != nil {
		return nil, fmt.Errorf("jwt: verify access token: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("jwt: access token invalid")
	}
	return claims, nil
}

// VerifyRefresh parses and validates a refresh token.
func VerifyRefresh(cfg config.JWTConfig, raw string) (jti string, userID uuid.UUID, err error) {
	claims := &RefreshClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, jwtKeyFunc(cfg.SecretKey))
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("jwt: verify refresh token: %w", err)
	}
	if !token.Valid {
		return "", uuid.Nil, errors.New("jwt: refresh token invalid")
	}
	if claims.ID == "" {
		return "", uuid.Nil, errors.New("jwt: refresh token missing jti")
	}
	return claims.ID, claims.UserID, nil
}

func jwtKeyFunc(secret string) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		method, ok := token.Method.(*jwt.SigningMethodHMAC)
		if !ok || method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSigningMethod, token.Header["alg"])
		}
		return []byte(secret), nil
	}
}
