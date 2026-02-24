package schedule

import (
	"context"
	"testing"
	"time"

	"github.com/esnunes/bobot/auth"
)

func ctxWithUser(userID int64) context.Context {
	return auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: userID,
		Role:   "user",
	})
}

func ctxWithUserAndTopic(userID int64, topicID int64) context.Context {
	ctx := ctxWithUser(userID)
	return auth.ContextWithChatData(ctx, auth.ChatData{
		TopicID: topicID,
	})
}

func TestRemindTool_Name(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)
	if tool.Name() != "remind" {
		t.Errorf("got %q, want %q", tool.Name(), "remind")
	}
}

func TestRemindTool_ParseArgs(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	tests := []struct {
		name    string
		raw     string
		wantCmd string
		wantErr bool
	}{
		{"list", "list", "list", false},
		{"cancel", "cancel 5", "cancel", false},
		{"cancel missing id", "cancel", "", true},
		{"create", "2026-12-25T10:00:00Z buy presents", "create", false},
		{"empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cmd, _ := result["command"].(string); cmd != tt.wantCmd {
				t.Errorf("got command %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestRemindTool_Create(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	futureTime := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "create",
		"message": "call dentist",
		"run_at":  futureTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Verify reminder was created
	reminders, _ := db.ListPendingReminders(1)
	if len(reminders) != 1 {
		t.Fatalf("expected 1 reminder, got %d", len(reminders))
	}
	if reminders[0].Message != "call dentist" {
		t.Errorf("got message %q, want %q", reminders[0].Message, "call dentist")
	}
}

func TestRemindTool_CreatePast(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	pastTime := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)

	_, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "create",
		"message": "too late",
		"run_at":  pastTime,
	})
	if err == nil {
		t.Error("expected error for past run_at")
	}
}

func TestRemindTool_CreateMissingFields(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	// Missing message
	_, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "create",
		"run_at":  "2026-12-25T10:00:00Z",
	})
	if err == nil {
		t.Error("expected error for missing message")
	}

	// Missing run_at
	_, err = tool.Execute(ctxWithUser(1), map[string]any{
		"command": "create",
		"message": "test",
	})
	if err == nil {
		t.Error("expected error for missing run_at")
	}
}

func TestRemindTool_List(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	futureTime := time.Now().UTC().Add(1 * time.Hour)
	db.CreateReminder(1, 0, "reminder 1", futureTime)
	db.CreateReminder(1, 0, "reminder 2", futureTime.Add(1*time.Hour))
	db.CreateReminder(2, 0, "other user", futureTime) // different user

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestRemindTool_ListEmpty(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "No pending reminders." {
		t.Errorf("got %q, want 'No pending reminders.'", result)
	}
}

func TestRemindTool_ListByTopic(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	futureTime := time.Now().UTC().Add(1 * time.Hour)
	db.CreateReminder(1, 0, "private reminder", futureTime)
	db.CreateReminder(1, 42, "topic reminder", futureTime)

	// List with topic context should only show topic reminders
	result, err := tool.Execute(ctxWithUserAndTopic(1, 42), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "No pending reminders." {
		t.Error("expected topic reminders to be listed")
	}
}

func TestRemindTool_Cancel(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	futureTime := time.Now().UTC().Add(1 * time.Hour)
	id, _ := db.CreateReminder(1, 0, "cancel me", futureTime)

	// Cancel with float64 (JSON deserialization)
	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "cancel",
		"id":      float64(id),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Verify cancelled
	r, _ := db.GetReminder(id)
	if r.Status != "cancelled" {
		t.Errorf("got status %q, want %q", r.Status, "cancelled")
	}
}

func TestRemindTool_CancelWrongUser(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	futureTime := time.Now().UTC().Add(1 * time.Hour)
	id, _ := db.CreateReminder(1, 0, "not yours", futureTime)

	_, err := tool.Execute(ctxWithUser(2), map[string]any{
		"command": "cancel",
		"id":      float64(id),
	})
	if err == nil {
		t.Error("expected error when cancelling another user's reminder")
	}
}

func TestRemindTool_NoContext(t *testing.T) {
	db := newTestDB(t)
	tool := NewRemindTool(db)

	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "list",
	})
	if err == nil {
		t.Error("expected error without user context")
	}
}
