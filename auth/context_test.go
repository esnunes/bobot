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

	data := UserDataFromContext(ctx)
	if data.UserID != 123 {
		t.Error("expected user_id 123")
	}
	if data.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", data.Role)
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

	data := UserDataFromContext(ctx)
	if data.UserID != 0 || data.Role != "" {
		t.Error("expected zero values from empty context")
	}
}

func TestContextWithChatData(t *testing.T) {
	receiverID := int64(42)
	topicID := int64(99)

	ctx := context.Background()
	ctx = ContextWithChatData(ctx, ChatData{
		ReceiverID: &receiverID,
		TopicID:    &topicID,
	})

	data := ChatDataFromContext(ctx)
	if data.ReceiverID == nil || *data.ReceiverID != 42 {
		t.Errorf("expected receiver_id 42, got %v", data.ReceiverID)
	}
	if data.TopicID == nil || *data.TopicID != 99 {
		t.Errorf("expected topic_id 99, got %v", data.TopicID)
	}
}

func TestChatDataFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	data := ChatDataFromContext(ctx)
	if data.ReceiverID != nil || data.TopicID != nil {
		t.Error("expected nil values from empty context")
	}
}

func TestChatDataWithNilFields(t *testing.T) {
	ctx := context.Background()
	ctx = ContextWithChatData(ctx, ChatData{})

	data := ChatDataFromContext(ctx)
	if data.ReceiverID != nil || data.TopicID != nil {
		t.Error("expected nil values")
	}
}
