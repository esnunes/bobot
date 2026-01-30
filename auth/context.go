// auth/context.go
package auth

import "context"

type contextKey string

const userIDKey contextKey = "user_id"

// ContextWithUserID returns a new context with the user ID stored.
func ContextWithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// UserIDFromContext extracts the user ID from the context.
// Returns 0 if no user ID is present.
func UserIDFromContext(ctx context.Context) int64 {
	if id, ok := ctx.Value(userIDKey).(int64); ok {
		return id
	}
	return 0
}

type roleKey struct{}

// ContextWithRole returns a new context with the role stored.
func ContextWithRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

// RoleFromContext extracts the role from the context.
// Returns empty string if no role is present.
func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(roleKey{}).(string)
	return role
}
