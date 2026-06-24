package auth

import "testing"

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
