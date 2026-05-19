package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/pikip/lms/backend/internal/config"
)

func TestIssueAndVerifyAccess_RoundTrip(t *testing.T) {
	cfg := testJWTConfig()
	userID := uuid.New()
	now := time.Now().UTC()

	token, expiresAt, err := IssueAccess(cfg, userID, Guru)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}
	claims, err := VerifyAccess(cfg, token)
	if err != nil {
		t.Fatalf("VerifyAccess() error = %v", err)
	}
	if claims.UserID != userID {
		t.Fatalf("claims.UserID = %s, want %s", claims.UserID, userID)
	}
	if claims.Role != Guru {
		t.Fatalf("claims.Role = %s, want %s", claims.Role, Guru)
	}
	if !expiresAt.After(now) {
		t.Fatalf("expiresAt = %s, want after %s", expiresAt, now)
	}
}

func TestVerifyAccess_WrongSecret(t *testing.T) {
	cfg := testJWTConfig()
	userID := uuid.New()

	token, _, err := IssueAccess(cfg, userID, Guru)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}
	cfg.SecretKey = "different-test-secret-min-32-chars"

	_, err = VerifyAccess(cfg, token)
	if !errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		t.Fatalf("VerifyAccess() error = %v, want %v", err, jwt.ErrTokenSignatureInvalid)
	}
}

func TestIssueAndVerifyRefresh_RoundTrip(t *testing.T) {
	cfg := testJWTConfig()
	userID := uuid.New()

	jti, token, _, err := IssueRefresh(cfg, userID)
	if err != nil {
		t.Fatalf("IssueRefresh() error = %v", err)
	}
	if _, err := uuid.Parse(jti); err != nil {
		t.Fatalf("uuid.Parse(jti) error = %v", err)
	}
	gotJTI, gotUserID, err := VerifyRefresh(cfg, token)
	if err != nil {
		t.Fatalf("VerifyRefresh() error = %v", err)
	}
	if gotJTI != jti {
		t.Fatalf("VerifyRefresh() jti = %s, want %s", gotJTI, jti)
	}
	if gotUserID != userID {
		t.Fatalf("VerifyRefresh() userID = %s, want %s", gotUserID, userID)
	}
}

func TestVerifyAccess_ExpiredToken(t *testing.T) {
	cfg := testJWTConfig()
	cfg.AccessTTLMin = -1
	userID := uuid.New()

	token, _, err := IssueAccess(cfg, userID, Guru)
	if err != nil {
		t.Fatalf("IssueAccess() error = %v", err)
	}
	_, err = VerifyAccess(cfg, token)
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatalf("VerifyAccess() error = %v, want %v", err, jwt.ErrTokenExpired)
	}
}

func TestVerifyAccess_InvalidSigningMethod(t *testing.T) {
	cfg := testJWTConfig()
	userID := uuid.New()
	now := time.Now().UTC()

	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, &AccessClaims{
		UserID: userID,
		Role:   Guru,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    jwtIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
		},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	_, err = VerifyAccess(cfg, token)
	if !errors.Is(err, ErrInvalidSigningMethod) {
		t.Fatalf("VerifyAccess() error = %v, want %v", err, ErrInvalidSigningMethod)
	}
}

func testJWTConfig() config.JWTConfig {
	return config.JWTConfig{
		SecretKey:      "test-secret-min-32-chars-1234567890",
		AccessTTLMin:   15,
		RefreshTTLDays: 7,
		BcryptCost:     4,
	}
}
