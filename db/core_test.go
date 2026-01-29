// db/core_test.go
package db

import (
	"path/filepath"
	"testing"
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
