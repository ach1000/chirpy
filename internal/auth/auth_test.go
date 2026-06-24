package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashPasswordAndCheckPasswordHash(t *testing.T) {
	password := "04234"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == password {
		t.Fatalf("expected hash to differ from plain password")
	}

	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash failed: %v", err)
	}

	if !match {
		t.Fatalf("expected password to match hash")
	}
}

func TestCheckPasswordHashWrongPassword(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	match, err := CheckPasswordHash("wrong", hash)
	if err != nil {
		t.Fatalf("CheckPasswordHash failed: %v", err)
	}

	if match {
		t.Fatalf("expected wrong password not to match hash")
	}
}

func TestMakeJWTAndValidateJWT(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}

	gotUserID, err := ValidateJWT(token, secret)
	if err != nil {
		t.Fatalf("ValidateJWT failed: %v", err)
	}

	if gotUserID != userID {
		t.Fatalf("expected user id %v, got %v", userID, gotUserID)
	}
}

func TestValidateJWTExpired(t *testing.T) {
	userID := uuid.New()
	secret := "test-secret"

	token, err := MakeJWT(userID, secret, -time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}

	if _, err := ValidateJWT(token, secret); err == nil {
		t.Fatalf("expected expired token to be rejected")
	}
}

func TestValidateJWTWrongSecret(t *testing.T) {
	userID := uuid.New()

	token, err := MakeJWT(userID, "right-secret", time.Hour)
	if err != nil {
		t.Fatalf("MakeJWT failed: %v", err)
	}

	if _, err := ValidateJWT(token, "wrong-secret"); err == nil {
		t.Fatalf("expected token signed with a different secret to be rejected")
	}
}
