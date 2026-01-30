// auth/jwt_test.go
package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func TestJWTService_WrongSigningMethod(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	claims := &Claims{
		UserID: 123,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// Test "none" signing method (algorithm confusion attack)
	tokenNone := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStringNone, _ := tokenNone.SignedString(jwt.UnsafeAllowNoneSignatureType)

	_, err := svc.ValidateAccessToken(tokenStringNone)
	if err == nil {
		t.Error("expected error for token with 'none' signing method")
	}

	// Test HS384 - should reject even though it's HMAC family
	tokenHS384 := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	tokenStringHS384, _ := tokenHS384.SignedString([]byte("test-secret-key-32-chars-min!!"))

	_, err = svc.ValidateAccessToken(tokenStringHS384)
	if err == nil {
		t.Error("expected error for token with HS384 signing method")
	}
}

func TestJWTService_GenerateAccessTokenWithRole(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	token, err := svc.GenerateAccessTokenWithRole(123, "admin")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	if claims.UserID != 123 {
		t.Errorf("expected user_id 123, got %d", claims.UserID)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", claims.Role)
	}
}

func TestJWTService_RoleDefaultsToEmpty(t *testing.T) {
	svc := NewJWTService("test-secret-key-32-chars-min!!")

	// Old tokens without role should still work
	token, _ := svc.GenerateAccessToken(456)
	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("failed to validate token: %v", err)
	}

	// Role should be empty for backward compat
	if claims.Role != "" {
		t.Errorf("expected empty role for old token, got %s", claims.Role)
	}
}
