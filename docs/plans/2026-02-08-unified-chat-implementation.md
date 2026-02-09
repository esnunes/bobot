# Unified Chat Engine Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Unify `Chat` and `ChatWithContext` into a single `Chat(ctx, ChatOptions)` method with shared tool loop, persistence, and system prompt — branching only for profile injection, context retrieval, message saving, and message attribution.

**Architecture:** Single `Chat` method with `ChatOptions{Message, TopicID, DisplayName}`. Topic chat (`TopicID > 0`) injects all member profiles, fetches topic context, saves via `SaveTopicMessage`, and prepends `[DisplayName]:` to user messages. Private chat (`TopicID == 0`) works exactly as today.

**Tech Stack:** Go, SQLite (existing)

---

### Task 1: Refactor Chat signature to ChatOptions

Pure refactor — no behavior change. Update the `Chat` method to accept `ChatOptions` instead of a plain string. Update all callers.

**Files:**
- Modify: `assistant/engine.go:61-63`
- Modify: `assistant/engine_test.go` (all `engine.Chat(ctx, "msg")` calls)
- Modify: `server/chat.go:129`

**Step 1: Add ChatOptions struct and update Chat signature**

In `assistant/engine.go`, add the struct after line 36 and update the signature:

```go
// ChatOptions configures a Chat call.
type ChatOptions struct {
	Message     string
	TopicID     int64  // if > 0, topic chat; 0 means private chat
	DisplayName string // sender's display name (for topic message attribution)
}
```

Change line 63 from:
```go
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
```
to:
```go
func (e *Engine) Chat(ctx context.Context, opts ChatOptions) (string, error) {
```

Replace all `message` references inside `Chat` with `opts.Message`.

**Step 2: Update test callers**

In `assistant/engine_test.go`, replace every `engine.Chat(ctx, "...")` with `engine.Chat(ctx, ChatOptions{Message: "..."})`.

Affected tests:
- `TestEngine_Chat_SimpleResponse` (line 58)
- `TestEngine_Chat_WithToolUse` (line 87)
- `TestEngine_ChatWithContext` (line 135) — rename to `TestEngine_Chat_WithContextMessages`
- `TestEngine_Chat_InjectsProfile` (line 223)
- `TestEngine_Chat_PersistsToolLoop` (line 285)
- `TestEngine_Chat_NoSaver_StillWorks` (line 339)
- `TestEngine_Chat_NoProfileNoInjection` (line 364)

**Step 3: Update server caller**

In `server/chat.go`, change line 129 from:
```go
response, err := s.engine.Chat(ctx, content)
```
to:
```go
response, err := s.engine.Chat(ctx, assistant.ChatOptions{Message: content})
```

Add `"github.com/esnunes/bobot/assistant"` to imports if not present.

**Step 4: Run tests**

Run: `go test ./...`
Expected: All tests pass (no behavior change).

**Step 5: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go server/chat.go
git commit -m "refactor(assistant): change Chat signature to accept ChatOptions struct"
```

---

### Task 2: Extend interfaces with topic methods

Add the new topic methods to `ContextProvider`, `MessageSaver`, and `ProfileProvider`. Add stub implementations in mocks and adapters so everything compiles.

**Files:**
- Modify: `assistant/engine.go:16-36` (interfaces)
- Modify: `assistant/engine_test.go` (mock structs)
- Modify: `context/adapter.go` (stub implementations)
- Modify: `context/saver.go` (stub implementation)

**Step 1: Extend interfaces in engine.go**

Update `ContextProvider`:
```go
type ContextProvider interface {
	GetContextMessages(userID int64) ([]ContextMessage, error)
	GetTopicContextMessages(topicID int64) ([]ContextMessage, error)
}
```

Update `ProfileProvider`:
```go
type ProfileProvider interface {
	GetUserProfile(userID int64) (string, int64, error)
	GetTopicMemberProfiles(topicID int64) (string, error)
}
```

Update `MessageSaver`:
```go
type MessageSaver interface {
	SaveMessage(userID int64, role, content, rawContent string) error
	SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error
}
```

**Step 2: Add stubs in context/adapter.go**

```go
func (a *CoreDBAdapter) GetTopicContextMessages(topicID int64) ([]assistant.ContextMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (a *CoreDBAdapter) GetTopicMemberProfiles(topicID int64) (string, error) {
	return "", fmt.Errorf("not implemented")
}
```

Add `"fmt"` to imports.

**Step 3: Add stub in context/saver.go**

```go
func (s *CoreDBMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	return fmt.Errorf("not implemented")
}
```

Add `"fmt"` to imports.

**Step 4: Update test mocks in engine_test.go**

Add to `mockContextProvider`:
```go
func (m *mockContextProvider) GetTopicContextMessages(topicID int64) ([]ContextMessage, error) {
	return m.messages, nil
}
```

Add to `mockProfileProvider`:
```go
func (m *mockProfileProvider) GetTopicMemberProfiles(topicID int64) (string, error) {
	return "", nil
}
```

Add to `mockMessageSaver`:
```go
func (m *mockMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	return nil
}
```

**Step 5: Run tests**

Run: `go test ./...`
Expected: All tests pass (stubs compile, no behavior change).

**Step 6: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go context/adapter.go context/saver.go
git commit -m "refactor(assistant): extend interfaces with topic context, save, and profile methods"
```

---

### Task 3: Implement GetTopicContextMessages in adapter

**Files:**
- Modify: `context/adapter.go`
- Modify: `context/adapter_test.go`

**Step 1: Write the failing test**

In `context/adapter_test.go`, add:

```go
func TestCoreDBAdapter_GetTopicContextMessages(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	// Create users and topic
	user1, _ := coreDB.CreateUser("alice", "hash")
	coreDB.UpdateUserProfile(user1.ID, "Alice", "user")
	topic, _ := coreDB.CreateTopic("Test Topic", user1.ID)

	// Create topic messages with raw_content containing attribution
	coreDB.CreateTopicMessageWithContext(
		topic.ID, user1.ID, "user", "Hello", "[Alice]: Hello",
		1000, 4000,
	)
	coreDB.CreateTopicMessageWithContext(
		topic.ID, db.BobotUserID, "assistant", "Hi there", "Hi there",
		1000, 4000,
	)

	adapter := NewCoreDBAdapter(coreDB)
	messages, err := adapter.GetTopicContextMessages(topic.ID)
	if err != nil {
		t.Fatalf("failed to get topic messages: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	if messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %s", messages[0].Role)
	}
	if messages[0].RawContent != "[Alice]: Hello" {
		t.Errorf("expected raw_content '[Alice]: Hello', got '%s'", messages[0].RawContent)
	}
	if messages[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %s", messages[1].Role)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./context/ -run TestCoreDBAdapter_GetTopicContextMessages -v`
Expected: FAIL (stub returns error "not implemented")

**Step 3: Implement GetTopicContextMessages**

In `context/adapter.go`, replace the stub:

```go
func (a *CoreDBAdapter) GetTopicContextMessages(topicID int64) ([]assistant.ContextMessage, error) {
	messages, err := a.db.GetTopicContextMessages(topicID)
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

Remove `"fmt"` from imports if no longer needed.

**Step 4: Run test to verify it passes**

Run: `go test ./context/ -run TestCoreDBAdapter_GetTopicContextMessages -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add context/adapter.go context/adapter_test.go
git commit -m "feat(context): implement GetTopicContextMessages in adapter"
```

---

### Task 4: Implement GetTopicMemberProfiles in adapter

**Files:**
- Modify: `context/adapter.go`
- Modify: `context/adapter_test.go`

**Step 1: Write the failing test**

In `context/adapter_test.go`, add:

```go
func TestCoreDBAdapter_GetTopicMemberProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	// Create users with profiles
	user1, _ := coreDB.CreateUser("alice", "hash")
	coreDB.UpdateUserProfile(user1.ID, "Alice", "user")
	coreDB.UpsertUserProfile(user1.ID, "Alice is a morning person.", 0)

	user2, _ := coreDB.CreateUser("bob", "hash")
	coreDB.UpdateUserProfile(user2.ID, "Bob", "user")
	coreDB.UpsertUserProfile(user2.ID, "Bob handles groceries.", 0)

	// Create topic with both members
	topic, _ := coreDB.CreateTopic("Family", user1.ID)
	coreDB.AddTopicMember(topic.ID, user2.ID)

	adapter := NewCoreDBAdapter(coreDB)
	profiles, err := adapter.GetTopicMemberProfiles(topic.ID)
	if err != nil {
		t.Fatalf("failed to get profiles: %v", err)
	}

	if profiles == "" {
		t.Fatal("expected non-empty profiles string")
	}

	// Should contain both members' profiles
	if !strings.Contains(profiles, "Alice is a morning person.") {
		t.Error("expected profiles to contain Alice's profile")
	}
	if !strings.Contains(profiles, "Bob handles groceries.") {
		t.Error("expected profiles to contain Bob's profile")
	}
	if !strings.Contains(profiles, `name="Alice"`) {
		t.Error("expected profiles to contain Alice's display name tag")
	}
	if !strings.Contains(profiles, `name="Bob"`) {
		t.Error("expected profiles to contain Bob's display name tag")
	}
}

func TestCoreDBAdapter_GetTopicMemberProfiles_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	user1, _ := coreDB.CreateUser("alice", "hash")
	coreDB.UpdateUserProfile(user1.ID, "Alice", "user")
	topic, _ := coreDB.CreateTopic("Empty", user1.ID)

	adapter := NewCoreDBAdapter(coreDB)
	profiles, err := adapter.GetTopicMemberProfiles(topic.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if profiles != "" {
		t.Errorf("expected empty profiles when no member has a profile, got '%s'", profiles)
	}
}
```

Add `"strings"` to imports.

**Step 2: Run test to verify it fails**

Run: `go test ./context/ -run TestCoreDBAdapter_GetTopicMemberProfiles -v`
Expected: FAIL

**Step 3: Implement GetTopicMemberProfiles**

In `context/adapter.go`, replace the stub:

```go
func (a *CoreDBAdapter) GetTopicMemberProfiles(topicID int64) (string, error) {
	members, err := a.db.GetTopicMembers(topicID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	hasProfiles := false

	for _, m := range members {
		content, _, err := a.db.GetUserProfile(m.UserID)
		if err != nil || content == "" {
			continue
		}

		if !hasProfiles {
			sb.WriteString("## Topic Members\nThe following are the profiles of the members in this topic:\n")
			hasProfiles = true
		}

		name := m.DisplayName
		if name == "" {
			name = m.Username
		}
		fmt.Fprintf(&sb, "\n<member name=%q>\n%s\n</member>\n", name, content)
	}

	if !hasProfiles {
		return "", nil
	}
	return sb.String(), nil
}
```

Add `"fmt"` and `"strings"` to imports.

**Step 4: Run test to verify it passes**

Run: `go test ./context/ -run TestCoreDBAdapter_GetTopicMemberProfiles -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add context/adapter.go context/adapter_test.go
git commit -m "feat(context): implement GetTopicMemberProfiles in adapter"
```

---

### Task 5: Implement SaveTopicMessage in saver

**Files:**
- Modify: `context/saver.go`
- Create: `context/saver_test.go`

**Step 1: Write the failing test**

Create `context/saver_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./context/ -run TestCoreDBMessageSaver_SaveTopicMessage -v`
Expected: FAIL (stub returns "not implemented")

**Step 3: Implement SaveTopicMessage**

In `context/saver.go`, replace the stub:

```go
func (s *CoreDBMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	var senderID int64
	if role == "assistant" {
		senderID = db.BobotUserID
	} else {
		senderID = userID
	}

	_, err := s.db.CreateTopicMessageWithContext(
		topicID, senderID, role, content, rawContent,
		s.tokensStart, s.tokensMax,
	)
	return err
}
```

Remove `"fmt"` from imports if no longer needed.

**Step 4: Run test to verify it passes**

Run: `go test ./context/ -run TestCoreDBMessageSaver_SaveTopicMessage -v`
Expected: PASS

**Step 5: Run all tests**

Run: `go test ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add context/saver.go context/saver_test.go
git commit -m "feat(context): implement SaveTopicMessage in message saver"
```

---

### Task 6: Add topic chat support to engine Chat method

Refactor `Chat` to support topic chats. Add ChatData to context for tool calls. Remove `ChatWithContext`.

**Files:**
- Modify: `assistant/engine.go`
- Modify: `assistant/engine_test.go`

**Step 1: Write failing test for topic chat**

In `assistant/engine_test.go`, add:

```go
type mockTopicContextProvider struct {
	privateMessages []ContextMessage
	topicMessages   []ContextMessage
}

func (m *mockTopicContextProvider) GetContextMessages(userID int64) ([]ContextMessage, error) {
	return m.privateMessages, nil
}

func (m *mockTopicContextProvider) GetTopicContextMessages(topicID int64) ([]ContextMessage, error) {
	return m.topicMessages, nil
}

type mockTopicProfileProvider struct {
	userProfiles  map[int64]string
	topicProfiles map[int64]string
}

func (m *mockTopicProfileProvider) GetUserProfile(userID int64) (string, int64, error) {
	content := m.userProfiles[userID]
	return content, 0, nil
}

func (m *mockTopicProfileProvider) GetTopicMemberProfiles(topicID int64) (string, error) {
	return m.topicProfiles[topicID], nil
}

type mockTopicMessageSaver struct {
	privateMessages []savedMessage
	topicMessages   []savedTopicMessage
}

type savedTopicMessage struct {
	TopicID    int64
	UserID     int64
	Role       string
	Content    string
	RawContent string
}

func (m *mockTopicMessageSaver) SaveMessage(userID int64, role, content, rawContent string) error {
	m.privateMessages = append(m.privateMessages, savedMessage{
		UserID: userID, Role: role, Content: content, RawContent: rawContent,
	})
	return nil
}

func (m *mockTopicMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	m.topicMessages = append(m.topicMessages, savedTopicMessage{
		TopicID: topicID, UserID: userID, Role: role, Content: content, RawContent: rawContent,
	})
	return nil
}

func TestEngine_Chat_TopicSimpleResponse(t *testing.T) {
	var capturedSystemPrompt string
	var capturedMessages []llm.Message
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			capturedMessages = req.Messages
			return &llm.ChatResponse{Content: "Got it!"}, nil
		},
	}

	topicCtx := &mockTopicContextProvider{
		topicMessages: []ContextMessage{
			{Role: "user", Content: "hello", RawContent: "[Alice]: hello"},
			{Role: "assistant", Content: "hi there", RawContent: "hi there"},
		},
	}

	topicProfile := &mockTopicProfileProvider{
		topicProfiles: map[int64]string{
			42: "## Topic Members\n\n<member name=\"Alice\">\nLikes coffee.\n</member>",
		},
	}

	saver := &mockTopicMessageSaver{}
	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, topicCtx, topicProfile)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})
	result, err := engine.Chat(ctx, ChatOptions{
		Message:     "Hey @bobot, what's up?",
		TopicID:     42,
		DisplayName: "Bob",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Got it!" {
		t.Errorf("expected 'Got it!', got '%s'", result)
	}

	// System prompt should contain topic member profiles
	if !strings.Contains(capturedSystemPrompt, "Topic Members") {
		t.Error("expected system prompt to contain topic member profiles")
	}
	if !strings.Contains(capturedSystemPrompt, "Alice") {
		t.Error("expected system prompt to contain Alice's profile")
	}

	// Should NOT contain single user profile section
	if strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to NOT contain <user-profile> tags in topic chat")
	}

	// Context messages should use raw_content
	if len(capturedMessages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(capturedMessages))
	}
	// First context message should use raw_content (with attribution)
	if capturedMessages[0].Content != "[Alice]: hello" {
		t.Errorf("expected first context message to be '[Alice]: hello', got '%v'", capturedMessages[0].Content)
	}

	// New user message should have DisplayName prepended
	lastMsg := capturedMessages[len(capturedMessages)-1]
	if lastMsg.Content != "[Bob]: Hey @bobot, what's up?" {
		t.Errorf("expected last message to be '[Bob]: Hey @bobot, what's up?', got '%v'", lastMsg.Content)
	}

	// Should have saved via SaveTopicMessage, not SaveMessage
	if len(saver.topicMessages) != 1 {
		t.Fatalf("expected 1 topic message saved, got %d", len(saver.topicMessages))
	}
	if saver.topicMessages[0].TopicID != 42 {
		t.Errorf("expected topicID 42, got %d", saver.topicMessages[0].TopicID)
	}
	if len(saver.privateMessages) != 0 {
		t.Errorf("expected 0 private messages saved, got %d", len(saver.privateMessages))
	}
}

func TestEngine_Chat_TopicWithTools(t *testing.T) {
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
			{Content: "Here are the tasks.", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk"})

	topicCtx := &mockTopicContextProvider{}
	saver := &mockTopicMessageSaver{}
	engine := NewEngine(mockProv, registry, nil, topicCtx, nil)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})
	result, err := engine.Chat(ctx, ChatOptions{
		Message:     "list tasks",
		TopicID:     10,
		DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are the tasks." {
		t.Errorf("unexpected result: %s", result)
	}

	// Should have saved 3 topic messages (tool_use, tool_result, final response)
	if len(saver.topicMessages) != 3 {
		t.Fatalf("expected 3 topic messages, got %d", len(saver.topicMessages))
	}
	for _, m := range saver.topicMessages {
		if m.TopicID != 10 {
			t.Errorf("expected topicID 10, got %d", m.TopicID)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./assistant/ -run TestEngine_Chat_Topic -v`
Expected: FAIL (topic branching not yet implemented)

**Step 3: Implement topic support in Chat**

Replace the entire `Chat` method in `assistant/engine.go` with the unified implementation. Add ChatData injection for tools. Remove `ChatWithContext`.

```go
func (e *Engine) Chat(ctx context.Context, opts ChatOptions) (string, error) {
	userData := auth.UserDataFromContext(ctx)

	// Build system prompt with role-filtered tools
	llmTools := e.registry.ToLLMToolsForRole(userData.Role)
	systemPrompt := BuildSystemPrompt(e.skills, llmTools)

	// Inject profiles
	if opts.TopicID > 0 {
		// Topic chat: inject all member profiles
		if e.profileProvider != nil {
			profiles, err := e.profileProvider.GetTopicMemberProfiles(opts.TopicID)
			if err == nil && profiles != "" {
				systemPrompt += "\n\n" + profiles
			}
		}
	} else {
		// Private chat: inject single user profile
		if e.profileProvider != nil {
			profileContent, _, err := e.profileProvider.GetUserProfile(userData.UserID)
			if err == nil && profileContent != "" {
				systemPrompt += "\n\n## User Profile\nThe following is known about the user you are chatting with:\n<user-profile>\n" + profileContent + "\n</user-profile>"
			}
		}
	}
	slog.Debug("chat system prompt", "content", systemPrompt, "topicID", opts.TopicID)

	// Get context messages
	var contextMsgs []ContextMessage
	var err error
	if opts.TopicID > 0 {
		contextMsgs, err = e.contextProvider.GetTopicContextMessages(opts.TopicID)
	} else {
		contextMsgs, err = e.contextProvider.GetContextMessages(userData.UserID)
	}

	var messages []llm.Message
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

	// Add the new user message (with attribution for topic chat)
	userContent := opts.Message
	if opts.TopicID > 0 && opts.DisplayName != "" {
		userContent = fmt.Sprintf("[%s]: %s", opts.DisplayName, opts.Message)
	}
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userContent,
	})

	// Set ChatData for tool calls in topic chat
	if opts.TopicID > 0 {
		ctx = auth.ContextWithChatData(ctx, auth.ChatData{TopicID: &opts.TopicID})
	}

	// Helper to save messages (private or topic)
	save := func(role, content, rawContent string) {
		if e.messageSaver == nil {
			return
		}
		if opts.TopicID > 0 {
			e.messageSaver.SaveTopicMessage(opts.TopicID, userData.UserID, role, content, rawContent)
		} else {
			e.messageSaver.SaveMessage(userData.UserID, role, content, rawContent)
		}
	}

	// Loop for tool use
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
			save("assistant", resp.Content, resp.RawContent)
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
		save("assistant", resp.Content, resp.RawContent)

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
		rawToolResults, _ := json.Marshal(toolResults)
		save("user", "", string(rawToolResults))
	}

	return "", fmt.Errorf("max iterations reached")
}
```

Remove the `ChatWithContext` method entirely.

**Step 4: Update TestEngine_ChatWithConversation**

Replace `TestEngine_ChatWithConversation` — this test tested the old `ChatWithContext`. Remove it since it's now covered by `TestEngine_Chat_TopicSimpleResponse`.

**Step 5: Run tests**

Run: `go test ./assistant/ -v`
Expected: All pass.

**Step 6: Run all tests**

Run: `go test ./...`
Expected: All pass (server still compiles because it doesn't call `ChatWithContext` yet — wait, it does).

**Note:** The server's `handleTopicAssistantResponse` calls `ChatWithContext`. This will break compilation. Task 7 must be done immediately after or combined with this step. See step 7 below.

If needed, temporarily keep a deprecated `ChatWithContext` wrapper:

```go
// Deprecated: use Chat with ChatOptions{TopicID: ...} instead.
func (e *Engine) ChatWithContext(ctx context.Context, conversation []string) (string, error) {
	return "", fmt.Errorf("ChatWithContext is deprecated, use Chat with TopicID")
}
```

**Step 7: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go
git commit -m "feat(assistant): add topic chat support to unified Chat method"
```

---

### Task 7: Update server callers and remove ChatWithContext

Simplify server topic chat handling. Remove conversation building, display name lookup, and manual message saving from `handleTopicAssistantResponse`. Update `handleTopicChatMessage` to save user messages with `[DisplayName]:` in `raw_content`.

**Files:**
- Modify: `server/chat.go`
- Modify: `assistant/engine.go` (remove deprecated ChatWithContext if kept)

**Step 1: Update handleTopicChatMessage to save raw_content with attribution**

In `server/chat.go`, in `handleTopicChatMessage`, change the user message save (line 194-197) from:
```go
s.db.CreateTopicMessageWithContext(
    topicID, userID, "user", content, content,
    s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
)
```
to:
```go
rawContent := fmt.Sprintf("[%s]: %s", user.DisplayName, content)
s.db.CreateTopicMessageWithContext(
    topicID, userID, "user", content, rawContent,
    s.cfg.Context.TokensStart, s.cfg.Context.TokensMax,
)
```

**Step 2: Replace handleTopicAssistantResponse**

Replace the entire `handleTopicAssistantResponse` method:

```go
func (s *Server) handleTopicAssistantResponse(ctx context.Context, userID, topicID int64, displayName string) {
	response, err := s.engine.Chat(ctx, assistant.ChatOptions{
		Message:     "",
		TopicID:     topicID,
		DisplayName: displayName,
	})
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	// Broadcast to topic
	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "assistant",
		"content":      response,
		"display_name": "bobot",
	})
	s.broadcastToTopic(topicID, assistantMsgJSON)
}
```

Note: the engine now handles fetching context and saving the assistant message. The server only broadcasts.

Wait — passing `Message: ""` is wrong. The user message is already saved to DB by the server. The engine fetches context from DB (which includes it) and adds it again as the new message. But with empty message, it would add an empty message.

Actually, looking at the private chat flow: the server saves the user message to DB, then calls `engine.Chat(ctx, ChatOptions{Message: content})`. The engine fetches context (which includes the just-saved message) AND adds `content` again. So there's a duplication in private chat too.

For topic chat, we should follow the same pattern: pass the message that triggered @bobot. Update the call site:

```go
func (s *Server) handleTopicAssistantResponse(ctx context.Context, userID, topicID int64, content, displayName string) {
	response, err := s.engine.Chat(ctx, assistant.ChatOptions{
		Message:     content,
		TopicID:     topicID,
		DisplayName: displayName,
	})
	if err != nil {
		log.Printf("assistant error: %v", err)
		response = "Sorry, I encountered an error. Please try again."
	}

	assistantMsgJSON, _ := json.Marshal(map[string]interface{}{
		"topic_id":     topicID,
		"role":         "assistant",
		"content":      response,
		"display_name": "bobot",
	})
	s.broadcastToTopic(topicID, assistantMsgJSON)
}
```

**Step 3: Update the call site in handleTopicChatMessage**

Change line 211 from:
```go
s.handleTopicAssistantResponse(ctx, topicID)
```
to:
```go
s.handleTopicAssistantResponse(ctx, userID, topicID, content, user.DisplayName)
```

**Step 4: Remove deprecated ChatWithContext from engine.go**

Remove the `ChatWithContext` method (or the deprecated stub) from `assistant/engine.go`.

**Step 5: Add assistant import to server/chat.go**

Add `"github.com/esnunes/bobot/assistant"` to imports if not already present (was added in Task 1).

**Step 6: Run all tests**

Run: `go test ./...`
Expected: All pass.

**Step 7: Commit**

```bash
git add assistant/engine.go server/chat.go
git commit -m "feat(server): use unified Chat for topic chats, save raw_content with attribution"
```
