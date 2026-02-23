package skill

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	t.Helper()
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
	return auth.ContextWithChatData(ctx, auth.ChatData{TopicID: &topicID})
}

func TestSkillTool_Interface(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	tool := NewSkillTool(coreDB)
	if tool.Name() != "skill" {
		t.Errorf("expected name 'skill', got %q", tool.Name())
	}
	if tool.AdminOnly() {
		t.Error("expected AdminOnly to be false")
	}
}

func TestSkillTool_Create(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command":     "create",
		"name":        "groceries",
		"description": "Manage grocery lists",
		"content":     "Use task tool for groceries",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(result, "groceries") {
		t.Errorf("expected skill name in result, got: %s", result)
	}

	// Verify in DB
	skill, _ := coreDB.GetTopicSkillByName(topic.ID, "groceries")
	if skill == nil {
		t.Fatal("expected skill to exist in DB")
	}
}

func TestSkillTool_CreateTopic(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command":     "create",
		"name":        "notes",
		"description": "Meeting notes",
		"content":     "Summarize meetings",
	})
	if err != nil {
		t.Fatalf("create topic skill failed: %v", err)
	}
	if !strings.Contains(result, "notes") {
		t.Errorf("expected skill name in result, got: %s", result)
	}
}

func TestSkillTool_CreateTopicNotOwnerOrAdmin(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	topic, _ := coreDB.CreateTopic("General", alice.ID)
	coreDB.AddTopicMember(topic.ID, alice.ID)
	coreDB.AddTopicMember(topic.ID, bob.ID)
	tool := NewSkillTool(coreDB)

	// Bob is a member but not owner, not admin
	_, err := tool.Execute(ctxForUserInTopic(bob.ID, "user", topic.ID), map[string]any{
		"command": "create",
		"name":    "notes",
		"content": "content",
	})
	if err == nil {
		t.Error("expected permission error for non-owner/non-admin")
	}
}

func TestSkillTool_CreateTopicAdmin(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	topic, _ := coreDB.CreateTopic("General", alice.ID)
	coreDB.AddTopicMember(topic.ID, alice.ID)
	coreDB.AddTopicMember(topic.ID, admin.ID)
	tool := NewSkillTool(coreDB)

	// Admin can create topic skills even though not owner
	_, err := tool.Execute(ctxForUserInTopic(admin.ID, "admin", topic.ID), map[string]any{
		"command": "create",
		"name":    "notes",
		"content": "content",
	})
	if err != nil {
		t.Fatalf("admin create should succeed: %v", err)
	}
}

func TestSkillTool_CreateMissingName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	_, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command": "create",
		"content": "some content",
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestSkillTool_Update(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	ctx := ctxForUserInTopic(user.ID, "user", topic.ID)
	tool.Execute(ctx, map[string]any{
		"command": "create", "name": "groceries", "description": "old", "content": "old content",
	})

	result, err := tool.Execute(ctx, map[string]any{
		"command": "update", "name": "groceries", "description": "new desc", "content": "new content",
	})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !strings.Contains(result, "updated") {
		t.Errorf("expected update confirmation, got: %s", result)
	}

	skill, _ := coreDB.GetTopicSkillByName(topic.ID, "groceries")
	if skill.Content != "new content" {
		t.Errorf("expected updated content, got %q", skill.Content)
	}
}

func TestSkillTool_UpdateNotFound(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	_, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command": "update", "name": "nonexistent", "content": "new content",
	})
	if err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

func TestSkillTool_Delete(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	ctx := ctxForUserInTopic(user.ID, "user", topic.ID)
	tool.Execute(ctx, map[string]any{
		"command": "create", "name": "groceries", "content": "content",
	})

	result, err := tool.Execute(ctx, map[string]any{
		"command": "delete", "name": "groceries",
	})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected delete confirmation, got: %s", result)
	}
}

func TestSkillTool_List(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	ctx := ctxForUserInTopic(user.ID, "user", topic.ID)
	tool.Execute(ctx, map[string]any{
		"command": "create", "name": "groceries", "description": "Grocery lists", "content": "content",
	})
	tool.Execute(ctx, map[string]any{
		"command": "create", "name": "recipes", "description": "Recipe management", "content": "content",
	})

	result, err := tool.Execute(ctx, map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "groceries") || !strings.Contains(result, "recipes") {
		t.Errorf("expected both skills in list, got: %s", result)
	}
}

func TestSkillTool_ListEmpty(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateBobotTopic(user.ID)
	tool := NewSkillTool(coreDB)

	result, err := tool.Execute(ctxForUserInTopic(user.ID, "user", topic.ID), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "No skills") {
		t.Errorf("expected 'No skills' message, got: %s", result)
	}
}

func TestSkillTool_ParseArgs(t *testing.T) {
	tool := &SkillTool{}

	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: true},
		{name: "create", raw: "create groceries", want: map[string]any{"command": "create", "name": "groceries"}},
		{name: "update", raw: "update groceries", want: map[string]any{"command": "update", "name": "groceries"}},
		{name: "delete", raw: "delete groceries", want: map[string]any{"command": "delete", "name": "groceries"}},
		{name: "list", raw: "list", want: map[string]any{"command": "list"}},
		{name: "create multi-word", raw: "create my skill", want: map[string]any{"command": "create", "name": "my skill"}},
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
		})
	}
}
