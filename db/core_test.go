// db/core_test.go
package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewCoreDB_CreatesSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "core.db")

	db, err := NewCoreDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"users", "refresh_tokens", "messages"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestCoreDB_CreateUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, err := db.CreateUser("testuser", "hashedpass")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.ID == 0 {
		t.Error("expected user ID to be set")
	}
	if user.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", user.Username)
	}
}

func TestCoreDB_GetUserByUsername(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	db.CreateUser("findme", "hashedpass")

	user, err := db.GetUserByUsername("findme")
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.Username != "findme" {
		t.Errorf("expected username findme, got %s", user.Username)
	}
}

func TestCoreDB_UserNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	_, err := db.GetUserByUsername("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCoreDB_RefreshTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("tokenuser", "hash")

	// Create token
	token, err := db.CreateRefreshToken(user.ID, "token123", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	if token.Token != "token123" {
		t.Errorf("expected token token123, got %s", token.Token)
	}

	// Get token
	found, err := db.GetRefreshToken("token123")
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}
	if found.UserID != user.ID {
		t.Errorf("expected user_id %d, got %d", user.ID, found.UserID)
	}

	// Delete token
	err = db.DeleteRefreshToken("token123")
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}

	_, err = db.GetRefreshToken("token123")
	if err != ErrNotFound {
		t.Error("expected token to be deleted")
	}
}

func TestCoreDB_DeleteExpiredTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("expireuser", "hash")

	// Create expired token
	db.CreateRefreshToken(user.ID, "expired", time.Now().Add(-1*time.Hour))
	// Create valid token
	db.CreateRefreshToken(user.ID, "valid", time.Now().Add(1*time.Hour))

	deleted, err := db.DeleteExpiredRefreshTokens()
	if err != nil {
		t.Fatalf("failed to delete expired: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Valid token should still exist
	_, err = db.GetRefreshToken("valid")
	if err != nil {
		t.Error("valid token should still exist")
	}
}

func TestCoreDB_Messages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("msguser", "hash")

	// Create messages
	msg1, err := db.CreateMessage(user.ID, "user", "Hello")
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}
	if msg1.Content != "Hello" {
		t.Errorf("expected content Hello, got %s", msg1.Content)
	}

	db.CreateMessage(user.ID, "assistant", "Hi there!")

	// Get messages
	messages, err := db.GetMessages(user.ID, 10)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Messages should be in chronological order
	if messages[0].Role != "user" {
		t.Error("first message should be from user")
	}
	if messages[1].Role != "assistant" {
		t.Error("second message should be from assistant")
	}
}

func TestCoreDB_GetMessagesLimit(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("limituser", "hash")

	for i := 0; i < 5; i++ {
		db.CreateMessage(user.ID, "user", "msg")
	}

	messages, _ := db.GetMessages(user.ID, 3)
	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}
}
