package topic

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func ctxForUser(userID int64, role string) context.Context {
	return auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: userID,
		Role:   role,
	})
}

func ctxForUserInTopic(userID int64, role string, topicID int64) context.Context {
	ctx := ctxForUser(userID, role)
	return auth.ContextWithChatData(ctx, auth.ChatData{TopicID: topicID})
}

func TestTopicTool_Create(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "create", "name": "General"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(result, `"General"`) {
		t.Errorf("expected topic name in result, got: %s", result)
	}

	// Verify topic exists and user is a member
	topic, _ := coreDB.GetTopicByName("General")
	if topic == nil {
		t.Fatal("expected topic to exist")
	}
	if topic.OwnerID != user.ID {
		t.Errorf("expected owner to be user, got %d", topic.OwnerID)
	}
	isMember, _ := coreDB.IsTopicMember(topic.ID, user.ID)
	if !isMember {
		t.Error("expected creator to be a member")
	}
}

func TestTopicTool_CreateMissingName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "create"})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestTopicTool_CreateDuplicateNameAllowed(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "create", "name": "General"})
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	// Topic names are not globally unique — creating with same name should succeed
	_, err = tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "create", "name": "general"})
	if err != nil {
		t.Fatalf("second create (same name) failed: %v", err)
	}
}

func TestTopicTool_List(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	// Create two topics
	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	tool.Execute(ctxForUser(bob.ID, "user"), map[string]any{"command": "create", "name": "Random"})

	// Alice should see only General
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "General") {
		t.Errorf("expected General in list, got: %s", result)
	}
	if strings.Contains(result, "Random") {
		t.Errorf("expected Random NOT in alice's list, got: %s", result)
	}
}

func TestTopicTool_ListEmpty(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "No topics") {
		t.Errorf("expected 'No topics' message, got: %s", result)
	}
}

func TestTopicTool_Delete(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")

	// Delete from within topic chat (no name needed)
	result, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "delete"},
	)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected deletion confirmation, got: %s", result)
	}

	// Verify deleted
	_, err = coreDB.GetTopicByName("General")
	if err != db.ErrNotFound {
		t.Error("expected topic to be deleted")
	}
}

func TestTopicTool_DeleteByName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})

	// Delete by name (from private chat)
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "delete", "name": "General"})
	if err != nil {
		t.Fatalf("delete by name failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected deletion confirmation, got: %s", result)
	}
}

func TestTopicTool_DeleteNotOwner(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})

	_, err := tool.Execute(ctxForUser(bob.ID, "user"), map[string]any{"command": "delete", "name": "General"})
	if err == nil {
		t.Error("expected error for non-owner delete")
	}
	if !strings.Contains(err.Error(), "owner") {
		t.Errorf("expected owner error, got: %v", err)
	}
}

func TestTopicTool_Leave(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Bob leaves
	result, err := tool.Execute(
		ctxForUserInTopic(bob.ID, "user", topic.ID),
		map[string]any{"command": "leave"},
	)
	if err != nil {
		t.Fatalf("leave failed: %v", err)
	}
	if !strings.Contains(result, "left") {
		t.Errorf("expected leave confirmation, got: %s", result)
	}

	// Verify bob is no longer a member
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if isMember {
		t.Error("expected bob to no longer be a member")
	}
}

func TestTopicTool_LeaveOwnerDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "leave"},
	)
	if err == nil {
		t.Error("expected error when owner tries to leave")
	}
	if !strings.Contains(err.Error(), "owner cannot leave") {
		t.Errorf("expected owner error, got: %v", err)
	}
}

func TestTopicTool_AddMember(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")

	// Add bob from within topic chat
	result, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "add", "username": "bob"},
	)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if !strings.Contains(result, "bob") || !strings.Contains(result, "added") {
		t.Errorf("expected add confirmation, got: %s", result)
	}

	// Verify bob is a member
	bob, _ := coreDB.GetUserByUsername("bob")
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if !isMember {
		t.Error("expected bob to be a member")
	}
}

func TestTopicTool_AddMemberByTopicName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})

	// Add bob from private chat (specify topic name)
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{
		"command":  "add",
		"username": "bob",
		"name":     "General",
	})
	if err != nil {
		t.Fatalf("add by topic name failed: %v", err)
	}
	if !strings.Contains(result, "bob") {
		t.Errorf("expected add confirmation, got: %s", result)
	}
}

func TestTopicTool_AddMemberNotOwner(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	coreDB.CreateUserFull("charlie", "hash", "Charlie", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Bob tries to add charlie — should fail
	_, err := tool.Execute(
		ctxForUserInTopic(bob.ID, "user", topic.ID),
		map[string]any{"command": "add", "username": "charlie"},
	)
	if err == nil {
		t.Error("expected error for non-owner add")
	}
}

func TestTopicTool_AddMemberUserNotFound(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "add", "username": "nonexistent"},
	)
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestTopicTool_RemoveMember(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Remove bob
	result, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "remove", "username": "bob"},
	)
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !strings.Contains(result, "removed") {
		t.Errorf("expected remove confirmation, got: %s", result)
	}

	// Verify bob is removed
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if isMember {
		t.Error("expected bob to be removed")
	}
}

func TestTopicTool_RemoveSelfDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(
		ctxForUserInTopic(alice.ID, "user", topic.ID),
		map[string]any{"command": "remove", "username": "alice"},
	)
	if err == nil {
		t.Error("expected error when owner removes self")
	}
}

func TestTopicTool_UnknownCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), map[string]any{"command": "foobar"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestTopicTool_NoTopicContext(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "create", "name": "General"})

	// Try to delete without name and without topic context
	_, err := tool.Execute(ctxForUser(alice.ID, "user"), map[string]any{"command": "delete"})
	if err == nil {
		t.Error("expected error when no topic context and no name")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestTopicTool_ParseArgs(t *testing.T) {
	tool := &TopicTool{}

	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{
			name:    "empty input",
			raw:     "",
			wantErr: true,
		},
		{
			name: "create with name",
			raw:  "create General",
			want: map[string]any{"command": "create", "name": "General"},
		},
		{
			name: "create with multi-word name",
			raw:  "create My Cool Topic",
			want: map[string]any{"command": "create", "name": "My Cool Topic"},
		},
		{
			name: "delete with name",
			raw:  "delete General",
			want: map[string]any{"command": "delete", "name": "General"},
		},
		{
			name: "delete without name (topic context)",
			raw:  "delete",
			want: map[string]any{"command": "delete"},
		},
		{
			name: "leave without name",
			raw:  "leave",
			want: map[string]any{"command": "leave"},
		},
		{
			name: "leave with multi-word name",
			raw:  "leave My Cool Topic",
			want: map[string]any{"command": "leave", "name": "My Cool Topic"},
		},
		{
			name: "add with username only",
			raw:  "add bob",
			want: map[string]any{"command": "add", "username": "bob"},
		},
		{
			name: "add with username and topic name",
			raw:  "add bob General",
			want: map[string]any{"command": "add", "username": "bob", "name": "General"},
		},
		{
			name: "add with username and multi-word topic name",
			raw:  "add bob My Cool Topic",
			want: map[string]any{"command": "add", "username": "bob", "name": "My Cool Topic"},
		},
		{
			name: "remove with username only",
			raw:  "remove charlie",
			want: map[string]any{"command": "remove", "username": "charlie"},
		},
		{
			name: "remove with username and topic name",
			raw:  "remove charlie General",
			want: map[string]any{"command": "remove", "username": "charlie", "name": "General"},
		},
		{
			name: "list command",
			raw:  "list",
			want: map[string]any{"command": "list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %v, want %v", k, got[k], v)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d keys, want %d keys", len(got), len(tt.want))
			}
		})
	}
}
