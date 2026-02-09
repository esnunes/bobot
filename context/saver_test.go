package context

import (
	"path/filepath"
	"testing"

	"github.com/esnunes/bobot/db"
)

func TestCoreDBMessageSaver_SaveTopicMessage(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	// Create user and topic
	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("Test", user.ID)

	saver := NewCoreDBMessageSaver(coreDB, 1000, 4000)

	// Save assistant message — senderID should be BobotUserID
	err := saver.SaveTopicMessage(topic.ID, user.ID, "assistant", "Hello!", "Hello!")
	if err != nil {
		t.Fatalf("failed to save topic message: %v", err)
	}

	// Save user tool_result — senderID should be userID
	err = saver.SaveTopicMessage(topic.ID, user.ID, "user", "", `[{"type":"tool_result"}]`)
	if err != nil {
		t.Fatalf("failed to save tool result: %v", err)
	}

	// Verify messages were saved
	messages, err := coreDB.GetTopicContextMessages(topic.ID)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Assistant message: sender should be BobotUserID
	if messages[0].SenderID != db.BobotUserID {
		t.Errorf("expected assistant sender to be BobotUserID (%d), got %d", db.BobotUserID, messages[0].SenderID)
	}

	// User message: sender should be the user
	if messages[1].SenderID != user.ID {
		t.Errorf("expected user sender to be %d, got %d", user.ID, messages[1].SenderID)
	}
}
