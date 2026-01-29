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
