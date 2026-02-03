package auth

import (
	"testing"
	"time"
)

func TestSessionService_CreateAndDecrypt(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	token, err := svc.CreateToken(123, "admin")
	if err != nil {
		t.Fatalf("CreateToken() error: %v", err)
	}

	if token == "" {
		t.Fatal("CreateToken() returned empty token")
	}

	session, err := svc.DecryptToken(token)
	if err != nil {
		t.Fatalf("DecryptToken() error: %v", err)
	}

	if session.UserID != 123 {
		t.Errorf("UserID = %d, want 123", session.UserID)
	}
	if session.Role != "admin" {
		t.Errorf("Role = %s, want admin", session.Role)
	}
}

func TestSessionService_DecryptInvalidToken(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	_, err := svc.DecryptToken("invalid-token")
	if err == nil {
		t.Error("DecryptToken() should fail for invalid token")
	}
}

func TestSessionService_NeedsReissue(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	// Token that expires in 10 minutes - should not need reissue
	fresh := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	if svc.NeedsReissue(fresh) {
		t.Error("Fresh token should not need reissue")
	}

	// Token that expires in 3 minutes - should need reissue (within 5 min threshold)
	nearExpiry := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now().Add(-27 * time.Minute),
		ExpiresAt: time.Now().Add(3 * time.Minute),
	}
	if !svc.NeedsReissue(nearExpiry) {
		t.Error("Near-expiry token should need reissue")
	}

	// Token that expired 5 minutes ago - should need reissue
	expired := &SessionToken{
		UserID:    1,
		Role:      "user",
		IssuedAt:  time.Now().Add(-35 * time.Minute),
		ExpiresAt: time.Now().Add(-5 * time.Minute),
	}
	if !svc.NeedsReissue(expired) {
		t.Error("Expired token should need reissue")
	}
}

func TestSessionService_IsPastDeadline(t *testing.T) {
	svc := NewSessionService("test-secret-key-32-bytes-long!!", 30*time.Minute, 7*24*time.Hour, 5*time.Minute)

	// Token issued now - not past deadline
	fresh := &SessionToken{
		UserID:   1,
		Role:     "user",
		IssuedAt: time.Now(),
	}
	if svc.IsPastDeadline(fresh) {
		t.Error("Fresh token should not be past deadline")
	}

	// Token issued 8 days ago - past deadline
	old := &SessionToken{
		UserID:   1,
		Role:     "user",
		IssuedAt: time.Now().Add(-8 * 24 * time.Hour),
	}
	if !svc.IsPastDeadline(old) {
		t.Error("Old token should be past deadline")
	}
}
