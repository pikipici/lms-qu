// Package auth uses bcrypt password hashing with a cost supplied by config;
// HashPassword treats cost 0 as bcrypt.DefaultCost (10).
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password using the supplied bcrypt cost.
func HashPassword(plain string, cost int) (string, error) {
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return "", fmt.Errorf("auth: bcrypt cost %d outside range [%d,%d]", cost, bcrypt.MinCost, bcrypt.MaxCost)
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(hashed), nil
}

// VerifyPassword compares a bcrypt hash against a plaintext password.
func VerifyPassword(hashed, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(plain))
}
