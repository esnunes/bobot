package auth

import (
	"context"
	"testing"
)

func TestContextWithUserData(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUserData(ctx, UserData{
		UserID: 123,
		Role:   "admin",
	})

	if UserIDFromContext(ctx) != 123 {
		t.Error("expected user_id 123")
	}
	if RoleFromContext(ctx) != "admin" {
		t.Errorf("expected role 'admin', got %s", RoleFromContext(ctx))
	}
}

func TestUserDataFromContext(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUserData(ctx, UserData{
		UserID: 456,
		Role:   "user",
	})

	data := UserDataFromContext(ctx)
	if data.UserID != 456 {
		t.Errorf("expected user_id 456, got %d", data.UserID)
	}
	if data.Role != "user" {
		t.Errorf("expected role 'user', got %s", data.Role)
	}
}

func TestEmptyContext(t *testing.T) {
	ctx := context.Background()

	if UserIDFromContext(ctx) != 0 {
		t.Error("expected user_id 0 from empty context")
	}
	if RoleFromContext(ctx) != "" {
		t.Error("expected empty role from empty context")
	}

	data := UserDataFromContext(ctx)
	if data.UserID != 0 || data.Role != "" {
		t.Error("expected zero values from empty context")
	}
}
