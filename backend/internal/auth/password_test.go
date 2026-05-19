package auth

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const samplePassword = "S3cret!Pass"

func TestHashPassword_RoundTrip(t *testing.T) {
	hashed := mustHashPassword(t, samplePassword, bcrypt.MinCost)

	if err := VerifyPassword(hashed, samplePassword); err != nil {
		t.Fatalf("VerifyPassword() error = %v", err)
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hashed := mustHashPassword(t, samplePassword, bcrypt.MinCost)

	err := VerifyPassword(hashed, "Wrong!Pass")
	if !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		t.Fatalf("VerifyPassword() error = %v, want %v", err, bcrypt.ErrMismatchedHashAndPassword)
	}
}

func TestHashPassword_DefaultCostWhenZero(t *testing.T) {
	hashed := mustHashPassword(t, samplePassword, 0)

	cost, err := bcrypt.Cost([]byte(hashed))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}
	if cost != bcrypt.DefaultCost {
		t.Fatalf("bcrypt.Cost() = %d, want %d", cost, bcrypt.DefaultCost)
	}
}

func TestHashPassword_RejectsInvalidCost(t *testing.T) {
	_, err := HashPassword(samplePassword, 1)
	if err == nil {
		t.Fatal("HashPassword() error = nil, want error")
	}
}

func mustHashPassword(t *testing.T, plain string, cost int) string {
	t.Helper()

	hashed, err := HashPassword(plain, cost)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	return hashed
}
