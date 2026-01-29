// auth/password_test.go
package auth

import "testing"

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("mypassword")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if hash == "mypassword" {
		t.Error("hash should not equal plaintext")
	}
	if len(hash) < 50 {
		t.Error("hash seems too short for bcrypt")
	}
}

func TestCheckPassword_Valid(t *testing.T) {
	hash, _ := HashPassword("correctpassword")

	if !CheckPassword("correctpassword", hash) {
		t.Error("expected password to match")
	}
}

func TestCheckPassword_Invalid(t *testing.T) {
	hash, _ := HashPassword("correctpassword")

	if CheckPassword("wrongpassword", hash) {
		t.Error("expected password not to match")
	}
}
