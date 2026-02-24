// db/core_test.go
package db

import (
	"database/sql"
	"path/filepath"
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

	// Includes the system bobot user (ID 0) created during migration
	if len(users) != 4 {
		t.Errorf("expected 4 users (including bobot), got %d", len(users))
	}
}

func TestCreateTopic(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("owner", "hash")

	topic, err := db.CreateTopic("Test Topic", user.ID)
	if err != nil {
		t.Fatalf("CreateTopic failed: %v", err)
	}

	if topic.ID == 0 {
		t.Error("expected non-zero topic ID")
	}
	if topic.Name != "Test Topic" {
		t.Errorf("expected name 'Test Topic', got %q", topic.Name)
	}
	if topic.OwnerID != user.ID {
		t.Errorf("expected owner_id %d, got %d", user.ID, topic.OwnerID)
	}
	if topic.DeletedAt != nil {
		t.Error("expected nil deleted_at for new topic")
	}
}

func TestAddTopicMember(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)

	err := db.AddTopicMember(topic.ID, member.ID)
	if err != nil {
		t.Fatalf("AddTopicMember failed: %v", err)
	}

	// Adding same member again should fail or be idempotent
	err = db.AddTopicMember(topic.ID, member.ID)
	// SQLite will error on duplicate primary key
	if err == nil {
		t.Error("expected error when adding duplicate member")
	}
}

func TestRemoveTopicMember(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)
	db.AddTopicMember(topic.ID, member.ID)

	err := db.RemoveTopicMember(topic.ID, member.ID)
	if err != nil {
		t.Fatalf("RemoveTopicMember failed: %v", err)
	}

	// Verify member is removed by checking membership
	isMember, _ := db.IsTopicMember(topic.ID, member.ID)
	if isMember {
		t.Error("expected member to be removed")
	}
}

func TestGetTopicByID(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	created, _ := db.CreateTopic("Test Topic", owner.ID)

	topic, err := db.GetTopicByID(created.ID)
	if err != nil {
		t.Fatalf("GetTopicByID failed: %v", err)
	}
	if topic.Name != "Test Topic" {
		t.Errorf("expected name 'Test Topic', got %q", topic.Name)
	}

	// Test not found
	_, err = db.GetTopicByID(9999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetUserTopics(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	member, _ := db.CreateUser("member", "hash")

	topic1, _ := db.CreateTopic("Topic 1", owner.ID)
	topic2, _ := db.CreateTopic("Topic 2", owner.ID)
	db.AddTopicMember(topic1.ID, member.ID)
	db.AddTopicMember(topic2.ID, member.ID)

	topics, err := db.GetUserTopics(member.ID)
	if err != nil {
		t.Fatalf("GetUserTopics failed: %v", err)
	}
	if len(topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topics))
	}
}

func TestSoftDeleteTopic(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)

	err := db.SoftDeleteTopic(topic.ID)
	if err != nil {
		t.Fatalf("SoftDeleteTopic failed: %v", err)
	}

	// Should not be found after soft delete
	_, err = db.GetTopicByID(topic.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestGetTopicMembers(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUserFull("owner", "hash", "Owner", "user")
	member, _ := db.CreateUserFull("member", "hash", "Member", "user")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)
	db.AddTopicMember(topic.ID, owner.ID)
	db.AddTopicMember(topic.ID, member.ID)

	members, err := db.GetTopicMembers(topic.ID)
	if err != nil {
		t.Fatalf("GetTopicMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

func TestCreateTopicMessage(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)

	msg, err := db.CreateTopicMessage(topic.ID, owner.ID, "user", "Hello topic!", "Hello topic!")
	if err != nil {
		t.Fatalf("CreateTopicMessage failed: %v", err)
	}
	if msg.TopicID != topic.ID {
		t.Error("expected topic_id to be set")
	}
	if msg.Content != "Hello topic!" {
		t.Errorf("expected content 'Hello topic!', got %q", msg.Content)
	}
}

func TestGetTopicRecentMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)

	db.CreateTopicMessage(topic.ID, owner.ID, "user", "Message 1", "Message 1")
	db.CreateTopicMessage(topic.ID, owner.ID, "assistant", "Response 1", "Response 1")
	db.CreateTopicMessage(topic.ID, owner.ID, "user", "Message 2", "Message 2")

	msgs, err := db.GetTopicRecentMessages(topic.ID, 10)
	if err != nil {
		t.Fatalf("GetTopicRecentMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}
}

func TestGetTopicContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	topic, _ := db.CreateTopic("Test Topic", owner.ID)

	db.CreateTopicMessageWithContext(topic.ID, owner.ID, "user", "Hello", "Hello", 1000, 80000)
	db.CreateTopicMessageWithContext(topic.ID, owner.ID, "assistant", "Hi there", "Hi there", 1000, 80000)

	msgs, err := db.GetTopicContextMessages(topic.ID)
	if err != nil {
		t.Fatalf("GetTopicContextMessages failed: %v", err)
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

func TestGetTopicByName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	db.CreateTopic("General", owner.ID)

	// Exact match
	topic, err := db.GetTopicByName("General")
	if err != nil {
		t.Fatalf("GetTopicByName failed: %v", err)
	}
	if topic.Name != "General" {
		t.Errorf("expected name 'General', got %q", topic.Name)
	}

	// Case-insensitive match
	topic, err = db.GetTopicByName("general")
	if err != nil {
		t.Fatalf("GetTopicByName case-insensitive failed: %v", err)
	}
	if topic.Name != "General" {
		t.Errorf("expected name 'General', got %q", topic.Name)
	}

	// Not found
	_, err = db.GetTopicByName("nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Deleted topic not found
	db.SoftDeleteTopic(topic.ID)
	_, err = db.GetTopicByName("General")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for deleted topic, got %v", err)
	}
}

func TestTopicNameNotGloballyUnique(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	_, err := db.CreateTopic("General", owner.ID)
	if err != nil {
		t.Fatalf("first CreateTopic failed: %v", err)
	}

	// Creating topic with same name should succeed (names are not globally unique)
	_, err = db.CreateTopic("General", owner.ID)
	if err != nil {
		t.Fatalf("second CreateTopic failed: %v", err)
	}
}

func TestCoreDB_UserProfilesTableExists(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Verify user_profiles table exists
	var name string
	err := db.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='user_profiles'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("user_profiles table not found: %v", err)
	}
}

func TestCoreDB_GetUserProfile_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("profileuser", "hash")

	content, lastMsgID, err := db.GetUserProfile(user.ID)
	if err != nil {
		t.Fatalf("GetUserProfile failed: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty content, got %q", content)
	}
	if lastMsgID != 0 {
		t.Errorf("expected lastMsgID=0, got %d", lastMsgID)
	}
}

func TestCoreDB_UpsertUserProfile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("profileuser", "hash")

	// Insert new profile
	err := db.UpsertUserProfile(user.ID, "Likes Go programming.", 42)
	if err != nil {
		t.Fatalf("UpsertUserProfile (insert) failed: %v", err)
	}

	content, lastMsgID, _ := db.GetUserProfile(user.ID)
	if content != "Likes Go programming." {
		t.Errorf("expected 'Likes Go programming.', got %q", content)
	}
	if lastMsgID != 42 {
		t.Errorf("expected lastMsgID=42, got %d", lastMsgID)
	}

	// Update existing profile
	err = db.UpsertUserProfile(user.ID, "Likes Go and Rust.", 100)
	if err != nil {
		t.Fatalf("UpsertUserProfile (update) failed: %v", err)
	}

	content, lastMsgID, _ = db.GetUserProfile(user.ID)
	if content != "Likes Go and Rust." {
		t.Errorf("expected 'Likes Go and Rust.', got %q", content)
	}
	if lastMsgID != 100 {
		t.Errorf("expected lastMsgID=100, got %d", lastMsgID)
	}
}

func TestCoreDB_GetUserMessagesSince(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("msguser", "hash")
	bobotTopic, _ := db.CreateBobotTopic(user.ID)

	// Create mixed messages
	msg1, _ := db.CreateTopicMessage(bobotTopic.ID, user.ID, "user", "Hello", "Hello")        // user msg
	db.CreateTopicMessage(bobotTopic.ID, BobotUserID, "assistant", "Hi!", "Hi!")               // assistant msg
	msg3, _ := db.CreateTopicMessage(bobotTopic.ID, user.ID, "user", "How are you?", "How are you?") // user msg

	// Get messages since before all messages
	msgs, err := db.GetUserMessagesSince(user.ID, 0)
	if err != nil {
		t.Fatalf("GetUserMessagesSince failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 user messages, got %d", len(msgs))
	}

	// Get messages since msg1 (should only return msg3)
	msgs, err = db.GetUserMessagesSince(user.ID, msg1.ID)
	if err != nil {
		t.Fatalf("GetUserMessagesSince failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ID != msg3.ID {
		t.Errorf("expected message ID %d, got %d", msg3.ID, msgs[0].ID)
	}
}

func TestCoreDB_ListActiveUsers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	db.CreateUserFull("active1", "hash", "Active One", "user")
	db.CreateUserFull("active2", "hash", "Active Two", "admin")
	blocked, _ := db.CreateUserFull("blocked", "hash", "Blocked", "user")
	db.BlockUser(blocked.ID)

	users, err := db.ListActiveUsers()
	if err != nil {
		t.Fatalf("ListActiveUsers failed: %v", err)
	}

	// Should return active1 and active2, exclude bobot (id=0) and blocked user
	if len(users) != 2 {
		t.Errorf("expected 2 active users, got %d", len(users))
	}

	for _, u := range users {
		if u.ID == BobotUserID {
			t.Error("should not include bobot system user")
		}
		if u.Blocked {
			t.Error("should not include blocked users")
		}
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

func TestCoreDB_MarkChatRead_BobotTopic(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("reader", "hash")
	bobotTopic, _ := db.CreateBobotTopic(user.ID)

	// Create a message in the bobot topic
	msg, _ := db.CreateTopicMessage(bobotTopic.ID, user.ID, "user", "hello", "hello")

	// Before marking read, should show unread
	unreads, err := db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if !unreads[bobotTopic.ID] {
		t.Error("expected bobot topic to be unread")
	}

	// Mark as read
	err = db.MarkChatRead(user.ID, bobotTopic.ID, msg.ID)
	if err != nil {
		t.Fatalf("MarkChatRead failed: %v", err)
	}

	// After marking read, should not show unread
	unreads, err = db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if unreads[bobotTopic.ID] {
		t.Error("expected bobot topic to be read after MarkChatRead")
	}
}

func TestCoreDB_MarkChatRead_TopicChat(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("reader", "hash")
	topic, _ := db.CreateTopic("Test Topic", user.ID)
	db.AddTopicMember(topic.ID, user.ID)

	// Create a topic message
	msg, _ := db.CreateTopicMessage(topic.ID, user.ID, "user", "hello topic", "hello topic")

	// Before marking read, should show unread
	unreads, err := db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if !unreads[topic.ID] {
		t.Error("expected topic chat to be unread")
	}

	// Mark as read
	err = db.MarkChatRead(user.ID, topic.ID, msg.ID)
	if err != nil {
		t.Fatalf("MarkChatRead failed: %v", err)
	}

	// After marking read, should not show unread
	unreads, err = db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if unreads[topic.ID] {
		t.Error("expected topic chat to be read after MarkChatRead")
	}
}

func TestCoreDB_MarkChatRead_Upsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("reader", "hash")
	bobotTopic, _ := db.CreateBobotTopic(user.ID)

	// Mark read with message ID 1
	err := db.MarkChatRead(user.ID, bobotTopic.ID, 1)
	if err != nil {
		t.Fatalf("MarkChatRead failed: %v", err)
	}

	// Mark read again with message ID 5 (upsert)
	err = db.MarkChatRead(user.ID, bobotTopic.ID, 5)
	if err != nil {
		t.Fatalf("MarkChatRead upsert failed: %v", err)
	}

	// Verify it was updated, not duplicated
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM chat_read_status WHERE user_id = ? AND topic_id = ?", user.ID, bobotTopic.ID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}

	var lastRead int64
	db.db.QueryRow("SELECT last_read_message_id FROM chat_read_status WHERE user_id = ? AND topic_id = ?", user.ID, bobotTopic.ID).Scan(&lastRead)
	if lastRead != 5 {
		t.Errorf("expected last_read_message_id=5, got %d", lastRead)
	}
}

func TestCoreDB_GetUnreadChats_NoRowsMeansRead(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("newuser", "hash")

	// No messages, no read status — should not show unread
	unreads, err := db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if len(unreads) != 0 {
		t.Errorf("expected no unreads, got %d", len(unreads))
	}
}

func TestCoreDB_GetUnreadChats_NewMessageAfterRead(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("reader", "hash")
	bobotTopic, _ := db.CreateBobotTopic(user.ID)

	// Create and read a message in the bobot topic
	msg1, _ := db.CreateTopicMessage(bobotTopic.ID, user.ID, "user", "hello", "hello")
	db.MarkChatRead(user.ID, bobotTopic.ID, msg1.ID)

	// New message arrives from bobot
	db.CreateTopicMessage(bobotTopic.ID, BobotUserID, "assistant", "hi there", "hi there")

	// Should show unread again
	unreads, err := db.GetUnreadChats(user.ID)
	if err != nil {
		t.Fatalf("GetUnreadChats failed: %v", err)
	}
	if !unreads[bobotTopic.ID] {
		t.Error("expected bobot topic to be unread after new message")
	}
}

func TestCoreDB_GetLatestTopicMessageID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("reader", "hash")
	topic, _ := db.CreateTopic("Test", user.ID)

	// No messages
	id, err := db.GetLatestTopicMessageID(topic.ID)
	if err != nil {
		t.Fatalf("GetLatestTopicMessageID failed: %v", err)
	}
	if id != 0 {
		t.Errorf("expected 0 for no messages, got %d", id)
	}

	// Create messages
	db.CreateTopicMessage(topic.ID, user.ID, "user", "first", "first")
	msg2, _ := db.CreateTopicMessage(topic.ID, user.ID, "user", "second", "second")

	id, err = db.GetLatestTopicMessageID(topic.ID)
	if err != nil {
		t.Fatalf("GetLatestTopicMessageID failed: %v", err)
	}
	if id != msg2.ID {
		t.Errorf("expected %d, got %d", msg2.ID, id)
	}
}
