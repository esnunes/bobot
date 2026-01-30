# Message Persistence and Cross-Device Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable LLM context from recent messages, infinite scroll message history, and cross-device synchronization.

**Architecture:** Add `tokens` and `context_tokens` columns to messages table for chunk-based context management. Create ConnectionRegistry for multi-device WebSocket broadcast. Add REST endpoints for history pagination and reconnect sync.

**Tech Stack:** Go, SQLite, Gorilla WebSocket, vanilla JavaScript

---

## Task 1: Add Configuration for Context and History Limits

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`

**Step 1: Write the failing test**

Add to `config/config_test.go`:

```go
func TestLoad_ContextConfig_Defaults(t *testing.T) {
	// Set required env vars
	os.Setenv("BOBOT_LLM_BASE_URL", "http://test")
	os.Setenv("BOBOT_LLM_API_KEY", "key")
	os.Setenv("BOBOT_LLM_MODEL", "model")
	os.Setenv("BOBOT_JWT_SECRET", "secret")
	defer func() {
		os.Unsetenv("BOBOT_LLM_BASE_URL")
		os.Unsetenv("BOBOT_LLM_API_KEY")
		os.Unsetenv("BOBOT_LLM_MODEL")
		os.Unsetenv("BOBOT_JWT_SECRET")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Context.TokensStart != 30000 {
		t.Errorf("expected TokensStart 30000, got %d", cfg.Context.TokensStart)
	}
	if cfg.Context.TokensMax != 80000 {
		t.Errorf("expected TokensMax 80000, got %d", cfg.Context.TokensMax)
	}
	if cfg.History.DefaultLimit != 50 {
		t.Errorf("expected DefaultLimit 50, got %d", cfg.History.DefaultLimit)
	}
	if cfg.History.MaxLimit != 100 {
		t.Errorf("expected MaxLimit 100, got %d", cfg.History.MaxLimit)
	}
	if cfg.Sync.MaxLookback != 24*time.Hour {
		t.Errorf("expected MaxLookback 24h, got %v", cfg.Sync.MaxLookback)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./config -run TestLoad_ContextConfig_Defaults -v`
Expected: FAIL with "cfg.Context undefined"

**Step 3: Write minimal implementation**

In `config/config.go`, add new config structs and fields:

```go
type ContextConfig struct {
	TokensStart int
	TokensMax   int
}

type HistoryConfig struct {
	DefaultLimit int
	MaxLimit     int
}

type SyncConfig struct {
	MaxLookback time.Duration
}
```

Add to `Config` struct:

```go
type Config struct {
	Server   ServerConfig
	LLM      LLMConfig
	JWT      JWTConfig
	Context  ContextConfig
	History  HistoryConfig
	Sync     SyncConfig
	DataDir  string
	InitUser string
	InitPass string
}
```

Update `Load()` to populate:

```go
Context: ContextConfig{
	TokensStart: getEnvIntOrDefault("BOBOT_CONTEXT_TOKENS_START", 30000),
	TokensMax:   getEnvIntOrDefault("BOBOT_CONTEXT_TOKENS_MAX", 80000),
},
History: HistoryConfig{
	DefaultLimit: getEnvIntOrDefault("BOBOT_HISTORY_DEFAULT_LIMIT", 50),
	MaxLimit:     getEnvIntOrDefault("BOBOT_HISTORY_MAX_LIMIT", 100),
},
Sync: SyncConfig{
	MaxLookback: getEnvDurationOrDefault("BOBOT_SYNC_MAX_LOOKBACK", 24*time.Hour),
},
```

Add helper function:

```go
func getEnvDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}
```

Add import for `time` package.

**Step 4: Run test to verify it passes**

Run: `go test ./config -run TestLoad_ContextConfig_Defaults -v`
Expected: PASS

**Step 5: Commit**

```bash
git add config/config.go config/config_test.go
git commit -m "feat(config): add context, history, and sync configuration"
```

---

## Task 2: Add tokens and context_tokens Columns to Messages Table

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_MessageTokenColumns -v`
Expected: FAIL with "no such column: tokens"

**Step 3: Write minimal implementation**

Update the `migrate()` function in `db/core.go`. Modify the messages table schema:

```go
CREATE TABLE IF NOT EXISTS messages (
	id INTEGER PRIMARY KEY,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	tokens INTEGER NOT NULL DEFAULT 0,
	context_tokens INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_messages_user_context
ON messages(user_id, id) WHERE context_tokens = 0;
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestCoreDB_MessageTokenColumns -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add tokens and context_tokens columns to messages"
```

---

## Task 3: Update Message Struct and CreateMessage Function

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_CreateMessageWithTokens -v`
Expected: FAIL with "db.CreateMessageWithContext undefined"

**Step 3: Write minimal implementation**

Update `Message` struct in `db/core.go`:

```go
type Message struct {
	ID            int64
	UserID        int64
	Role          string
	Content       string
	Tokens        int
	ContextTokens int
	CreatedAt     time.Time
}
```

Add new function:

```go
func (c *CoreDB) CreateMessageWithContext(userID int64, role, content string) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, userID).Scan(&prevTokens, &prevContextTokens)

	var contextTokens int
	if err == sql.ErrNoRows {
		// First message - starts a new chunk
		contextTokens = 0
	} else if err != nil {
		return nil, err
	} else {
		// Continue the chunk
		contextTokens = prevContextTokens + prevTokens + tokens
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (user_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?)",
		userID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		UserID:        userID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestCoreDB_CreateMessageWithTokens -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add CreateMessageWithContext with token tracking"
```

---

## Task 4: Implement Chunk Reset Logic

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
func TestCoreDB_ChunkReset(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer db.Close()

	user, _ := db.CreateUser("chunkuser", "hash")

	// Use small thresholds for testing: start=10, max=20
	// Create messages that will exceed threshold
	// Each "aaaa" = 4 chars = 1 token

	db.CreateMessageWithContextThreshold(user.ID, "user", "aaaa", 10, 20)         // tokens=1, ctx=0 (first)
	db.CreateMessageWithContextThreshold(user.ID, "assistant", "bbbbbbbb", 10, 20) // tokens=2, ctx=3
	db.CreateMessageWithContextThreshold(user.ID, "user", "cccccccccccc", 10, 20)  // tokens=3, ctx=6
	db.CreateMessageWithContextThreshold(user.ID, "assistant", "dddddddddddddddd", 10, 20) // tokens=4, ctx=10

	// This message would make ctx=15, but max=20, so no reset yet
	msg5, _ := db.CreateMessageWithContextThreshold(user.ID, "user", "eeeeeeeeeeeeeeeeeeee", 10, 20) // tokens=5, ctx=15

	if msg5.ContextTokens != 15 {
		t.Errorf("expected context_tokens=15, got %d", msg5.ContextTokens)
	}

	// This message would make ctx=22, exceeds max=20
	// Should reset: find msg with ctx < 10 (start threshold = max - start = 20 - 10 = 10)
	// msg3 has ctx=6 < 10, so it becomes new chunk start
	// Subtract 6 from msg3 onwards, then add new message
	msg6, _ := db.CreateMessageWithContextThreshold(user.ID, "assistant", "fffffffffffffffffffffffffff", 10, 20) // tokens=7

	// After reset: msg3 ctx=0, msg4 ctx=4, msg5 ctx=9, msg6 ctx=16
	if msg6.ContextTokens != 16 {
		t.Errorf("expected context_tokens=16 after reset, got %d", msg6.ContextTokens)
	}

	// Verify msg3 is now chunk start
	var msg3Ctx int
	db.db.QueryRow("SELECT context_tokens FROM messages WHERE user_id = ? ORDER BY id ASC LIMIT 1 OFFSET 2", user.ID).Scan(&msg3Ctx)
	if msg3Ctx != 0 {
		t.Errorf("expected msg3 context_tokens=0 (chunk start), got %d", msg3Ctx)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_ChunkReset -v`
Expected: FAIL with "db.CreateMessageWithContextThreshold undefined"

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
func (c *CoreDB) CreateMessageWithContextThreshold(userID int64, role, content string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(content) / 4

	// Get the latest message's context state
	var prevContextTokens, prevTokens int
	err := c.db.QueryRow(`
		SELECT tokens, context_tokens FROM messages
		WHERE user_id = ? ORDER BY id DESC LIMIT 1
	`, userID).Scan(&prevTokens, &prevContextTokens)

	var contextTokens int
	if err == sql.ErrNoRows {
		// First message - starts a new chunk
		contextTokens = 0
	} else if err != nil {
		return nil, err
	} else {
		// Calculate what context_tokens would be
		contextTokens = prevContextTokens + prevTokens + tokens

		// Check if we need to reset the chunk
		if contextTokens > tokensMax {
			targetThreshold := tokensMax - tokensStart

			// Find the new chunk start
			var newChunkStartID int64
			var subtractValue int
			err := c.db.QueryRow(`
				SELECT id, context_tokens FROM messages
				WHERE user_id = ? AND context_tokens < ?
				ORDER BY id DESC LIMIT 1
			`, userID, targetThreshold).Scan(&newChunkStartID, &subtractValue)

			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}

			if err == nil {
				// Slide the window
				_, err = c.db.Exec(`
					UPDATE messages SET context_tokens = context_tokens - ?
					WHERE user_id = ? AND id >= ?
				`, subtractValue, userID, newChunkStartID)
				if err != nil {
					return nil, err
				}

				// Recalculate contextTokens based on updated values
				err = c.db.QueryRow(`
					SELECT tokens, context_tokens FROM messages
					WHERE user_id = ? ORDER BY id DESC LIMIT 1
				`, userID).Scan(&prevTokens, &prevContextTokens)
				if err != nil {
					return nil, err
				}
				contextTokens = prevContextTokens + prevTokens + tokens
			}
		}
	}

	result, err := c.db.Exec(
		"INSERT INTO messages (user_id, role, content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?)",
		userID, role, content, tokens, contextTokens,
	)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &Message{
		ID:            id,
		UserID:        userID,
		Role:          role,
		Content:       content,
		Tokens:        tokens,
		ContextTokens: contextTokens,
		CreatedAt:     time.Now(),
	}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestCoreDB_ChunkReset -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): implement chunk reset logic for context window"
```

---

## Task 5: Add GetContextMessages Function

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

Add import for `strings` package in the test file.

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_GetContextMessages -v`
Expected: FAIL with "db.GetContextMessages undefined"

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
func (c *CoreDB) GetContextMessages(userID int64) ([]Message, error) {
	// Find the most recent chunk start
	var chunkStartID int64
	err := c.db.QueryRow(`
		SELECT id FROM messages
		WHERE user_id = ? AND context_tokens = 0
		ORDER BY id DESC LIMIT 1
	`, userID).Scan(&chunkStartID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, err
	}

	// Fetch all messages from chunk start to present
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND id >= ?
		ORDER BY id ASC
	`, userID, chunkStartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestCoreDB_GetContextMessages -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add GetContextMessages for LLM context retrieval"
```

---

## Task 6: Add Pagination Functions

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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

	// Create messages with small time gaps
	db.CreateMessage(user.ID, "user", "old message")
	time.Sleep(10 * time.Millisecond)

	since := time.Now()
	time.Sleep(10 * time.Millisecond)

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
```

Add import for `fmt` package in the test file.

**Step 2: Run test to verify it fails**

Run: `go test ./db -run "TestCoreDB_GetMessages(Before|Since)" -v`
Expected: FAIL with undefined functions

**Step 3: Write minimal implementation**

Add to `db/core.go`:

```go
func (c *CoreDB) GetMessagesBefore(userID, beforeID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND id < ?
		ORDER BY id DESC
		LIMIT ?
	`, userID, beforeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (c *CoreDB) GetMessagesSince(userID int64, since time.Time) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ? AND created_at > ?
		ORDER BY id ASC
	`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run "TestCoreDB_GetMessages(Before|Since)" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): add GetMessagesBefore and GetMessagesSince for pagination"
```

---

## Task 7: Update Existing GetMessages and GetRecentMessages to Include New Columns

**Files:**
- Modify: `db/core.go`
- Modify: `db/core_test.go`

**Step 1: Write the failing test**

Add to `db/core_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db -run TestCoreDB_GetRecentMessagesIncludesTokens -v`
Expected: FAIL (tokens field not being scanned)

**Step 3: Write minimal implementation**

Update `GetMessages` in `db/core.go`:

```go
func (c *CoreDB) GetMessages(userID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at
		FROM messages
		WHERE user_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

Update `GetRecentMessages` in `db/core.go`:

```go
func (c *CoreDB) GetRecentMessages(userID int64, limit int) ([]Message, error) {
	rows, err := c.db.Query(`
		SELECT id, user_id, role, content, tokens, context_tokens, created_at FROM (
			SELECT id, user_id, role, content, tokens, context_tokens, created_at
			FROM messages
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ?
		) ORDER BY created_at ASC
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./db -run TestCoreDB_GetRecentMessagesIncludesTokens -v`
Expected: PASS

**Step 5: Run all db tests**

Run: `go test ./db -v`
Expected: All tests pass

**Step 6: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat(db): update GetMessages and GetRecentMessages to include token columns"
```

---

## Task 8: Create ConnectionRegistry for Multi-Device WebSocket

**Files:**
- Create: `server/connections.go`
- Create: `server/connections_test.go`

**Step 1: Write the failing test**

Create `server/connections_test.go`:

```go
// server/connections_test.go
package server

import (
	"sync"
	"testing"
)

type mockConn struct {
	id       int
	messages [][]byte
	mu       sync.Mutex
	closed   bool
}

func (m *mockConn) WriteMessage(messageType int, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.messages = append(m.messages, data)
	return nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func TestConnectionRegistry_AddRemove(t *testing.T) {
	registry := NewConnectionRegistry()

	conn1 := &mockConn{id: 1}
	conn2 := &mockConn{id: 2}

	registry.Add(1, conn1)
	registry.Add(1, conn2)
	registry.Add(2, conn1) // Different user

	if registry.Count(1) != 2 {
		t.Errorf("expected 2 connections for user 1, got %d", registry.Count(1))
	}
	if registry.Count(2) != 1 {
		t.Errorf("expected 1 connection for user 2, got %d", registry.Count(2))
	}

	registry.Remove(1, conn1)
	if registry.Count(1) != 1 {
		t.Errorf("expected 1 connection after remove, got %d", registry.Count(1))
	}
}

func TestConnectionRegistry_Broadcast(t *testing.T) {
	registry := NewConnectionRegistry()

	conn1 := &mockConn{id: 1}
	conn2 := &mockConn{id: 2}
	conn3 := &mockConn{id: 3} // Different user

	registry.Add(1, conn1)
	registry.Add(1, conn2)
	registry.Add(2, conn3)

	registry.Broadcast(1, []byte("hello"))

	if len(conn1.messages) != 1 {
		t.Errorf("conn1 should have 1 message, got %d", len(conn1.messages))
	}
	if len(conn2.messages) != 1 {
		t.Errorf("conn2 should have 1 message, got %d", len(conn2.messages))
	}
	if len(conn3.messages) != 0 {
		t.Errorf("conn3 should have 0 messages, got %d", len(conn3.messages))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestConnectionRegistry -v`
Expected: FAIL with "NewConnectionRegistry undefined"

**Step 3: Write minimal implementation**

Create `server/connections.go`:

```go
// server/connections.go
package server

import (
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketWriter is the interface for writing to a WebSocket connection.
// This allows for easier testing with mock connections.
type WebSocketWriter interface {
	WriteMessage(messageType int, data []byte) error
	Close() error
}

// ConnectionRegistry manages WebSocket connections per user for multi-device support.
type ConnectionRegistry struct {
	mu    sync.RWMutex
	conns map[int64][]WebSocketWriter
}

// NewConnectionRegistry creates a new ConnectionRegistry.
func NewConnectionRegistry() *ConnectionRegistry {
	return &ConnectionRegistry{
		conns: make(map[int64][]WebSocketWriter),
	}
}

// Add registers a connection for a user.
func (r *ConnectionRegistry) Add(userID int64, conn WebSocketWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[userID] = append(r.conns[userID], conn)
}

// Remove unregisters a connection for a user.
func (r *ConnectionRegistry) Remove(userID int64, conn WebSocketWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	conns := r.conns[userID]
	for i, c := range conns {
		if c == conn {
			r.conns[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	if len(r.conns[userID]) == 0 {
		delete(r.conns, userID)
	}
}

// Broadcast sends a message to all connections for a user.
func (r *ConnectionRegistry) Broadcast(userID int64, data []byte) {
	r.mu.RLock()
	conns := r.conns[userID]
	r.mu.RUnlock()

	for _, conn := range conns {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

// Count returns the number of connections for a user.
func (r *ConnectionRegistry) Count(userID int64) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.conns[userID])
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestConnectionRegistry -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/connections.go server/connections_test.go
git commit -m "feat(server): add ConnectionRegistry for multi-device WebSocket"
```

---

## Task 9: Create REST Endpoints for Messages

**Files:**
- Create: `server/messages.go`
- Create: `server/messages_test.go`

**Step 1: Write the failing test**

Create `server/messages_test.go`:

```go
// server/messages_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func TestHandleRecentMessages(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "testsecret"},
		History: config.HistoryConfig{
			DefaultLimit: 50,
			MaxLimit:     100,
		},
	}
	jwt := auth.NewJWTService(cfg.JWT.Secret)

	user, _ := coreDB.CreateUser("testuser", "hash")
	coreDB.CreateMessage(user.ID, "user", "Hello")
	coreDB.CreateMessage(user.ID, "assistant", "Hi there")

	srv := New(cfg, coreDB, jwt)

	// Create request with auth context
	req := httptest.NewRequest("GET", "/api/messages/recent?limit=10", nil)
	req = req.WithContext(auth.ContextWithUserID(req.Context(), user.ID))
	rec := httptest.NewRecorder()

	srv.handleRecentMessages(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var messages []db.Message
	json.NewDecoder(rec.Body).Decode(&messages)

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}

func TestHandleMessageHistory(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "testsecret"},
		History: config.HistoryConfig{
			DefaultLimit: 50,
			MaxLimit:     100,
		},
	}
	jwt := auth.NewJWTService(cfg.JWT.Secret)

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create 5 messages
	var lastID int64
	for i := 0; i < 5; i++ {
		msg, _ := coreDB.CreateMessage(user.ID, "user", "msg")
		lastID = msg.ID
	}

	srv := New(cfg, coreDB, jwt)

	req := httptest.NewRequest("GET", "/api/messages/history?before="+string(rune(lastID+'0'))+"&limit=2", nil)
	req = req.WithContext(auth.ContextWithUserID(req.Context(), user.ID))
	rec := httptest.NewRecorder()

	// Use proper URL parsing for the test
	req2 := httptest.NewRequest("GET", "/api/messages/history?before=5&limit=2", nil)
	req2 = req2.WithContext(auth.ContextWithUserID(req2.Context(), user.ID))
	rec2 := httptest.NewRecorder()

	srv.handleMessageHistory(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleMessageSync(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "testsecret"},
		Sync: config.SyncConfig{
			MaxLookback: 24 * time.Hour,
		},
	}
	jwt := auth.NewJWTService(cfg.JWT.Secret)

	user, _ := coreDB.CreateUser("testuser", "hash")
	coreDB.CreateMessage(user.ID, "user", "old message")

	since := time.Now().Format(time.RFC3339)
	time.Sleep(10 * time.Millisecond)

	coreDB.CreateMessage(user.ID, "assistant", "new message")

	srv := New(cfg, coreDB, jwt)

	req := httptest.NewRequest("GET", "/api/messages/sync?since="+since, nil)
	req = req.WithContext(auth.ContextWithUserID(req.Context(), user.ID))
	rec := httptest.NewRecorder()

	srv.handleMessageSync(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var messages []db.Message
	json.NewDecoder(rec.Body).Decode(&messages)

	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./server -run "TestHandle(Recent|Message)" -v`
Expected: FAIL with undefined handlers

**Step 3: Write minimal implementation**

Create `server/messages.go`:

```go
// server/messages.go
package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/esnunes/bobot/auth"
)

func (s *Server) handleRecentMessages(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > s.cfg.History.MaxLimit {
		limit = s.cfg.History.DefaultLimit
	}

	messages, err := s.db.GetRecentMessages(userID, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleMessageHistory(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	beforeID, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if beforeID <= 0 {
		http.Error(w, "invalid before parameter", http.StatusBadRequest)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > s.cfg.History.MaxLimit {
		limit = s.cfg.History.DefaultLimit
	}

	messages, err := s.db.GetMessagesBefore(userID, beforeID, limit)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleMessageSync(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	if userID == 0 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	sinceStr := r.URL.Query().Get("since")
	if sinceStr == "" {
		http.Error(w, "missing since parameter", http.StatusBadRequest)
		return
	}

	since, err := time.Parse(time.RFC3339, sinceStr)
	if err != nil {
		http.Error(w, "invalid since parameter", http.StatusBadRequest)
		return
	}

	// Apply max lookback
	minTime := time.Now().Add(-s.cfg.Sync.MaxLookback)
	if since.Before(minTime) {
		since = minTime
	}

	messages, err := s.db.GetMessagesSince(userID, since)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run "TestHandle(Recent|Message)" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/messages.go server/messages_test.go
git commit -m "feat(server): add REST endpoints for message history and sync"
```

---

## Task 10: Register Message Routes and Add Auth Middleware

**Files:**
- Modify: `server/server.go`
- Modify: `server/server_test.go`

**Step 1: Write the failing test**

Add to `server/server_test.go`:

```go
func TestMessageEndpointsRequireAuth(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "testsecret"},
		History: config.HistoryConfig{
			DefaultLimit: 50,
			MaxLimit:     100,
		},
		Sync: config.SyncConfig{
			MaxLookback: 24 * time.Hour,
		},
	}
	jwt := auth.NewJWTService(cfg.JWT.Secret)
	srv := New(cfg, coreDB, jwt)

	endpoints := []string{
		"/api/messages/recent",
		"/api/messages/history?before=1",
		"/api/messages/sync?since=2020-01-01T00:00:00Z",
	}

	for _, endpoint := range endpoints {
		req := httptest.NewRequest("GET", endpoint, nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", endpoint, rec.Code)
		}
	}
}
```

Add imports for `time` and other needed packages.

**Step 2: Run test to verify it fails**

Run: `go test ./server -run TestMessageEndpointsRequireAuth -v`
Expected: FAIL (404 - routes not registered)

**Step 3: Write minimal implementation**

Update `routes()` in `server/server.go`:

```go
func (s *Server) routes() {
	// API routes
	s.router.HandleFunc("GET /health", s.handleHealth)
	s.router.HandleFunc("POST /api/login", s.handleLogin)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/logout", s.handleLogout)
	s.router.HandleFunc("GET /ws/chat", s.handleChat)

	// Message routes (require auth)
	s.router.HandleFunc("GET /api/messages/recent", s.authMiddleware(s.handleRecentMessages))
	s.router.HandleFunc("GET /api/messages/history", s.authMiddleware(s.handleMessageHistory))
	s.router.HandleFunc("GET /api/messages/sync", s.authMiddleware(s.handleMessageSync))

	// Page routes
	s.router.HandleFunc("GET /", s.handleLoginPage)
	s.router.HandleFunc("GET /chat", s.handleChatPage)

	// Static files
	staticFS, _ := fs.Sub(web.FS, "static")
	s.router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || len(authHeader) < 8 || authHeader[:7] != "Bearer " {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := authHeader[7:]
		claims, err := s.jwt.ValidateAccessToken(token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := auth.ContextWithUserID(r.Context(), claims.UserID)
		next(w, r.WithContext(ctx))
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./server -run TestMessageEndpointsRequireAuth -v`
Expected: PASS

**Step 5: Commit**

```bash
git add server/server.go server/server_test.go
git commit -m "feat(server): register message endpoints with auth middleware"
```

---

## Task 11: Integrate Context-Aware Message Creation in Chat Handler

**Files:**
- Modify: `server/chat.go`
- Modify: `server/server.go`

**Step 1: Update Server struct to include config**

The Server struct already has `cfg`, so we can use it. Update `chat.go` to use `CreateMessageWithContextThreshold`.

**Step 2: Write the implementation**

Update `server/chat.go`:

```go
// server/chat.go
package server

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/auth"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

type chatMessage struct {
	Content string `json:"content"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	// Get token from query param
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := s.jwt.ValidateAccessToken(token)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Create context with user ID
	ctx := auth.ContextWithUserID(r.Context(), claims.UserID)

	// Handle messages
	for {
		var msg chatMessage
		if err := conn.ReadJSON(&msg); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("websocket error: %v", err)
			}
			break
		}

		// Save user message with context tracking
		s.db.CreateMessageWithContextThreshold(
			claims.UserID, "user", msg.Content,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Get assistant response
		response, err := s.engine.Chat(ctx, msg.Content)
		if err != nil {
			log.Printf("assistant error: %v", err)
			response = "Sorry, I encountered an error. Please try again."
		}

		// Save assistant message with context tracking
		s.db.CreateMessageWithContextThreshold(
			claims.UserID, "assistant", response,
			s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
		)

		// Send response
		if err := conn.WriteJSON(chatMessage{Content: response}); err != nil {
			log.Printf("websocket write error: %v", err)
			break
		}
	}
}
```

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

**Step 4: Commit**

```bash
git add server/chat.go
git commit -m "feat(server): use context-aware message creation in chat handler"
```

---

## Task 12: Integrate LLM Context in Assistant Engine

**Files:**
- Modify: `assistant/engine.go`
- Modify: `assistant/engine_test.go`

**Step 1: Write the failing test**

Add to `assistant/engine_test.go`:

```go
func TestEngine_ChatWithContext(t *testing.T) {
	// Create a mock provider that captures the messages sent
	var capturedMessages []llm.Message
	mockProvider := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedMessages = req.Messages
			return &llm.ChatResponse{Content: "response"}, nil
		},
	}

	// Create mock DB with context messages
	mockDB := &mockContextDB{
		messages: []db.Message{
			{Role: "user", Content: "previous question"},
			{Role: "assistant", Content: "previous answer"},
		},
	}

	engine := NewEngineWithContext(mockProvider, nil, nil, mockDB)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	_, err := engine.Chat(ctx, "new question")
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	// Should have 3 messages: 2 from context + 1 new
	if len(capturedMessages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(capturedMessages))
	}

	// Last message should be the new question
	if capturedMessages[2].Content != "new question" {
		t.Errorf("expected last message to be 'new question', got %v", capturedMessages[2].Content)
	}
}
```

This requires defining interfaces and mocks, which we'll add in the implementation.

**Step 2: Run test to verify it fails**

Run: `go test ./assistant -run TestEngine_ChatWithContext -v`
Expected: FAIL with undefined types

**Step 3: Write minimal implementation**

Update `assistant/engine.go`:

```go
// assistant/engine.go
package assistant

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

// ContextProvider retrieves context messages for a user.
type ContextProvider interface {
	GetContextMessages(userID int64) ([]ContextMessage, error)
}

// ContextMessage represents a message for context (simplified from db.Message).
type ContextMessage struct {
	Role    string
	Content string
}

type Engine struct {
	provider        llm.Provider
	registry        *tools.Registry
	skills          []Skill
	contextProvider ContextProvider
}

func NewEngine(provider llm.Provider, registry *tools.Registry, skills []Skill) *Engine {
	return &Engine{
		provider: provider,
		registry: registry,
		skills:   skills,
	}
}

func NewEngineWithContext(provider llm.Provider, registry *tools.Registry, skills []Skill, contextProvider ContextProvider) *Engine {
	return &Engine{
		provider:        provider,
		registry:        registry,
		skills:          skills,
		contextProvider: contextProvider,
	}
}

// Chat processes a user message and returns the assistant's response.
// The context must contain the user ID (set by auth middleware).
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
	// Build system prompt
	var llmTools []llm.Tool
	if e.registry != nil {
		llmTools = e.registry.ToLLMTools()
	}
	systemPrompt := BuildSystemPrompt(e.skills, llmTools)

	// Build messages with context
	var messages []llm.Message

	// Get context messages if provider is set
	if e.contextProvider != nil {
		userID := auth.UserIDFromContext(ctx)
		if userID != 0 {
			contextMsgs, err := e.contextProvider.GetContextMessages(userID)
			if err == nil {
				for _, cm := range contextMsgs {
					messages = append(messages, llm.Message{
						Role:    cm.Role,
						Content: cm.Content,
					})
				}
			}
		}
	}

	// Add the new user message
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: message,
	})

	// Loop for tool use
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        llmTools,
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// If no tool calls, return the response
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Build assistant message with tool use
		toolUseContent := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			toolUseContent = append(toolUseContent, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": tc.Input,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: toolUseContent,
		})

		// Execute tools and add results
		toolResults := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			result, err := e.registry.Execute(ctx, tc.Name, tc.Input)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			toolResults = append(toolResults, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return "", fmt.Errorf("max iterations reached")
}
```

Add the test helpers to `assistant/engine_test.go`:

```go
type mockContextDB struct {
	messages []db.Message
}

func (m *mockContextDB) GetContextMessages(userID int64) ([]assistant.ContextMessage, error) {
	var result []assistant.ContextMessage
	for _, msg := range m.messages {
		result = append(result, assistant.ContextMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./assistant -run TestEngine_ChatWithContext -v`
Expected: PASS

**Step 5: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go
git commit -m "feat(assistant): integrate LLM context from message history"
```

---

## Task 13: Create Context Adapter for CoreDB

**Files:**
- Create: `context/adapter.go`
- Create: `context/adapter_test.go`

**Step 1: Write the failing test**

Create `context/adapter_test.go`:

```go
// context/adapter_test.go
package context

import (
	"path/filepath"
	"testing"

	"github.com/esnunes/bobot/db"
)

func TestCoreDBAdapter_GetContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")
	coreDB.CreateMessageWithContext(user.ID, "user", "Hello")
	coreDB.CreateMessageWithContext(user.ID, "assistant", "Hi there")

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
```

**Step 2: Run test to verify it fails**

Run: `go test ./context -v`
Expected: FAIL with package not found

**Step 3: Write minimal implementation**

Create directory and file `context/adapter.go`:

```go
// context/adapter.go
package context

import (
	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

// CoreDBAdapter adapts CoreDB to the ContextProvider interface.
type CoreDBAdapter struct {
	db *db.CoreDB
}

// NewCoreDBAdapter creates a new adapter.
func NewCoreDBAdapter(coreDB *db.CoreDB) *CoreDBAdapter {
	return &CoreDBAdapter{db: coreDB}
}

// GetContextMessages returns context messages for a user.
func (a *CoreDBAdapter) GetContextMessages(userID int64) ([]assistant.ContextMessage, error) {
	messages, err := a.db.GetContextMessages(userID)
	if err != nil {
		return nil, err
	}

	result := make([]assistant.ContextMessage, len(messages))
	for i, m := range messages {
		result[i] = assistant.ContextMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}
	return result, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./context -v`
Expected: PASS

**Step 5: Commit**

```bash
git add context/adapter.go context/adapter_test.go
git commit -m "feat(context): add CoreDB adapter for ContextProvider interface"
```

---

## Task 14: Wire Everything Together in main.go

**Files:**
- Modify: `main.go`

**Step 1: Read current main.go**

Read the file to understand current structure.

**Step 2: Update main.go**

Update to use the new context adapter:

```go
// In main.go, update the engine creation:

// Create context adapter
contextAdapter := context.NewCoreDBAdapter(coreDB)

// Create assistant engine with context
engine := assistant.NewEngineWithContext(provider, registry, skills, contextAdapter)

// Create server with engine
srv := server.NewWithAssistant(cfg, coreDB, jwtService, engine)
```

Add import for the context package.

**Step 3: Run all tests and verify the app builds**

Run: `go build ./...`
Run: `go test ./... -v`
Expected: All pass

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: wire context adapter into main application"
```

---

## Task 15: Update Frontend for Infinite Scroll and Sync

**Files:**
- Modify: `web/static/chat.js`

**Step 1: Update chat.js with history loading and sync**

```javascript
class ChatClient {
    constructor() {
        this.ws = null;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.logoutBtn = document.getElementById('logout-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        // Load initial messages
        await this.loadRecentMessages(token);

        // Sync any missed messages
        await this.syncMessages(token);

        // Connect WebSocket
        this.connect(token);
        this.setupEventListeners();
    }

    async loadRecentMessages(token) {
        try {
            const resp = await fetch('/api/messages/recent?limit=50', {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 401) {
                    this.logout();
                    return;
                }
                throw new Error('Failed to load messages');
            }

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => this.addMessage(msg.Content, msg.Role, msg.ID, false));
                this.oldestMessageId = messages[0].ID;
                this.updateLastSeenTimestamp(messages[messages.length - 1].CreatedAt);
            }
            this.scrollToBottom();
        } catch (err) {
            console.error('Failed to load messages:', err);
        }
    }

    async loadMoreHistory() {
        if (this.isLoadingHistory || !this.hasMoreHistory || !this.oldestMessageId) {
            return;
        }

        this.isLoadingHistory = true;
        const token = localStorage.getItem('access_token');

        try {
            const resp = await fetch(`/api/messages/history?before=${this.oldestMessageId}&limit=50`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to load history');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                // Remember scroll position
                const scrollHeight = this.messagesEl.scrollHeight;
                const scrollTop = this.messagesEl.scrollTop;

                // Prepend messages (they come in DESC order, so reverse for display)
                messages.reverse().forEach(msg => this.prependMessage(msg.Content, msg.Role, msg.ID));
                this.oldestMessageId = messages[0].ID;

                // Restore scroll position
                this.messagesEl.scrollTop = this.messagesEl.scrollHeight - scrollHeight + scrollTop;
            } else {
                this.hasMoreHistory = false;
            }
        } catch (err) {
            console.error('Failed to load history:', err);
        } finally {
            this.isLoadingHistory = false;
        }
    }

    async syncMessages(token) {
        const lastSeen = localStorage.getItem('lastMessageTimestamp');
        if (!lastSeen) return;

        try {
            const resp = await fetch(`/api/messages/sync?since=${encodeURIComponent(lastSeen)}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) return;

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => {
                    // Only add if not already displayed
                    if (!document.querySelector(`[data-message-id="${msg.ID}"]`)) {
                        this.addMessage(msg.Content, msg.Role, msg.ID, false);
                    }
                });
                this.updateLastSeenTimestamp(messages[messages.length - 1].CreatedAt);
                this.scrollToBottom();
            }
        } catch (err) {
            console.error('Sync failed:', err);
        }
    }

    updateLastSeenTimestamp(timestamp) {
        localStorage.setItem('lastMessageTimestamp', timestamp);
    }

    connect(token) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('WebSocket connected');
        };

        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.removeTypingIndicator();
            this.addMessage(data.content, 'assistant');
            this.updateLastSeenTimestamp(new Date().toISOString());
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.refreshAndReconnect();
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }

    async refreshAndReconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            this.logout();
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({refresh_token: refreshToken})
            });

            if (!resp.ok) {
                throw new Error('Refresh failed');
            }

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);

            // Sync messages before reconnecting
            await this.syncMessages(data.access_token);

            // Reconnect with new token
            setTimeout(() => this.connect(data.access_token), 1000);
        } catch (err) {
            console.error('Token refresh failed:', err);
            this.logout();
        }
    }

    setupEventListeners() {
        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
        });

        this.menuBtn.addEventListener('click', () => {
            this.menuOverlay.classList.remove('hidden');
        });

        this.menuOverlay.addEventListener('click', (e) => {
            if (e.target === this.menuOverlay) {
                this.menuOverlay.classList.add('hidden');
            }
        });

        this.logoutBtn.addEventListener('click', () => {
            this.logout();
        });

        // Infinite scroll - load more when scrolling near top
        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }

        this.addMessage(content, 'user');
        this.updateLastSeenTimestamp(new Date().toISOString());
        this.showTypingIndicator();
        this.ws.send(JSON.stringify({content: content}));
        this.input.value = '';
    }

    addMessage(content, role, id = null, scroll = true) {
        const msgEl = document.createElement('div');
        msgEl.className = `message ${role}`;
        msgEl.textContent = content;
        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }
        this.messagesEl.appendChild(msgEl);
        if (scroll) {
            this.scrollToBottom();
        }
    }

    prependMessage(content, role, id = null) {
        const msgEl = document.createElement('div');
        msgEl.className = `message ${role}`;
        msgEl.textContent = content;
        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }
        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
    }

    showTypingIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'message assistant typing-indicator';
        indicator.id = 'typing-indicator';
        indicator.innerHTML = '<span></span><span></span><span></span>';
        this.messagesEl.appendChild(indicator);
        this.scrollToBottom();
    }

    removeTypingIndicator() {
        const indicator = document.getElementById('typing-indicator');
        if (indicator) {
            indicator.remove();
        }
    }

    scrollToBottom() {
        this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    async logout() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (refreshToken) {
            try {
                await fetch('/api/logout', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({refresh_token: refreshToken})
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
        }

        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');
        window.location.href = '/';
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new ChatClient();
});
```

**Step 2: Test manually in browser**

Start the app and verify:
1. Messages load on page open
2. Scrolling up loads older messages
3. Reconnecting syncs missed messages

**Step 3: Commit**

```bash
git add web/static/chat.js
git commit -m "feat(web): add infinite scroll and cross-device sync to chat UI"
```

---

## Task 16: Final Integration Test and Cleanup

**Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All pass

**Step 2: Build and test manually**

Run: `go build -o bobot-web .`
Start the app and test:
1. Login
2. Send messages
3. Scroll up to load history
4. Open in second browser tab
5. Send message in one tab, verify appears in other

**Step 3: Final commit**

```bash
git add -A
git commit -m "feat: complete message persistence and cross-device sync implementation"
```

---

## Summary

This plan implements:

1. **Configuration** - New env vars for context, history, and sync limits
2. **Database** - `tokens` and `context_tokens` columns, chunk reset logic, pagination queries
3. **Context** - Adapter pattern for LLM context retrieval
4. **Server** - ConnectionRegistry, REST endpoints for messages, auth middleware
5. **Assistant** - Context-aware chat with message history
6. **Frontend** - Infinite scroll, reconnect sync, timestamp tracking

All changes follow TDD with failing tests first, minimal implementation, and frequent commits.
