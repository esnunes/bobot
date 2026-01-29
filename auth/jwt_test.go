// auth/jwt_test.go
package auth

import (
	"testing"
	"time"
)

func TestJWTService_GenerateAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, err := svc.GenerateAccessToken(123)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestJWTService_ValidateAccessToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, _ := svc.GenerateAccessToken(456)

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}
	if claims.UserID != 456 {
		t.Errorf("expected user_id 456, got %d", claims.UserID)
	}
}

func TestJWTService_ExpiredToken(t *testing.T) {
	svc := &JWTService{
		secret:     []byte("test-secret-key-32-chars-min!!"),
		accessTTL:  -1 * time.Hour, // Already expired
		refreshTTL: 7 * 24 * time.Hour,
	}

	token, _ := svc.GenerateAccessToken(789)

	_, err := svc.ValidateAccessToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestJWTService_InvalidToken(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	_, err := svc.ValidateAccessToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestJWTService_GenerateRefreshToken(t *testing.T) {
	svc := NewJWTService("test-secret")

	token := svc.GenerateRefreshToken()
	if len(token) < 32 {
		t.Error("refresh token should be at least 32 chars")
	}
}
