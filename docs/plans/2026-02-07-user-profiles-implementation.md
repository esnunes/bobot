# User Profiles Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a CLI subcommand that extracts user profiles from messages via LLM, and inject those profiles into the assistant's system prompt.

**Architecture:** New `user_profiles` table in core.db, new DB methods on CoreDB, a `ProfileProvider` interface in the assistant package, and a `profiles.go` orchestration file in package main. The engine injects profile text into the system prompt for private chats.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Anthropic LLM API

---

### Task 1: Database Migration & Profile Table

**Files:**
- Modify: `db/core.go:107-326` (migrate function)

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestCoreDB_UserProfilesTableExists -v`
Expected: FAIL with "user_profiles table not found"

**Step 3: Write minimal implementation**

Add to the `migrate()` method in `db/core.go`, after the `idx_topics_name_active` index creation (around line 324), before `return nil`:

```go
	// Create user_profiles table
	_, err = c.db.Exec(`
		CREATE TABLE IF NOT EXISTS user_profiles (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			content TEXT NOT NULL DEFAULT '',
			last_message_id INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestCoreDB_UserProfilesTableExists -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add user_profiles table migration"
```

---

### Task 2: GetUserProfile DB Method

**Files:**
- Modify: `db/core.go` (add method after line 1284)
- Modify: `db/core_test.go` (add test)

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestCoreDB_GetUserProfile_Empty -v`
Expected: FAIL - method does not exist

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
// GetUserProfile returns the profile content and last processed message ID for a user.
// Returns empty content and 0 if no profile exists yet.
func (c *CoreDB) GetUserProfile(userID int64) (string, int64, error) {
	var content string
	var lastMessageID int64
	err := c.db.QueryRow(
		"SELECT content, last_message_id FROM user_profiles WHERE user_id = ?",
		userID,
	).Scan(&content, &lastMessageID)

	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, err
	}
	return content, lastMessageID, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestCoreDB_GetUserProfile_Empty -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add GetUserProfile method"
```

---

### Task 3: UpsertUserProfile DB Method

**Files:**
- Modify: `db/core.go` (add method)
- Modify: `db/core_test.go` (add test)

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestCoreDB_UpsertUserProfile -v`
Expected: FAIL - method does not exist

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
// UpsertUserProfile inserts or replaces a user's profile.
func (c *CoreDB) UpsertUserProfile(userID int64, content string, lastMessageID int64) error {
	_, err := c.db.Exec(
		"INSERT INTO user_profiles (user_id, content, last_message_id, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(user_id) DO UPDATE SET content = excluded.content, last_message_id = excluded.last_message_id, updated_at = CURRENT_TIMESTAMP",
		userID, content, lastMessageID,
	)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestCoreDB_UpsertUserProfile -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add UpsertUserProfile method"
```

---

### Task 4: GetUserMessagesSince DB Method

**Files:**
- Modify: `db/core.go` (add method)
- Modify: `db/core_test.go` (add test)

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
func TestCoreDB_GetUserMessagesSince(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user, _ := db.CreateUser("msguser", "hash")

	// Create mixed messages
	msg1, _ := db.CreateMessage(user.ID, BobotUserID, "user", "Hello")        // user msg
	db.CreateMessage(BobotUserID, user.ID, "assistant", "Hi!")                  // assistant msg
	msg3, _ := db.CreateMessage(user.ID, BobotUserID, "user", "How are you?") // user msg

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestCoreDB_GetUserMessagesSince -v`
Expected: FAIL - method does not exist

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
// GetUserMessagesSince returns user-role private messages sent by a user since a given message ID.
func (c *CoreDB) GetUserMessagesSince(userID int64, sinceMessageID int64) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE sender_id = ? AND role = 'user' AND topic_id IS NULL AND id > ?
		ORDER BY id ASC
	`, userID, sinceMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return c.scanMessages(rows)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestCoreDB_GetUserMessagesSince -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add GetUserMessagesSince method"
```

---

### Task 5: ListActiveUsers DB Method

**Files:**
- Modify: `db/core.go` (add method)
- Modify: `db/core_test.go` (add test)

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestCoreDB_ListActiveUsers -v`
Expected: FAIL - method does not exist

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
// ListActiveUsers returns all non-blocked, non-system users.
func (c *CoreDB) ListActiveUsers() ([]User, error) {
	rows, err := c.db.Query(`
		SELECT id, username, password_hash, display_name, role, blocked, created_at
		FROM users
		WHERE id != 0 AND blocked = 0
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		var blocked int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Role, &blocked, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Blocked = blocked == 1
		users = append(users, u)
	}
	return users, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestCoreDB_ListActiveUsers -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add ListActiveUsers method"
```

---

### Task 6: ProfileProvider Interface & Engine Integration

**Files:**
- Modify: `assistant/engine.go:14-39` (add interface, update Engine struct and constructor)
- Modify: `assistant/engine_test.go` (update tests)

**Step 1: Write the failing test**

Add to `assistant/engine_test.go`:

```go
type mockProfileProvider struct {
	profiles map[int64]string
}

func (m *mockProfileProvider) GetUserProfile(userID int64) (string, int64, error) {
	content, ok := m.profiles[userID]
	if !ok {
		return "", 0, nil
	}
	return content, 0, nil
}

func TestEngine_Chat_InjectsProfile(t *testing.T) {
	var capturedSystemPrompt string
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "Hello Eduardo!"}, nil
		},
	}

	mockCtxProvider := &mockContextProvider{messages: nil}
	mockProfile := &mockProfileProvider{
		profiles: map[int64]string{
			1: "Eduardo lives in Berlin. Prefers concise responses.",
		},
	}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, mockProfile)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "Eduardo lives in Berlin") {
		t.Error("expected system prompt to contain user profile")
	}
	if !strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to contain <user-profile> tags")
	}
}

func TestEngine_Chat_NoProfileNoInjection(t *testing.T) {
	var capturedSystemPrompt string
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "Hello!"}, nil
		},
	}

	mockCtxProvider := &mockContextProvider{messages: nil}
	mockProfile := &mockProfileProvider{profiles: map[int64]string{}}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, mockProfile)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to NOT contain profile tags when profile is empty")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant/ -run TestEngine_Chat_InjectsProfile -v`
Expected: FAIL - `NewEngine` has wrong number of arguments

**Step 3: Write minimal implementation**

Update `assistant/engine.go`:

1. Add the `ProfileProvider` interface after `ContextProvider`:

```go
// ProfileProvider retrieves user profile data.
type ProfileProvider interface {
	GetUserProfile(userID int64) (string, int64, error)
}
```

2. Update `Engine` struct to add field:

```go
type Engine struct {
	provider        llm.Provider
	registry        *tools.Registry
	skills          []Skill
	contextProvider ContextProvider
	profileProvider ProfileProvider
}
```

3. Update `NewEngine` to accept the new parameter:

```go
func NewEngine(provider llm.Provider, registry *tools.Registry, skills []Skill, contextProvider ContextProvider, profileProvider ProfileProvider) *Engine {
	return &Engine{
		provider:        provider,
		registry:        registry,
		skills:          skills,
		contextProvider: contextProvider,
		profileProvider: profileProvider,
	}
}
```

4. Update `Chat` method to inject profile into system prompt (after building the base system prompt, before the LLM loop):

```go
	// Inject user profile if available
	if e.profileProvider != nil {
		profileContent, _, err := e.profileProvider.GetUserProfile(userData.UserID)
		if err == nil && profileContent != "" {
			systemPrompt += "\n\n## User Profile\nThe following is known about the user you are chatting with:\n<user-profile>\n" + profileContent + "\n</user-profile>"
		}
	}
```

**Step 4: Fix existing tests - update all `NewEngine` calls**

Update existing tests in `assistant/engine_test.go` to pass `nil` as the last argument:

- `TestEngine_Chat_SimpleResponse`: `NewEngine(mockProvider, registry, nil, mockCtxProvider, nil)`
- `TestEngine_Chat_WithToolUse`: `NewEngine(mockProvider, registry, nil, mockCtxProvider, nil)`
- `TestEngine_ChatWithContext`: `NewEngine(mockProv, registry, nil, mockCtxProvider, nil)`
- `TestEngine_ChatWithConversation`: `NewEngine(mockProv, registry, nil, mockCtxProvider, nil)`

Also add `"strings"` to the imports in `engine_test.go`.

**Step 5: Run all tests to verify they pass**

Run: `go test ./assistant/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go
git commit -m "feat(assistant): add ProfileProvider interface and inject profile into system prompt"
```

---

### Task 7: Wire ProfileProvider in main.go and context/adapter.go

**Files:**
- Modify: `context/adapter.go` (add ProfileProvider method)
- Modify: `context/adapter_test.go` (add test)
- Modify: `main.go:81` (pass profile provider to NewEngine)

**Step 1: Write the failing test**

Add to `context/adapter_test.go`:

```go
func TestCoreDBAdapter_ImplementsProfileProvider(t *testing.T) {
	// Compile-time check that CoreDBAdapter implements ProfileProvider
	var _ assistant.ProfileProvider = (*context.CoreDBAdapter)(nil)
}
```

Note: This is a compile-time check. If `CoreDBAdapter` doesn't implement `ProfileProvider`, the test file won't compile.

**Step 2: Run test to verify it fails**

Run: `go test ./context/ -v`
Expected: FAIL - compilation error, `CoreDBAdapter` does not implement `ProfileProvider`

**Step 3: Write minimal implementation**

Add to `context/adapter.go`:

```go
// Compile-time check that CoreDBAdapter implements ProfileProvider.
var _ assistant.ProfileProvider = (*CoreDBAdapter)(nil)

// GetUserProfile returns the profile content and last message ID for a user.
func (a *CoreDBAdapter) GetUserProfile(userID int64) (string, int64, error) {
	return a.db.GetUserProfile(userID)
}
```

Update `main.go` line 81 to pass contextAdapter as profile provider:

```go
engine := assistant.NewEngine(llmProvider, registry, loadedSkills, contextAdapter, contextAdapter)
```

**Step 4: Run tests to verify they pass**

Run: `go test ./context/ -v && go test ./assistant/ -v`
Expected: PASS

**Step 5: Also fix any other callers of NewEngine**

Check if `server/` tests create engines directly. If they do, update those calls too.

Run: `go test ./... 2>&1 | head -50`
Fix any compilation errors from the new `NewEngine` signature.

**Step 6: Commit**

```bash
git add context/adapter.go main.go
git commit -m "feat: wire ProfileProvider through context adapter to engine"
```

---

### Task 8: Create profiles.go - Update Profiles Subcommand

**Files:**
- Create: `profiles.go` (package main)

**Step 1: Write the implementation**

Create `profiles.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
)

const profileSystemPrompt = `You are a profile extraction assistant. Given a user's current profile (which may be empty) and their recent messages, produce an updated profile summary.

Extract and maintain:
- Personal details: name, location, timezone, language, job/role, company
- Preferences: communication style, response format preferences, interests, hobbies, topics they care about

Rules:
- Write in third person, concise natural language
- Preserve existing information unless explicitly contradicted
- Only add information the user has clearly stated or implied
- If the current profile is empty, create one from scratch
- Do not invent or assume information
- Output ONLY the updated profile text, nothing else`

func runUpdateProfiles(cfg *config.Config, coreDB *db.CoreDB) {
	llmProvider := llm.NewAnthropicClient(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Model)

	users, err := coreDB.ListActiveUsers()
	if err != nil {
		log.Fatalf("Failed to list users: %v", err)
	}

	log.Printf("Processing %d users...", len(users))

	for _, user := range users {
		profile, lastMsgID, err := coreDB.GetUserProfile(user.ID)
		if err != nil {
			log.Printf("Error getting profile for user %s: %v", user.Username, err)
			continue
		}

		messages, err := coreDB.GetUserMessagesSince(user.ID, lastMsgID)
		if err != nil {
			log.Printf("Error getting messages for user %s: %v", user.Username, err)
			continue
		}

		if len(messages) == 0 {
			log.Printf("Skipping %s: no new messages", user.Username)
			continue
		}

		log.Printf("Processing %s: %d new messages since message ID %d", user.Username, len(messages), lastMsgID)

		// Build user message
		profileText := profile
		if profileText == "" {
			profileText = "No profile yet."
		}

		var msgLines []string
		for _, m := range messages {
			msgLines = append(msgLines, m.Content)
		}

		userMessage := fmt.Sprintf("Current profile:\n<profile>\n%s\n</profile>\n\nNew messages:\n<messages>\n%s\n</messages>", profileText, strings.Join(msgLines, "\n"))

		resp, err := llmProvider.Chat(context.Background(), &llm.ChatRequest{
			SystemPrompt: profileSystemPrompt,
			Messages: []llm.Message{
				{Role: "user", Content: userMessage},
			},
		})
		if err != nil {
			log.Printf("LLM error for user %s: %v", user.Username, err)
			continue
		}

		newLastMsgID := messages[len(messages)-1].ID
		err = coreDB.UpsertUserProfile(user.ID, resp.Content, newLastMsgID)
		if err != nil {
			log.Printf("Error saving profile for user %s: %v", user.Username, err)
			continue
		}

		log.Printf("Updated profile for %s (last_message_id: %d)", user.Username, newLastMsgID)
	}

	log.Println("Done.")
}
```

**Step 2: Run build to verify it compiles**

Run: `go build .`
Expected: SUCCESS (no errors)

**Step 3: Commit**

```bash
git add profiles.go
git commit -m "feat: add runUpdateProfiles orchestration function"
```

---

### Task 9: Add CLI Routing in main.go

**Files:**
- Modify: `main.go` (add os.Args routing and "os" import)

**Step 1: Write the implementation**

Add `"os"` to the imports in `main.go`.

Add the subcommand routing after the coreDB initialization and initial user setup (after line 60), before the tool registry initialization:

```go
	// Handle subcommands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "update-profiles":
			runUpdateProfiles(cfg, coreDB)
			return
		default:
			log.Fatalf("Unknown command: %s", os.Args[1])
		}
	}
```

**Step 2: Run build to verify it compiles**

Run: `go build .`
Expected: SUCCESS

**Step 3: Run all tests**

Run: `go test ./...`
Expected: ALL PASS (except the pre-existing `TestCreateTopic` failure in `server/`)

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: add CLI subcommand routing for update-profiles"
```

---

### Task 10: Final Integration Test

**Step 1: Run the full test suite**

Run: `go test ./... -v`
Expected: All tests pass (except pre-existing `TestCreateTopic` in `server/`)

**Step 2: Verify build produces working binary**

Run: `go build -o bin/bobot .`
Expected: Binary built successfully

**Step 3: Verify subcommand help (unknown command)**

Run: `./bin/bobot unknown-command 2>&1`
Expected: "Unknown command: unknown-command"

**Step 4: Commit (if any final fixes needed)**

If all good, no extra commit needed.
