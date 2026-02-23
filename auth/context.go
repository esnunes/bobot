// auth/context.go
package auth

import "context"

type userDataKey struct{}

type UserData struct {
	UserID int64
	Role   string
}

// ContextWithUserData returns a new context with the user data stored.
func ContextWithUserData(ctx context.Context, data UserData) context.Context {
	return context.WithValue(ctx, userDataKey{}, data)
}

// UserDataFromContext extracts the user data from the context.
// Returns zero values if no user data is present.
func UserDataFromContext(ctx context.Context) UserData {
	data, _ := ctx.Value(userDataKey{}).(UserData)
	return data
}

type chatDataKey struct{}

type ChatData struct {
	TopicID *int64
}

// ContextWithChatData returns a new context with the chat data stored.
func ContextWithChatData(ctx context.Context, data ChatData) context.Context {
	return context.WithValue(ctx, chatDataKey{}, data)
}

// ChatDataFromContext extracts the chat data from the context.
// Returns zero values if no chat data is present.
func ChatDataFromContext(ctx context.Context) ChatData {
	data, _ := ctx.Value(chatDataKey{}).(ChatData)
	return data
}
