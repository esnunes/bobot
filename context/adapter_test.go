// context/adapter_test.go
package context

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

func TestCoreDBAdapter_ImplementsProfileProvider(t *testing.T) {
	// Compile-time check that CoreDBAdapter implements ProfileProvider
	var _ assistant.ProfileProvider = (*CoreDBAdapter)(nil)
}

func TestCoreDBAdapter_GetContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")
	coreDB.CreatePrivateMessageWithContextThreshold(user.ID, db.BobotUserID, "user", "Hello", "Hello", 1000, 4000)
	coreDB.CreatePrivateMessageWithContextThreshold(db.BobotUserID, user.ID, "assistant", "Hi there", "Hi there", 1000, 4000)

	adapter := NewCoreDBAdapter(coreDB)

	messages, err := adapter.GetContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %s", messages[0].Role)
	}
}

func TestCoreDBAdapter_GetContextMessages_RawContent(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a message with raw_content containing a tool_use array
	rawContent := `[{"type":"text","text":"Let me check"},{"type":"tool_use","id":"call_1","name":"weather","input":{"loc":"Paris"}}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, db.BobotUserID, "assistant", "Let me check", rawContent, 1000, 4000,
	)

	adapter := NewCoreDBAdapter(coreDB)
	messages, err := adapter.GetContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].RawContent != rawContent {
		t.Errorf("expected raw_content preserved, got '%s'", messages[0].RawContent)
	}
}

func TestCoreDBAdapter_GetTopicContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	// Create user and topic
	user1, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("Test Topic", user1.ID)

	// Create topic messages with raw_content containing attribution
	coreDB.CreateTopicMessageWithContext(
		topic.ID, user1.ID, "user", "Hello", "[Alice]: Hello",
		1000, 4000,
	)
	coreDB.CreateTopicMessageWithContext(
		topic.ID, db.BobotUserID, "assistant", "Hi there", "Hi there",
		1000, 4000,
	)

	adapter := NewCoreDBAdapter(coreDB)
	messages, err := adapter.GetTopicContextMessages(topic.ID)
	if err != nil {
		t.Fatalf("failed to get topic messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %s", messages[0].Role)
	}
	if messages[0].RawContent != "[Alice]: Hello" {
		t.Errorf("expected raw_content '[Alice]: Hello', got '%s'", messages[0].RawContent)
	}
	if messages[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %s", messages[1].Role)
	}
}

func TestCoreDBAdapter_GetTopicMemberProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	// Create users with profiles
	user1, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	coreDB.UpsertUserProfile(user1.ID, "Alice is a morning person.", 0)

	user2, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	coreDB.UpsertUserProfile(user2.ID, "Bob handles groceries.", 0)

	// Create topic with both members
	topic, _ := coreDB.CreateTopic("Family", user1.ID)
	coreDB.AddTopicMember(topic.ID, user1.ID)
	coreDB.AddTopicMember(topic.ID, user2.ID)

	adapter := NewCoreDBAdapter(coreDB)
	profiles, err := adapter.GetTopicMemberProfiles(topic.ID)
	if err != nil {
		t.Fatalf("failed to get profiles: %v", err)
	}

	if profiles == "" {
		t.Fatal("expected non-empty profiles string")
	}

	// Should contain both members' profiles
	if !strings.Contains(profiles, "Alice is a morning person.") {
		t.Error("expected profiles to contain Alice's profile")
	}
	if !strings.Contains(profiles, "Bob handles groceries.") {
		t.Error("expected profiles to contain Bob's profile")
	}
	if !strings.Contains(profiles, `name="Alice"`) {
		t.Error("expected profiles to contain Alice's display name tag")
	}
	if !strings.Contains(profiles, `name="Bob"`) {
		t.Error("expected profiles to contain Bob's display name tag")
	}
}

func TestCoreDBAdapter_GetTopicMemberProfiles_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	user1, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	topic, _ := coreDB.CreateTopic("Empty", user1.ID)

	adapter := NewCoreDBAdapter(coreDB)
	profiles, err := adapter.GetTopicMemberProfiles(topic.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profiles != "" {
		t.Errorf("expected empty profiles when no member has a profile, got '%s'", profiles)
	}
}
