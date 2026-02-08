package db

import (
	"path/filepath"
	"testing"
)

func TestRawContentColumn(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a message with raw_content
	msg, err := coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Hello", "\"Hello\"", 1000, 4000,
	)
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	if msg.RawContent != "\"Hello\"" {
		t.Errorf("expected raw_content '\"Hello\"', got '%s'", msg.RawContent)
	}

	// Verify tokens are estimated from raw_content length
	expectedTokens := len("\"Hello\"") / 4
	if msg.Tokens != expectedTokens {
		t.Errorf("expected tokens %d (from raw_content), got %d", expectedTokens, msg.Tokens)
	}
}
