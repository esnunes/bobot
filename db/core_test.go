// db/core_test.go
package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *CoreDB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	return db
}

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

func TestCoreDB_MigratesExistingDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "core.db")

	// Create a database with old schema (without tokens/context_tokens columns)
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to create raw db: %v", err)
	}

	oldSchema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE refresh_tokens (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token TEXT UNIQUE NOT NULL,
		expires_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE messages (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = rawDB.Exec(oldSchema)
	if err != nil {
		t.Fatalf("failed to create old schema: %v", err)
	}

	// Insert a test message with old schema
	_, err = rawDB.Exec(`INSERT INTO users (username, password_hash) VALUES ('testuser', 'hash')`)
	if err != nil {
		t.Fatalf("failed to insert user: %v", err)
	}
	_, err = rawDB.Exec(`INSERT INTO messages (user_id, role, content) VALUES (1, 'user', 'hello')`)
	if err != nil {
		t.Fatalf("failed to insert message: %v", err)
	}
	rawDB.Close()

	// Now open with NewCoreDB which should migrate
	db, err := NewCoreDB(dbPath)
	if err != nil {
		t.Fatalf("failed to open db with migration: %v", err)
	}
	defer db.Close()

	// Verify the new columns exist by selecting them
	var tokens, contextTokens int
	err = db.db.QueryRow(`SELECT tokens, context_tokens FROM messages WHERE id = 1`).Scan(&tokens, &contextTokens)
	if err != nil {
		t.Fatalf("failed to select new columns: %v", err)
	}

	// Old messages should have default values
	if tokens != 0 {
		t.Errorf("expected tokens=0 for migrated row, got %d", tokens)
	}
	if contextTokens != 0 {
		t.Errorf("expected context_tokens=0 for migrated row, got %d", contextTokens)
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

func TestCoreDB_MessageTokenColumns(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	// Check that tokens and context_tokens columns exist
	var tokens, contextTokens int
	err := db.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages LIMIT 1
	`).Scan(&tokens, &contextTokens)

	// Should get no rows error, not column missing error
	if err != nil && err.Error() != "sql: no rows in result set" {
		t.Errorf("expected no rows error or success, got: %v", err)
	}
}

func TestCoreDB_CreateMessageWithTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("tokenuser", "hash")

	// First message starts a chunk (context_tokens = 0)
	msg1, err := db.CreateMessageWithContext(user.ID, "user", "Hello world")
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	// "Hello world" = 11 chars / 4 = 2 tokens (integer division)
	if msg1.Tokens != 2 {
		t.Errorf("expected 2 tokens, got %d", msg1.Tokens)
	}
	if msg1.ContextTokens != 0 {
		t.Errorf("first message should have context_tokens=0, got %d", msg1.ContextTokens)
	}

	// Second message continues the chunk
	msg2, _ := db.CreateMessageWithContext(user.ID, "assistant", "Hi there, how can I help?")
	// "Hi there, how can I help?" = 25 chars / 4 = 6 tokens
	if msg2.Tokens != 6 {
		t.Errorf("expected 6 tokens, got %d", msg2.Tokens)
	}
	// context_tokens = previous (0 + 2) + current (6) = 8
	if msg2.ContextTokens != 8 {
		t.Errorf("expected context_tokens=8, got %d", msg2.ContextTokens)
	}
}

func TestCoreDB_ChunkReset(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("chunkuser", "hash")

	// Use thresholds: start=10, max=30
	// Formula: contextTokens = prevContextTokens + prevTokens + tokens
	// Each 4 chars = 1 token (integer division)

	// msg1: "aaaa" = 4 chars = 1 token, ctx=0 (first message)
	db.CreateMessageWithContextThreshold(user.ID, "user", "aaaa", 10, 30)

	// msg2: "bbbbbbbb" = 8 chars = 2 tokens, ctx = 0 + 1 + 2 = 3
	db.CreateMessageWithContextThreshold(user.ID, "assistant", "bbbbbbbb", 10, 30)

	// msg3: "cccccccccccc" = 12 chars = 3 tokens, ctx = 3 + 2 + 3 = 8
	db.CreateMessageWithContextThreshold(user.ID, "user", "cccccccccccc", 10, 30)

	// msg4: "dddddddddddddddd" = 16 chars = 4 tokens, ctx = 8 + 3 + 4 = 15
	db.CreateMessageWithContextThreshold(user.ID, "assistant", "dddddddddddddddd", 10, 30)

	// msg5: "eeeeeeeeeeeeeeeeeeee" = 20 chars = 5 tokens, ctx = 15 + 4 + 5 = 24
	// 24 < 30, no reset yet
	msg5, _ := db.CreateMessageWithContextThreshold(user.ID, "user", "eeeeeeeeeeeeeeeeeeee", 10, 30)

	if msg5.ContextTokens != 24 {
		t.Errorf("expected context_tokens=24, got %d", msg5.ContextTokens)
	}

	// msg6: "ffffffffffffffffffffffff" = 24 chars = 6 tokens
	// Would be ctx = 24 + 5 + 6 = 35 > 30, triggers reset
	// targetThreshold = 30 - 10 = 20
	// Find most recent msg with ctx < 20: msg4 has ctx=15 < 20
	// Subtract 15 from msg4 onwards:
	//   msg4: 15 - 15 = 0
	//   msg5: 24 - 15 = 9
	// Then add msg6: ctx = 9 + 5 + 6 = 20
	msg6, _ := db.CreateMessageWithContextThreshold(user.ID, "assistant", "ffffffffffffffffffffffff", 10, 30)

	if msg6.ContextTokens != 20 {
		t.Errorf("expected context_tokens=20 after reset, got %d", msg6.ContextTokens)
	}

	// Verify msg4 is now chunk start (ctx=0)
	var msg4Ctx int
	db.db.QueryRow("SELECT context_tokens FROM messages WHERE user_id = ? ORDER BY id ASC LIMIT 1 OFFSET 3", user.ID).Scan(&msg4Ctx)
	if msg4Ctx != 0 {
		t.Errorf("expected msg4 context_tokens=0 (chunk start), got %d", msg4Ctx)
	}

	// Verify msg5 was updated (ctx=9)
	var msg5Ctx int
	db.db.QueryRow("SELECT context_tokens FROM messages WHERE user_id = ? ORDER BY id ASC LIMIT 1 OFFSET 4", user.ID).Scan(&msg5Ctx)
	if msg5Ctx != 9 {
		t.Errorf("expected msg5 context_tokens=9 after reset, got %d", msg5Ctx)
	}
}

func TestCoreDB_GetContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("ctxuser", "hash")

	// Create some messages - small thresholds for testing
	db.CreateMessageWithContextThreshold(user.ID, "user", "aaaa", 10, 20)         // msg1: ctx=0
	db.CreateMessageWithContextThreshold(user.ID, "assistant", "bbbb", 10, 20)     // msg2: ctx=2
	db.CreateMessageWithContextThreshold(user.ID, "user", "cccc", 10, 20)          // msg3: ctx=3

	// Force a reset by adding messages that exceed threshold
	db.CreateMessageWithContextThreshold(user.ID, "assistant", strings.Repeat("d", 40), 10, 20) // tokens=10, exceeds
	db.CreateMessageWithContextThreshold(user.ID, "user", "eeee", 10, 20)          // msg5

	// Get context messages (should only return from most recent chunk start)
	messages, err := db.GetContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get context messages: %v", err)
	}

	// Should not include msg1 and msg2 (before chunk reset)
	// First message in result should have context_tokens = 0
	if len(messages) == 0 {
		t.Fatal("expected at least one message")
	}
	if messages[0].ContextTokens != 0 {
		t.Errorf("first context message should have context_tokens=0, got %d", messages[0].ContextTokens)
	}
}

func TestCoreDB_GetMessagesBefore(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("pageuser", "hash")

	// Create 5 messages
	var lastID int64
	for i := 0; i < 5; i++ {
		msg, _ := db.CreateMessage(user.ID, "user", fmt.Sprintf("msg%d", i))
		lastID = msg.ID
	}

	// Get 2 messages before the last one
	messages, err := db.GetMessagesBefore(user.ID, lastID, 2)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	// Should be in DESC order (newest first of the older ones)
	if messages[0].Content != "msg3" {
		t.Errorf("expected msg3, got %s", messages[0].Content)
	}
	if messages[1].Content != "msg2" {
		t.Errorf("expected msg2, got %s", messages[1].Content)
	}
}

func TestCoreDB_GetMessagesSince(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("sinceuser", "hash")

	// Create messages with time gaps
	// Note: SQLite CURRENT_TIMESTAMP has second precision, so we need >1s gap
	db.CreateMessage(user.ID, "user", "old message")
	time.Sleep(1100 * time.Millisecond)

	since := time.Now()
	time.Sleep(1100 * time.Millisecond)

	db.CreateMessage(user.ID, "assistant", "new message 1")
	db.CreateMessage(user.ID, "user", "new message 2")

	messages, err := db.GetMessagesSince(user.ID, since)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestCoreDB_GetRecentMessagesIncludesTokens(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("recentuser", "hash")

	// Create message with context tracking
	db.CreateMessageWithContext(user.ID, "user", "Hello world")

	messages, err := db.GetRecentMessages(user.ID, 10)
	if err != nil {
		t.Fatalf("failed to get messages: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	// Tokens should be populated
	if messages[0].Tokens != 2 { // "Hello world" = 11 chars / 4 = 2
		t.Errorf("expected tokens=2, got %d", messages[0].Tokens)
	}
}

func TestCoreDB_CreateUserWithRole(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, err := db.CreateUserFull("testuser", "hashedpass", "Test User", "admin")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	if user.DisplayName != "Test User" {
		t.Errorf("expected display name 'Test User', got %s", user.DisplayName)
	}
	if user.Role != "admin" {
		t.Errorf("expected role 'admin', got %s", user.Role)
	}
	if user.Blocked {
		t.Error("expected user not blocked")
	}
}

func TestCoreDB_CreateInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")

	invite, err := db.CreateInvite(admin.ID, "abc123")
	if err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	if invite.Code != "abc123" {
		t.Errorf("expected code 'abc123', got %s", invite.Code)
	}
	if invite.CreatedBy != admin.ID {
		t.Errorf("expected created_by %d, got %d", admin.ID, invite.CreatedBy)
	}
	if invite.UsedBy != nil {
		t.Error("expected used_by to be nil")
	}
	if invite.Revoked {
		t.Error("expected invite not revoked")
	}
}

func TestCoreDB_GetInviteByCode(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "findme")

	invite, err := db.GetInviteByCode("findme")
	if err != nil {
		t.Fatalf("failed to get invite: %v", err)
	}
	if invite.Code != "findme" {
		t.Errorf("expected code 'findme', got %s", invite.Code)
	}
}

func TestCoreDB_UseInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "useme")

	user, _ := db.CreateUserFull("newuser", "hash", "New User", "user")
	err := db.UseInvite("useme", user.ID)
	if err != nil {
		t.Fatalf("failed to use invite: %v", err)
	}

	invite, _ := db.GetInviteByCode("useme")
	if invite.UsedBy == nil || *invite.UsedBy != user.ID {
		t.Error("expected invite to be marked as used")
	}
	if invite.UsedAt == nil {
		t.Error("expected used_at to be set")
	}
}

func TestCoreDB_RevokeInvite(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "revokeme")

	err := db.RevokeInvite("revokeme")
	if err != nil {
		t.Fatalf("failed to revoke invite: %v", err)
	}

	invite, _ := db.GetInviteByCode("revokeme")
	if !invite.Revoked {
		t.Error("expected invite to be revoked")
	}
}

func TestCoreDB_GetPendingInvites(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	admin, _ := db.CreateUserFull("admin", "hash", "Admin", "admin")
	db.CreateInvite(admin.ID, "pending1")
	db.CreateInvite(admin.ID, "pending2")
	db.CreateInvite(admin.ID, "used")
	db.CreateInvite(admin.ID, "revoked")

	user, _ := db.CreateUserFull("user", "hash", "User", "user")
	db.UseInvite("used", user.ID)
	db.RevokeInvite("revoked")

	invites, err := db.GetPendingInvites()
	if err != nil {
		t.Fatalf("failed to get pending invites: %v", err)
	}

	if len(invites) != 2 {
		t.Errorf("expected 2 pending invites, got %d", len(invites))
	}
}

func TestCoreDB_BlockUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUserFull("blockme", "hash", "Block Me", "user")

	err := db.BlockUser(user.ID)
	if err != nil {
		t.Fatalf("failed to block user: %v", err)
	}

	updated, _ := db.GetUserByID(user.ID)
	if !updated.Blocked {
		t.Error("expected user to be blocked")
	}
}

func TestCoreDB_UnblockUser(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUserFull("unblockme", "hash", "Unblock Me", "user")
	db.BlockUser(user.ID)

	err := db.UnblockUser(user.ID)
	if err != nil {
		t.Fatalf("failed to unblock user: %v", err)
	}

	updated, _ := db.GetUserByID(user.ID)
	if updated.Blocked {
		t.Error("expected user to be unblocked")
	}
}

func TestCoreDB_ListUsers(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	db.CreateUserFull("admin", "hash", "Admin User", "admin")
	db.CreateUserFull("user1", "hash", "User One", "user")
	db.CreateUserFull("user2", "hash", "User Two", "user")

	users, err := db.ListUsers()
	if err != nil {
		t.Fatalf("failed to list users: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}
}

func TestCreateGroup(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("owner", "hash")

	group, err := db.CreateGroup("Test Group", user.ID)
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}

	if group.ID == 0 {
		t.Error("expected non-zero group ID")
	}
	if group.Name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %q", group.Name)
	}
	if group.OwnerID != user.ID {
		t.Errorf("expected owner_id %d, got %d", user.ID, group.OwnerID)
	}
	if group.DeletedAt != nil {
		t.Error("expected nil deleted_at for new group")
	}
}

func TestAddGroupMember(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	err := db.AddGroupMember(group.ID, member.ID)
	if err != nil {
		t.Fatalf("AddGroupMember failed: %v", err)
	}

	// Adding same member again should fail or be idempotent
	err = db.AddGroupMember(group.ID, member.ID)
	// SQLite will error on duplicate primary key
	if err == nil {
		t.Error("expected error when adding duplicate member")
	}
}

func TestRemoveGroupMember(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)
	db.AddGroupMember(group.ID, member.ID)

	err := db.RemoveGroupMember(group.ID, member.ID)
	if err != nil {
		t.Fatalf("RemoveGroupMember failed: %v", err)
	}

	// Verify member is removed by checking membership
	isMember, _ := db.IsGroupMember(group.ID, member.ID)
	if isMember {
		t.Error("expected member to be removed")
	}
}

func TestGetGroupByID(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	created, _ := db.CreateGroup("Test Group", owner.ID)

	group, err := db.GetGroupByID(created.ID)
	if err != nil {
		t.Fatalf("GetGroupByID failed: %v", err)
	}
	if group.Name != "Test Group" {
		t.Errorf("expected name 'Test Group', got %q", group.Name)
	}

	// Test not found
	_, err = db.GetGroupByID(9999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetUserGroups(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")

	group1, _ := db.CreateGroup("Group 1", owner.ID)
	group2, _ := db.CreateGroup("Group 2", owner.ID)
	db.AddGroupMember(group1.ID, member.ID)
	db.AddGroupMember(group2.ID, member.ID)

	groups, err := db.GetUserGroups(member.ID)
	if err != nil {
		t.Fatalf("GetUserGroups failed: %v", err)
	}
	if len(groups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(groups))
	}
}

func TestSoftDeleteGroup(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	err := db.SoftDeleteGroup(group.ID)
	if err != nil {
		t.Fatalf("SoftDeleteGroup failed: %v", err)
	}

	// Should not be found after soft delete
	_, err = db.GetGroupByID(group.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestGetGroupMembers(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUserFull("owner", "hash", "Owner", "user")
	member, _ := db.CreateUserFull("member", "hash", "Member", "user")
	group, _ := db.CreateGroup("Test Group", owner.ID)
	db.AddGroupMember(group.ID, owner.ID)
	db.AddGroupMember(group.ID, member.ID)

	members, err := db.GetGroupMembers(group.ID)
	if err != nil {
		t.Fatalf("GetGroupMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestCreateGroupMessage(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	msg, err := db.CreateGroupMessage(group.ID, owner.ID, "user", "Hello group!")
	if err != nil {
		t.Fatalf("CreateGroupMessage failed: %v", err)
	}
	if msg.GroupID == nil || *msg.GroupID != group.ID {
		t.Error("expected group_id to be set")
	}
	if msg.Content != "Hello group!" {
		t.Errorf("expected content 'Hello group!', got %q", msg.Content)
	}
}

func TestGetGroupRecentMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	db.CreateGroupMessage(group.ID, owner.ID, "user", "Message 1")
	db.CreateGroupMessage(group.ID, owner.ID, "assistant", "Response 1")
	db.CreateGroupMessage(group.ID, owner.ID, "user", "Message 2")

	msgs, err := db.GetGroupRecentMessages(group.ID, 10)
	if err != nil {
		t.Fatalf("GetGroupRecentMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestGetGroupContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	group, _ := db.CreateGroup("Test Group", owner.ID)

	db.CreateGroupMessageWithContext(group.ID, owner.ID, "user", "Hello", 1000, 80000)
	db.CreateGroupMessageWithContext(group.ID, owner.ID, "assistant", "Hi there", 1000, 80000)

	msgs, err := db.GetGroupContextMessages(group.ID)
	if err != nil {
		t.Fatalf("GetGroupContextMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 context messages, got %d", len(msgs))
	}
}

func TestSessionRevocations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a user first
	user, err := db.CreateUserFull("testuser", "hash", "Test User", "user")
	if err != nil {
		t.Fatalf("CreateUserFull() error: %v", err)
	}

	// Create a revocation
	err = db.CreateSessionRevocation(user.ID, "logout_all")
	if err != nil {
		t.Fatalf("CreateSessionRevocation() error: %v", err)
	}

	// Check for revocation after the token was issued (before revocation)
	tokenIssuedAt := time.Now().Add(-1 * time.Hour)
	hasRevocation, err := db.HasSessionRevocation(user.ID, tokenIssuedAt)
	if err != nil {
		t.Fatalf("HasSessionRevocation() error: %v", err)
	}
	if !hasRevocation {
		t.Error("Expected revocation to be found for token issued before revocation")
	}

	// Check for revocation before the token was issued (after revocation)
	tokenIssuedAt = time.Now().Add(1 * time.Hour)
	hasRevocation, err = db.HasSessionRevocation(user.ID, tokenIssuedAt)
	if err != nil {
		t.Fatalf("HasSessionRevocation() error: %v", err)
	}
	if hasRevocation {
		t.Error("Expected no revocation for token issued after revocation")
	}
}

func TestDeleteOldSessionRevocations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, err := db.CreateUserFull("testuser", "hash", "Test User", "user")
	if err != nil {
		t.Fatalf("CreateUserFull() error: %v", err)
	}

	// Create a revocation
	err = db.CreateSessionRevocation(user.ID, "logout_all")
	if err != nil {
		t.Fatalf("CreateSessionRevocation() error: %v", err)
	}

	// Delete revocations older than 1 hour in the future (should delete the one we just created)
	deleted, err := db.DeleteOldSessionRevocations(time.Now().Add(1 * time.Hour))
	if err != nil {
		t.Fatalf("DeleteOldSessionRevocations() error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deletion, got %d", deleted)
	}

	// Verify it's gone
	hasRevocation, err := db.HasSessionRevocation(user.ID, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("HasSessionRevocation() error: %v", err)
	}
	if hasRevocation {
		t.Error("Expected no revocation after deletion")
	}
}
