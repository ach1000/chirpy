package auth

import (
	"net/http"
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

func TestGetBearerToken(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer abc.def.ghi")

	token, err := GetBearerToken(headers)
	if err != nil {
		t.Fatalf("GetBearerToken failed: %v", err)
	}

	if token != "abc.def.ghi" {
		t.Fatalf("expected token %q, got %q", "abc.def.ghi", token)
	}
}

func TestGetBearerTokenMissingHeader(t *testing.T) {
	if _, err := GetBearerToken(http.Header{}); err == nil {
		t.Fatalf("expected an error when the Authorization header is missing")
	}
}

func TestGetBearerTokenMalformedHeader(t *testing.T) {
	headers := http.Header{}
	headers.Set("Authorization", "abc.def.ghi")

	if _, err := GetBearerToken(headers); err == nil {
		t.Fatalf("expected an error for a header missing the Bearer prefix")
	}
}
