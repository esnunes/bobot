package auth

import (
	"context"
	"testing"
)

func TestContextWithRole(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithUserID(ctx, 123)
	ctx = ContextWithRole(ctx, "admin")

	if UserIDFromContext(ctx) != 123 {
		t.Error("expected user_id 123")
	}
	if RoleFromContext(ctx) != "admin" {
		t.Errorf("expected role 'admin', got %s", RoleFromContext(ctx))
	}
}

func TestRoleFromContextEmpty(t *testing.T) {
	ctx := context.Background()

	if RoleFromContext(ctx) != "" {
		t.Error("expected empty role from empty context")
	}
}
