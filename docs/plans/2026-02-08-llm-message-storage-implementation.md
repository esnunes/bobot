# LLM Message Storage Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Persist full LLM message data (including tool_use/tool_result exchanges) so conversation context includes tool call history.

**Architecture:** Add `raw_content` column to messages table alongside existing `content`. The engine persists each turn during the tool loop. Context building reads `raw_content` to reconstruct exact LLM message format. Token estimation uses `raw_content` length.

**Tech Stack:** Go, SQLite, Anthropic API

---

### Task 1: Add `RawContent` field to Message struct and DB migration

**Files:**
- Modify: `db/core.go:47-57` (Message struct)
- Modify: `db/core.go:107-339` (migrate function)
- Modify: `db/core.go:594-611` (scanMessages)

**Step 1: Write the failing test**

Create file `db/core_raw_content_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestRawContentColumn -v`
Expected: FAIL — `CreatePrivateMessageWithContextThreshold` doesn't accept `rawContent` parameter

**Step 3: Implement the changes**

In `db/core.go`, add `RawContent` to the Message struct:

```go
type Message struct {
	ID            int64
	SenderID      int64
	ReceiverID    *int64
	TopicID       *int64
	Role          string
	Content       string
	RawContent    string
	Tokens        int
	ContextTokens int
	CreatedAt     time.Time
}
```

In `migrate()`, add the column migration (after the existing `addColumnIfMissing` calls for messages):

```go
// Migrate: add raw_content column to messages
if err := c.addColumnIfMissing("messages", "raw_content", "TEXT NOT NULL DEFAULT ''"); err != nil {
    return err
}
```

Update `scanMessages` to scan `raw_content`:

```go
func (c *CoreDB) scanMessages(rows *sql.Rows) ([]Message, error) {
	var messages []Message
	for rows.Next() {
		var m Message
		var receiverID, topicID sql.NullInt64
		if err := rows.Scan(&m.ID, &m.SenderID, &receiverID, &topicID, &m.Role, &m.Content, &m.RawContent, &m.Tokens, &m.ContextTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		if receiverID.Valid {
			m.ReceiverID = &receiverID.Int64
		}
		if topicID.Valid {
			m.TopicID = &topicID.Int64
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
```

Update **all** SQL SELECT queries that use `scanMessages` to include `raw_content` in the column list. These are in the following methods — each one currently selects `id, sender_id, receiver_id, topic_id, role, content, tokens, context_tokens, created_at`. Add `raw_content` after `content`:

- `GetPrivateChatMessages` (line ~579)
- `GetPrivateChatRecentMessages` (line ~617)
- `GetPrivateChatContextMessages` (line ~769)
- `GetPrivateChatMessagesBefore` (line ~787)
- `GetPrivateChatMessagesSince` (line ~806)
- `GetTopicRecentMessages` (line ~1186)
- `GetTopicMessagesBefore` (line ~1204)
- `GetTopicMessagesSince` (line ~1221)
- `GetTopicContextMessages` (line ~1328)
- `GetUserMessagesSince` (line ~991)

The column list becomes: `id, sender_id, receiver_id, topic_id, role, content, raw_content, tokens, context_tokens, created_at`

Update `CreatePrivateMessageWithContextThreshold` to accept `rawContent` parameter and use it for token estimation:

```go
func (c *CoreDB) CreatePrivateMessageWithContextThreshold(senderID, receiverID int64, role, content, rawContent string, tokensStart, tokensMax int) (*Message, error) {
	tokens := len(rawContent) / 4
	// ... existing context window logic unchanged ...

	result, err := c.db.Exec(
		"INSERT INTO messages (sender_id, receiver_id, role, content, raw_content, tokens, context_tokens) VALUES (?, ?, ?, ?, ?, ?, ?)",
		senderID, receiverID, role, content, rawContent, tokens, contextTokens,
	)
	// ... return Message with RawContent field set ...
}
```

Similarly update `CreateMessageWithContext` and `CreateMessage` to accept and store `rawContent`.

Update `CreateTopicMessage` and `CreateTopicMessageWithContext` to accept and store `rawContent`.

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestRawContentColumn -v`
Expected: PASS

**Step 5: Fix all callers that now have wrong number of arguments**

Every caller of these functions needs to pass `rawContent`. For now, callers that don't have raw content yet should pass `content` as `rawContent` (plain text messages are their own raw content). This includes:

- `server/chat.go` — all `CreatePrivateMessageWithContextThreshold` and `CreateTopicMessageWithContext` calls
- `context/adapter_test.go` — `CreateMessageWithContext` call

Run: `go test ./... -v`
Expected: All existing tests pass (except the known failing `TestCreateTopic`)

**Step 6: Commit**

```
feat(db): add raw_content column to messages table
```

---

### Task 2: Add `RawContent` to `ChatResponse` and preserve original content array

**Files:**
- Modify: `llm/provider.go:29-33` (ChatResponse struct)
- Modify: `llm/anthropic.go:126-143` (response parsing)

**Step 1: Write the failing test**

Create file `llm/anthropic_raw_content_test.go`:

```go
package llm

import (
	"encoding/json"
	"testing"
)

func TestChatResponse_RawContent_TextOnly(t *testing.T) {
	apiResp := anthropicResponse{
		Content: []anthropicContent{
			{Type: "text", Text: "Hello!"},
		},
		StopReason: "end_turn",
	}

	result := buildChatResponse(&apiResp)

	if result.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got '%s'", result.Content)
	}

	// RawContent should be the JSON array of content blocks
	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal([]byte(result.RawContent), &rawBlocks); err != nil {
		t.Fatalf("failed to parse RawContent as JSON: %v", err)
	}
	if len(rawBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(rawBlocks))
	}
	if rawBlocks[0]["type"] != "text" {
		t.Errorf("expected type 'text', got '%v'", rawBlocks[0]["type"])
	}
}

func TestChatResponse_RawContent_WithToolUse(t *testing.T) {
	apiResp := anthropicResponse{
		Content: []anthropicContent{
			{Type: "text", Text: "Let me check."},
			{Type: "tool_use", ID: "call_1", Name: "get_weather", Input: map[string]interface{}{"location": "Paris"}},
		},
		StopReason: "tool_use",
	}

	result := buildChatResponse(&apiResp)

	if result.Content != "Let me check." {
		t.Errorf("expected content 'Let me check.', got '%s'", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}

	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal([]byte(result.RawContent), &rawBlocks); err != nil {
		t.Fatalf("failed to parse RawContent as JSON: %v", err)
	}
	if len(rawBlocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(rawBlocks))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./llm/ -run TestChatResponse_RawContent -v`
Expected: FAIL — `buildChatResponse` doesn't exist, `RawContent` field doesn't exist

**Step 3: Implement the changes**

Add `RawContent` to `ChatResponse` in `llm/provider.go`:

```go
type ChatResponse struct {
	Content    string
	RawContent string // JSON-encoded content array from API response
	ToolCalls  []ToolCall
	StopType   string
}
```

Extract a `buildChatResponse` function in `llm/anthropic.go` and populate `RawContent`:

```go
func buildChatResponse(apiResp *anthropicResponse) *ChatResponse {
	result := &ChatResponse{
		StopType: apiResp.StopReason,
	}

	// Build RawContent as JSON array of content blocks
	rawBytes, _ := json.Marshal(apiResp.Content)
	result.RawContent = string(rawBytes)

	for _, content := range apiResp.Content {
		switch content.Type {
		case "text":
			result.Content += content.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:    content.ID,
				Name:  content.Name,
				Input: content.Input,
			})
		}
	}

	return result
}
```

Update `AnthropicClient.Chat()` to use `buildChatResponse`:

```go
// Replace lines 126-143 with:
return buildChatResponse(&apiResp), nil
```

**Step 4: Run test to verify it passes**

Run: `go test ./llm/ -run TestChatResponse_RawContent -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -v`
Expected: All pass (except known failing `TestCreateTopic`)

**Step 6: Commit**

```
feat(llm): add RawContent to ChatResponse for full API fidelity
```

---

### Task 3: Update `ContextMessage` to carry `RawContent` and update context adapter

**Files:**
- Modify: `assistant/engine.go:21-24` (ContextMessage struct)
- Modify: `context/adapter.go:24-38` (GetContextMessages)
- Modify: `assistant/engine.go:68-80` (context building in Chat)

**Step 1: Write the failing test**

Add to `context/adapter_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./context/ -run TestCoreDBAdapter_GetContextMessages_RawContent -v`
Expected: FAIL — `ContextMessage` has no `RawContent` field

**Step 3: Implement the changes**

Update `ContextMessage` in `assistant/engine.go`:

```go
type ContextMessage struct {
	Role       string
	Content    string
	RawContent string
}
```

Update `context/adapter.go` to populate `RawContent`:

```go
func (a *CoreDBAdapter) GetContextMessages(userID int64) ([]assistant.ContextMessage, error) {
	messages, err := a.db.GetPrivateChatContextMessages(userID)
	if err != nil {
		return nil, err
	}

	result := make([]assistant.ContextMessage, len(messages))
	for i, m := range messages {
		result[i] = assistant.ContextMessage{
			Role:       m.Role,
			Content:    m.Content,
			RawContent: m.RawContent,
		}
	}
	return result, nil
}
```

Update context building in `engine.Chat()` to use `RawContent` for LLM messages. When `RawContent` is non-empty, parse it to determine if it's a JSON array (tool blocks) or plain string:

```go
// Get context messages
contextMsgs, err := e.contextProvider.GetContextMessages(userData.UserID)
if err == nil {
    for _, cm := range contextMsgs {
        msg := llm.Message{Role: cm.Role}
        if cm.RawContent != "" {
            msg.Content = parseRawContent(cm.RawContent)
        } else {
            msg.Content = cm.Content
        }
        messages = append(messages, msg)
    }
}
```

Add helper function in `assistant/engine.go`:

```go
// parseRawContent converts stored raw_content back to the appropriate type
// for the LLM Message.Content field (string or []map[string]any).
func parseRawContent(raw string) interface{} {
	if len(raw) == 0 {
		return ""
	}
	// If it starts with '[', it's a JSON array (tool blocks)
	if raw[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			return arr
		}
	}
	// Otherwise treat as plain string — strip surrounding quotes if present
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}
```

Update `mockContextProvider` in `assistant/engine_test.go` to include `RawContent`:

```go
// No changes needed — RawContent will default to "" which triggers the cm.Content fallback
```

**Step 4: Run test to verify it passes**

Run: `go test ./context/ -run TestCoreDBAdapter_GetContextMessages_RawContent -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -v`
Expected: All pass (except known failing `TestCreateTopic`)

**Step 6: Commit**

```
feat(assistant): use raw_content for LLM context building
```

---

### Task 4: Move message persistence into engine for tool loop turns

**Files:**
- Modify: `assistant/engine.go` (add MessageSaver interface, persist turns in Chat)
- Modify: `server/chat.go:85-147` (stop saving assistant response, pass saver to engine)
- Create: `context/saver.go` (implement MessageSaver using CoreDB)
- Modify: `main.go` (wire the saver)

**Step 1: Write the failing test**

Add to `assistant/engine_test.go`:

```go
type savedMessage struct {
	SenderID   int64
	ReceiverID int64
	Role       string
	Content    string
	RawContent string
}

type mockMessageSaver struct {
	messages []savedMessage
}

func (m *mockMessageSaver) SaveMessage(senderID, receiverID int64, role, content, rawContent string) error {
	m.messages = append(m.messages, savedMessage{
		SenderID:   senderID,
		ReceiverID: receiverID,
		Role:       role,
		Content:    content,
		RawContent: rawContent,
	})
	return nil
}

func TestEngine_Chat_PersistsToolLoop(t *testing.T) {
	mockProv := &mockLLM{
		responses: []*llm.ChatResponse{
			{
				Content:    "Let me check.",
				RawContent: `[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"call_1","name":"task","input":{"command":"list"}}]`,
				StopType:   "tool_use",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "task", Input: map[string]interface{}{"command": "list"}},
				},
			},
			{
				Content:    "Here are your tasks.",
				RawContent: `[{"type":"text","text":"Here are your tasks."}]`,
				StopType:   "end_turn",
			},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk"})

	saver := &mockMessageSaver{}
	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, nil)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, "What's on my list?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are your tasks." {
		t.Errorf("unexpected result: %s", result)
	}

	// Should have saved 3 messages:
	// 1. assistant tool_use message
	// 2. user tool_result message
	// 3. assistant final response
	if len(saver.messages) != 3 {
		t.Fatalf("expected 3 saved messages, got %d", len(saver.messages))
	}

	// First: assistant tool_use
	if saver.messages[0].Role != "assistant" {
		t.Errorf("msg 0: expected role 'assistant', got '%s'", saver.messages[0].Role)
	}
	if saver.messages[0].Content != "Let me check." {
		t.Errorf("msg 0: expected content 'Let me check.', got '%s'", saver.messages[0].Content)
	}

	// Second: user tool_result (content should be empty)
	if saver.messages[1].Role != "user" {
		t.Errorf("msg 1: expected role 'user', got '%s'", saver.messages[1].Role)
	}
	if saver.messages[1].Content != "" {
		t.Errorf("msg 1: expected empty content for tool_result, got '%s'", saver.messages[1].Content)
	}

	// Third: assistant final response
	if saver.messages[2].Role != "assistant" {
		t.Errorf("msg 2: expected role 'assistant', got '%s'", saver.messages[2].Role)
	}
	if saver.messages[2].Content != "Here are your tasks." {
		t.Errorf("msg 2: expected content 'Here are your tasks.', got '%s'", saver.messages[2].Content)
	}
}

func TestEngine_Chat_NoSaver_StillWorks(t *testing.T) {
	mockProv := &mockLLM{
		responses: []*llm.ChatResponse{
			{Content: "Hello!", RawContent: `[{"type":"text","text":"Hello!"}]`, StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, nil)
	// No saver set — should still work

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("expected 'Hello!', got '%s'", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./assistant/ -run TestEngine_Chat_Persists -v`
Expected: FAIL — `SetMessageSaver` doesn't exist

**Step 3: Implement the changes**

Add `MessageSaver` interface and update Engine in `assistant/engine.go`:

```go
// MessageSaver persists messages during the chat loop.
type MessageSaver interface {
	SaveMessage(senderID, receiverID int64, role, content, rawContent string) error
}

type Engine struct {
	provider        llm.Provider
	registry        *tools.Registry
	skills          []Skill
	contextProvider ContextProvider
	profileProvider ProfileProvider
	messageSaver    MessageSaver
}

func (e *Engine) SetMessageSaver(saver MessageSaver) {
	e.messageSaver = saver
}
```

Update `engine.Chat()` tool loop to persist each turn:

```go
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
	userData := auth.UserDataFromContext(ctx)

	// ... existing system prompt building (unchanged) ...

	// Build messages with context
	var messages []llm.Message
	contextMsgs, err := e.contextProvider.GetContextMessages(userData.UserID)
	if err == nil {
		for _, cm := range contextMsgs {
			msg := llm.Message{Role: cm.Role}
			if cm.RawContent != "" {
				msg.Content = parseRawContent(cm.RawContent)
			} else {
				msg.Content = cm.Content
			}
			messages = append(messages, msg)
		}
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: message,
	})

	maxIterations := 10
	for range maxIterations {
		resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        llmTools,
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// If no tool calls, save final response and return
		if len(resp.ToolCalls) == 0 {
			if e.messageSaver != nil {
				e.messageSaver.SaveMessage(db.BobotUserID, userData.UserID, "assistant", resp.Content, resp.RawContent)
			}
			return resp.Content, nil
		}

		// Build assistant message with tool use
		toolUseContent := make([]map[string]any, 0)
		for _, tc := range resp.ToolCalls {
			slog.Info("llm tool call requested", "tool", tc.Name, "id", tc.ID, "input", tc.Input)
			toolUseContent = append(toolUseContent, map[string]any{
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

		// Save assistant tool_use message
		if e.messageSaver != nil {
			e.messageSaver.SaveMessage(db.BobotUserID, userData.UserID, "assistant", resp.Content, resp.RawContent)
		}

		// Execute tools and add results
		toolResults := make([]map[string]any, 0)
		for _, tc := range resp.ToolCalls {
			result, err := e.registry.Execute(ctx, tc.Name, tc.Input)
			if err != nil {
				slog.Error("llm tool call failed", "tool", tc.Name, "id", tc.ID, "error", err)
				result = fmt.Sprintf("Error: %v", err)
			} else {
				slog.Info("llm tool call result", "tool", tc.Name, "id", tc.ID, "result", result)
			}
			toolResults = append(toolResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: toolResults,
		})

		// Save tool_result message
		if e.messageSaver != nil {
			rawToolResults, _ := json.Marshal(toolResults)
			e.messageSaver.SaveMessage(userData.UserID, db.BobotUserID, "user", "", string(rawToolResults))
		}
	}

	return "", fmt.Errorf("max iterations reached")
}
```

Note: The engine now imports `db` for `db.BobotUserID`. To avoid a circular dependency, pass the user IDs through context instead. The `userData.UserID` is already in context. For bobot's ID, the `MessageSaver` implementation knows it. So change the `MessageSaver` interface to not take sender/receiver IDs — instead, it takes the userID (from context) and figures out directionality from the role:

```go
type MessageSaver interface {
	SaveMessage(userID int64, role, content, rawContent string) error
}
```

The implementation in `context/saver.go` maps:
- `role = "assistant"` → sender=BobotUserID, receiver=userID
- `role = "user"` → sender=userID, receiver=BobotUserID

**Step 4: Create `context/saver.go`**

```go
package context

import (
	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/db"
)

// CoreDBMessageSaver implements assistant.MessageSaver using CoreDB.
type CoreDBMessageSaver struct {
	db         *db.CoreDB
	tokensStart int
	tokensMax   int
}

var _ assistant.MessageSaver = (*CoreDBMessageSaver)(nil)

func NewCoreDBMessageSaver(coreDB *db.CoreDB, tokensStart, tokensMax int) *CoreDBMessageSaver {
	return &CoreDBMessageSaver{db: coreDB, tokensStart: tokensStart, tokensMax: tokensMax}
}

func (s *CoreDBMessageSaver) SaveMessage(userID int64, role, content, rawContent string) error {
	var senderID, receiverID int64
	if role == "assistant" {
		senderID = db.BobotUserID
		receiverID = userID
	} else {
		senderID = userID
		receiverID = db.BobotUserID
	}

	_, err := s.db.CreatePrivateMessageWithContextThreshold(
		senderID, receiverID, role, content, rawContent,
		s.tokensStart, s.tokensMax,
	)
	return err
}
```

**Step 5: Update `server/chat.go` — remove assistant message saving**

In `handlePrivateChatMessage`, remove lines 135-139 (the `CreatePrivateMessageWithContextThreshold` call for the assistant response). The engine now handles it.

Also update the user message save to pass `rawContent` (which is the content itself for plain user messages):

```go
// Save user message: sender=user, receiver=bobot
s.db.CreatePrivateMessageWithContextThreshold(
    userID, db.BobotUserID, "user", content, content,
    s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
)
```

**Step 6: Wire it up in `main.go`**

After creating the context adapter, create the message saver and set it on the engine:

```go
contextAdapter := bobotcontext.NewCoreDBAdapter(coreDB)
messageSaver := bobotcontext.NewCoreDBMessageSaver(coreDB, cfg.Context.TokensStart, cfg.Context.TokensMax)

engine := assistant.NewEngine(llmProvider, registry, loadedSkills, contextAdapter, contextAdapter)
engine.SetMessageSaver(messageSaver)
```

**Step 7: Run all tests**

Run: `go test ./... -v`
Expected: All pass (except known failing `TestCreateTopic`)

**Step 8: Commit**

```
feat(assistant): persist tool loop messages with raw_content
```

---

### Task 5: Update context windowing to respect atomic exchanges

**Files:**
- Modify: `db/core.go:634-722` (CreatePrivateMessageWithContextThreshold windowing logic)

**Step 1: Write the failing test**

Add to `db/core_raw_content_test.go`:

```go
func TestContextWindowing_AtomicExchange(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Use small token limits to trigger windowing
	tokensStart := 50
	tokensMax := 100

	// Turn 1: user message
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Hello", "\"Hello\"", tokensStart, tokensMax,
	)

	// Turn 2: assistant with tool_use
	toolUseRaw := `[{"type":"tool_use","id":"c1","name":"task","input":{"cmd":"list"}}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "", toolUseRaw, tokensStart, tokensMax,
	)

	// Turn 3: tool_result
	toolResultRaw := `[{"type":"tool_result","tool_use_id":"c1","content":"done"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "", toolResultRaw, tokensStart, tokensMax,
	)

	// Turn 4: assistant final response
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "Here you go", `[{"type":"text","text":"Here you go"}]`, tokensStart, tokensMax,
	)

	// Turn 5: new user message (this may trigger windowing)
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Another question", "\"Another question\"", tokensStart, tokensMax,
	)

	// Get context messages — should never start in the middle of a tool loop
	msgs, err := coreDB.GetPrivateChatContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get context: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected at least one context message")
	}

	// The first message in context should be a real user message (content != ""),
	// not a tool_result message
	first := msgs[0]
	if first.Role == "user" && first.Content == "" {
		t.Error("context window starts with a tool_result message — should start with a real user message")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./db/ -run TestContextWindowing_AtomicExchange -v`
Expected: May pass or fail depending on token sizes — adjust `tokensStart`/`tokensMax` values if needed to trigger windowing.

**Step 3: Implement the change**

In `CreatePrivateMessageWithContextThreshold`, when finding the new chunk start during windowing, ensure it lands on a real user message (one with non-empty `content`), not on a tool_result:

```go
// Find the new chunk start — must be a real user message, not a tool_result
var newChunkStartID int64
var subtractValue int
err := c.db.QueryRow(`
    SELECT id, context_tokens FROM messages
    WHERE topic_id IS NULL
      AND ((sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?))
      AND context_tokens < ?
      AND content != ''
    ORDER BY id DESC LIMIT 1
`, senderID, receiverID, receiverID, senderID, targetThreshold).Scan(&newChunkStartID, &subtractValue)
```

The key change is `AND content != ''` — this ensures chunk boundaries only land on messages that have displayable content (real user messages or real assistant responses), never on tool_result messages (which have empty content).

**Step 4: Run test to verify it passes**

Run: `go test ./db/ -run TestContextWindowing_AtomicExchange -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./... -v`
Expected: All pass (except known failing `TestCreateTopic`)

**Step 6: Commit**

```
feat(db): ensure context window boundaries respect atomic tool exchanges
```

---

### Task 6: End-to-end integration test

**Files:**
- Create: `db/core_raw_content_test.go` (add integration test, or add to existing file)

**Step 1: Write the integration test**

Add to `db/core_raw_content_test.go`:

```go
func TestFullConversationWithToolUse(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")
	tokensStart := 1000
	tokensMax := 4000

	// Simulate: user asks about weather
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "What's the weather in Paris?",
		"\"What's the weather in Paris?\"",
		tokensStart, tokensMax,
	)

	// Assistant responds with tool_use
	assistantToolUseRaw := `[{"type":"text","text":"Let me check the weather."},{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"location":"Paris"}}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "Let me check the weather.", assistantToolUseRaw,
		tokensStart, tokensMax,
	)

	// Tool result
	toolResultRaw := `[{"type":"tool_result","tool_use_id":"toolu_01","content":"{\"temp\":18,\"condition\":\"sunny\"}"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "", toolResultRaw,
		tokensStart, tokensMax,
	)

	// Assistant final response
	finalRaw := `[{"type":"text","text":"It's 18°C and sunny in Paris!"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "It's 18°C and sunny in Paris!", finalRaw,
		tokensStart, tokensMax,
	)

	// Get context messages
	msgs, err := coreDB.GetPrivateChatContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get context: %v", err)
	}

	// Should have 4 messages: user, assistant+tool_use, tool_result, assistant_final
	if len(msgs) != 4 {
		t.Fatalf("expected 4 context messages, got %d", len(msgs))
	}

	// Verify roles
	expectedRoles := []string{"user", "assistant", "user", "assistant"}
	for i, expected := range expectedRoles {
		if msgs[i].Role != expected {
			t.Errorf("msg %d: expected role '%s', got '%s'", i, expected, msgs[i].Role)
		}
	}

	// Verify raw_content is preserved
	if msgs[1].RawContent != assistantToolUseRaw {
		t.Errorf("assistant tool_use raw_content not preserved")
	}
	if msgs[2].RawContent != toolResultRaw {
		t.Errorf("tool_result raw_content not preserved")
	}

	// Verify tool_result has empty content
	if msgs[2].Content != "" {
		t.Errorf("tool_result should have empty content, got '%s'", msgs[2].Content)
	}
}
```

**Step 2: Run the test**

Run: `go test ./db/ -run TestFullConversationWithToolUse -v`
Expected: PASS

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All pass (except known failing `TestCreateTopic`)

**Step 4: Commit**

```
test(db): add end-to-end test for conversation with tool use
```

---

### Task 7: Remove `role IN ('user', 'assistant')` filter from context queries

**Files:**
- Modify: `db/core.go` — `GetPrivateChatContextMessages` (line ~774)

**Step 1: Verify the filter**

The current query at line 774 has:
```sql
AND role IN ('user', 'assistant')
```

This was filtering out `command` and `system` messages. With the new approach, all messages in the context window (including tool_use/tool_result intermediate messages) use `user` and `assistant` roles, so this filter is actually fine as-is — it still correctly excludes `command` and `system` messages.

**No changes needed for this task.** The existing filter already works correctly because intermediate tool messages use `user`/`assistant` roles.

Mark this task as complete — no commit needed.

---

### Summary of Changes

| File | Change |
|------|--------|
| `db/core.go` | Add `RawContent` field to Message, migration, update all SELECT queries, update all Create functions to accept rawContent, token estimation from rawContent, atomic windowing |
| `llm/provider.go` | Add `RawContent` to ChatResponse |
| `llm/anthropic.go` | Extract `buildChatResponse`, populate RawContent from API content array |
| `assistant/engine.go` | Add `RawContent` to ContextMessage, add `MessageSaver` interface, persist turns in Chat, add `parseRawContent` helper |
| `context/adapter.go` | Map `RawContent` from db.Message to ContextMessage |
| `context/saver.go` | New file — CoreDBMessageSaver implementing MessageSaver |
| `server/chat.go` | Remove assistant message save, pass rawContent for user message save |
| `main.go` | Create and wire CoreDBMessageSaver |
