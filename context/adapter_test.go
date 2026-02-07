// context/adapter_test.go
package context

import (
	"path/filepath"
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
	coreDB.CreateMessageWithContext(user.ID, db.BobotUserID, "user", "Hello")
	coreDB.CreateMessageWithContext(db.BobotUserID, user.ID, "assistant", "Hi there")

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
