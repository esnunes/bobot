# Topic Management Tool Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `/topic` slash command tool for managing topics from chat, and refactor the tool interface to pass raw args instead of pre-parsed maps.

**Architecture:** Refactor `Tool.Execute` to accept a typed `ExecuteInput` struct with raw args string, receiver_id, and topic_id. Update all existing tools and callers. Then add the new TopicTool with 6 commands (create, delete, leave, add, remove, list). Add a case-insensitive unique index on active topic names and a `GetTopicByName` DB method.

**Tech Stack:** Go, SQLite, WebSocket

---

### Task 1: Refactor Tool interface — add ExecuteInput struct

**Files:**
- Modify: `tools/registry.go:11-16` (Tool interface) and `tools/registry.go:46-52` (Registry.Execute)

**Step 1: Write the failing test**

Update `tools/registry_test.go` — change the mockTool to use the new signature. The test will fail because the interface hasn't changed yet.

```go
// tools/registry_test.go
package tools

import (
	"context"
	"testing"
)

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock" }
func (m *mockTool) Description() string { return "A mock tool" }
func (m *mockTool) Schema() interface{} { return map[string]string{"type": "object"} }
func (m *mockTool) Execute(ctx context.Context, input ExecuteInput) (string, error) {
	return "executed", nil
}
func (m *mockTool) AdminOnly() bool { return false }

func TestRegistry_Register(t *testing.T) {
	reg := NewRegistry()
	mock := &mockTool{}

	reg.Register(mock)

	if len(reg.List()) != 1 {
		t.Errorf("expected 1 tool, got %d", len(reg.List()))
	}
}

func TestRegistry_Get(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	tool, ok := reg.Get("mock")
	if !ok {
		t.Fatal("expected to find mock tool")
	}
	if tool.Name() != "mock" {
		t.Errorf("expected name mock, got %s", tool.Name())
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find tool")
	}
}

func TestRegistry_Execute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{})

	result, err := reg.Execute(context.Background(), "mock", ExecuteInput{Args: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "executed" {
		t.Errorf("expected 'executed', got '%s'", result)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/ -v`
Expected: FAIL — `ExecuteInput` not defined, signature mismatch

**Step 3: Write minimal implementation**

Update `tools/registry.go`:

```go
// tools/registry.go
package tools

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/llm"
)

// ExecuteInput contains the input for tool execution.
type ExecuteInput struct {
	Args       string // raw string after tool name, e.g. "create hello world"
	ReceiverID *int64 // set in private chat, nil in topic chat
	TopicID    *int64 // set in topic chat, nil in private chat
}

type Tool interface {
	Name() string
	Description() string
	Schema() interface{}
	Execute(ctx context.Context, input ExecuteInput) (string, error)
	AdminOnly() bool
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, name string, input ExecuteInput) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, input)
}

func (r *Registry) ToLLMTools() []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}

func (r *Registry) ToLLMToolsForRole(role string) []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool.AdminOnly() && role != "admin" {
			continue
		}
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./tools/ -v`
Expected: PASS (registry tests pass, but other packages will break — that's OK, we fix them next)

**Step 5: Commit**

```bash
git add tools/registry.go tools/registry_test.go
git commit -m "refactor: update Tool interface to use ExecuteInput struct"
```

---

### Task 2: Update UserTool to use ExecuteInput

**Files:**
- Modify: `tools/user/user.go:57-84` (Execute method)
- Modify: `tools/user/user_test.go` (all Execute calls)

**Step 1: Update the tests**

Replace all `tool.Execute(ctx, map[string]interface{}{...})` calls with `tool.Execute(ctx, tools.ExecuteInput{Args: "..."})`.

Update `tools/user/user_test.go`:

```go
package user

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func TestUserTool_InviteCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "invite"})
	if err != nil {
		t.Fatalf("failed to execute invite: %v", err)
	}

	if !strings.Contains(result, "http://localhost:8080/signup?code=") {
		t.Errorf("expected signup URL, got: %s", result)
	}
}

func TestUserTool_BlockCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	user, _ := coreDB.CreateUserFull("victim", "hash", "Victim", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "block victim"})
	if err != nil {
		t.Fatalf("failed to execute block: %v", err)
	}

	if !strings.Contains(result, "blocked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify user is blocked
	updated, _ := coreDB.GetUserByID(user.ID)
	if !updated.Blocked {
		t.Error("expected user to be blocked")
	}
}

func TestUserTool_NonAdminDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("user", "hash", "User", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: user.ID,
		Role:   "user",
	})

	_, err := tool.Execute(ctx, tools.ExecuteInput{Args: "list"})
	if err == nil {
		t.Error("expected error for non-admin")
	}
	if !strings.Contains(err.Error(), "admin") {
		t.Errorf("expected admin error, got: %v", err)
	}
}

func TestUserTool_CannotBlockSelf(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	_, err := tool.Execute(ctx, tools.ExecuteInput{Args: "block admin"})
	if err == nil {
		t.Error("expected error when blocking self")
	}
}

func TestUserTool_ListCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateUserFull("user1", "hash", "User One", "user")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "list"})
	if err != nil {
		t.Fatalf("failed to execute list: %v", err)
	}

	if !strings.Contains(result, "admin") || !strings.Contains(result, "user1") {
		t.Errorf("expected user list, got: %s", result)
	}
}

func TestUserTool_UnblockCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	user, _ := coreDB.CreateUserFull("blocked", "hash", "Blocked", "user")
	coreDB.BlockUser(user.ID)
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "unblock blocked"})
	if err != nil {
		t.Fatalf("failed to execute unblock: %v", err)
	}

	if !strings.Contains(result, "unblocked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify user is unblocked
	updated, _ := coreDB.GetUserByID(user.ID)
	if updated.Blocked {
		t.Error("expected user to be unblocked")
	}
}

func TestUserTool_InvitesCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "invite1")
	coreDB.CreateInvite(admin.ID, "invite2")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "invites"})
	if err != nil {
		t.Fatalf("failed to execute invites: %v", err)
	}

	if !strings.Contains(result, "invite1") || !strings.Contains(result, "invite2") {
		t.Errorf("expected invite list, got: %s", result)
	}
}

func TestUserTool_RevokeCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	admin, _ := coreDB.CreateUserFull("admin", "hash", "Admin", "admin")
	coreDB.CreateInvite(admin.ID, "torevoke")
	tool := NewUserTool(coreDB, "http://localhost:8080")

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: admin.ID,
		Role:   "admin",
	})

	result, err := tool.Execute(ctx, tools.ExecuteInput{Args: "revoke torevoke"})
	if err != nil {
		t.Fatalf("failed to execute revoke: %v", err)
	}

	if !strings.Contains(result, "revoked") {
		t.Errorf("expected confirmation, got: %s", result)
	}

	// Verify invite is revoked
	invite, _ := coreDB.GetInviteByCode("torevoke")
	if !invite.Revoked {
		t.Error("expected invite to be revoked")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./tools/user/ -v`
Expected: FAIL — Execute signature mismatch

**Step 3: Update UserTool.Execute to parse Args**

Update `tools/user/user.go` — change the Execute signature and parse `input.Args` to extract command and arguments:

```go
func (t *UserTool) Execute(ctx context.Context, input tools.ExecuteInput) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	// Check admin role
	if userData.Role != "admin" {
		return "", fmt.Errorf("this command requires admin privileges")
	}

	parts := strings.Fields(input.Args)
	if len(parts) == 0 {
		return "", fmt.Errorf("missing command. Usage: /user <command>")
	}

	command := parts[0]
	var arg string
	if len(parts) > 1 {
		arg = parts[1]
	}

	switch command {
	case "invite":
		return t.invite(userData.UserID)
	case "block":
		return t.block(userData.UserID, arg)
	case "unblock":
		return t.unblock(arg)
	case "list":
		return t.list()
	case "invites":
		return t.listInvites()
	case "revoke":
		return t.revoke(arg)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}
```

Also add the `tools` import to `user.go`:
```go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./tools/user/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/user/user.go tools/user/user_test.go
git commit -m "refactor: update UserTool to use ExecuteInput"
```

---

### Task 3: Update TaskTool to use ExecuteInput

**Files:**
- Modify: `tools/task/task.go:59-87` (Execute method)
- Modify: `tools/task/task_test.go` (all Execute calls)

**Step 1: Read the existing task tests**

Read `tools/task/task_test.go` to understand all test cases.

**Step 2: Update the tests**

Change all `tool.Execute(ctx, map[string]interface{}{...})` calls to use `tools.ExecuteInput{Args: "..."}`. The task tool is more complex since it uses `project` and `title` and `status` — these get encoded as space-separated args in the raw string.

The task tool parses `input.Args` to extract: `command project [title] [--status=value]`. Adapt the test accordingly.

Note: Since the task tool is used by the LLM (which sends structured input), and by slash commands (which send raw text), the tool needs to handle both paths. However, per the design, the LLM tool execution path in `assistant/engine.go` also needs updating (Task 5). For now, update the slash command path.

The task tool args format: `/task create groceries buy milk` → Args: `"create groceries buy milk"`

Update `tools/task/task.go` Execute method to parse from `input.Args`:

```go
func (t *TaskTool) Execute(ctx context.Context, input tools.ExecuteInput) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	if userData.UserID == 0 {
		return "", fmt.Errorf("user_id not found in context")
	}

	parts := strings.Fields(input.Args)
	if len(parts) < 2 {
		return "", fmt.Errorf("missing arguments. Usage: /task <command> <project> [title] [--status=pending|done]")
	}

	command := parts[0]
	projectName := parts[1]

	// Parse optional title and status from remaining parts
	var title, status string
	remaining := parts[2:]
	var titleParts []string
	for _, p := range remaining {
		if strings.HasPrefix(p, "--status=") {
			status = strings.TrimPrefix(p, "--status=")
		} else {
			titleParts = append(titleParts, p)
		}
	}
	title = strings.Join(titleParts, " ")

	project, err := t.db.GetOrCreateProject(userData.UserID, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to get/create project: %w", err)
	}

	switch command {
	case "create":
		return t.create(project.ID, title)
	case "list":
		return t.list(project.ID, status)
	case "update":
		return t.update(project.ID, title, status)
	case "delete":
		return t.delete(project.ID, title)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}
```

Also add the `tools` import.

**Step 3: Update task tests to match new interface**

Update the test calls from `map[string]interface{}` to `tools.ExecuteInput{Args: "..."}`.

**Step 4: Run tests**

Run: `go test ./tools/task/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/task/task.go tools/task/task_test.go
git commit -m "refactor: update TaskTool to use ExecuteInput"
```

---

### Task 4: Update handleSlashCommand and chat handler

**Files:**
- Modify: `server/chat.go:85-87` (private chat slash call)
- Modify: `server/chat.go:148-164` (topic chat slash call)
- Modify: `server/chat.go:283-334` (handleSlashCommand)

**Step 1: Update handleSlashCommand**

The function signature changes to accept `receiverID *int64` and `topicID *int64`, and simplifies argument passing:

```go
func (s *Server) handleSlashCommand(ctx context.Context, content string, receiverID *int64, topicID *int64) (string, bool) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	// Split on first space: tool name vs rest
	toolName := content[1:] // strip leading /
	var args string
	if idx := strings.IndexByte(toolName, ' '); idx != -1 {
		args = toolName[idx+1:]
		toolName = toolName[:idx]
	}

	if toolName == "" {
		return "", false
	}

	// Check if tool exists
	tool, ok := s.registry.Get(toolName)
	if !ok {
		return "Error: unknown command /" + toolName, true
	}

	// Execute the tool
	result, err := tool.Execute(ctx, tools.ExecuteInput{
		Args:       args,
		ReceiverID: receiverID,
		TopicID:    topicID,
	})
	if err != nil {
		return "Error: " + err.Error(), true
	}

	return result, true
}
```

**Step 2: Update callers in chat.go**

In `handlePrivateChatMessage` (line 87), change to:
```go
receiverID := db.BobotUserID
if response, handled := s.handleSlashCommand(ctx, content, &receiverID, nil); handled {
```

In `handleTopicChatMessage` (line 164), change to:
```go
if response, handled := s.handleSlashCommand(ctx, content, nil, &topicID); handled {
```

**Step 3: Run existing chat tests**

Run: `go test ./server/ -v -run TestChatWebSocket`
Expected: PASS (the slash command test uses `/user list` which still works with raw args)

**Step 4: Commit**

```bash
git add server/chat.go
git commit -m "refactor: simplify handleSlashCommand to pass raw args"
```

---

### Task 5: Update engine.go LLM tool execution path

**Files:**
- Modify: `assistant/engine.go:106` (Registry.Execute call)
- Modify: `assistant/engine_test.go:34` (mockTool)

**Step 1: Update the mock and tests**

Update the mockTool in `assistant/engine_test.go` to use the new signature:

```go
func (m *mockTool) Execute(ctx context.Context, input tools.ExecuteInput) (string, error) {
	return m.result, nil
}
```

**Step 2: Update the engine.go Execute call**

In `assistant/engine.go:106`, the LLM sends structured tool calls with `tc.Input` as `map[string]interface{}`. We need to convert this to `ExecuteInput`. The LLM tool call input has a `"command"` key plus additional args. Convert by marshaling the input map to a JSON-like args string:

```go
// Build args from tool input map
var argParts []string
if cmd, ok := tc.Input["command"].(string); ok {
	argParts = append(argParts, cmd)
}
// Append remaining key-value pairs
for k, v := range tc.Input {
	if k == "command" {
		continue
	}
	argParts = append(argParts, fmt.Sprintf("%v", v))
}
args := strings.Join(argParts, " ")

result, err := e.registry.Execute(ctx, tc.Name, tools.ExecuteInput{Args: args})
```

**Step 3: Run engine tests**

Run: `go test ./assistant/ -v`
Expected: PASS

**Step 4: Run all tests to verify nothing is broken**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add assistant/engine.go assistant/engine_test.go
git commit -m "refactor: update LLM tool execution to use ExecuteInput"
```

---

### Task 6: Add GetTopicByName DB method and unique index

**Files:**
- Modify: `db/core.go` (add method + migration for unique index)
- Modify: `db/core_test.go` (add tests)

**Step 1: Write the failing tests**

Add to `db/core_test.go`:

```go
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

func TestTopicNameUniqueCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	owner, _ := db.CreateUser("owner", "hash")
	_, err := db.CreateTopic("General", owner.ID)
	if err != nil {
		t.Fatalf("first CreateTopic failed: %v", err)
	}

	// Creating topic with same name (different case) should fail
	_, err = db.CreateTopic("general", owner.ID)
	if err == nil {
		t.Error("expected error when creating duplicate topic name (case-insensitive)")
	}

	// After deleting, should be able to create again
	topic, _ := db.GetTopicByName("General")
	db.SoftDeleteTopic(topic.ID)

	_, err = db.CreateTopic("General", owner.ID)
	if err != nil {
		t.Fatalf("CreateTopic after delete failed: %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./db/ -v -run TestGetTopicByName`
Expected: FAIL — `GetTopicByName` method doesn't exist

**Step 3: Add the DB method and migration**

Add to `db/core.go` after the `GetTopicByID` method (after line 989):

```go
// GetTopicByName retrieves an active topic by name (case-insensitive).
func (c *CoreDB) GetTopicByName(name string) (*Topic, error) {
	var topic Topic
	var deletedAt sql.NullTime
	err := c.db.QueryRow(
		"SELECT id, name, owner_id, deleted_at, created_at FROM topics WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL",
		name,
	).Scan(&topic.ID, &topic.Name, &topic.OwnerID, &deletedAt, &topic.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		topic.DeletedAt = &deletedAt.Time
	}
	return &topic, nil
}
```

Add the unique index migration at the end of the `migrate()` method (after line 299, before `return nil`):

```go
	// Migrate: add case-insensitive unique index for active topic names
	_, err = c.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_topics_name_active ON topics(LOWER(name)) WHERE deleted_at IS NULL
	`)
	if err != nil {
		return err
	}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./db/ -v -run "TestGetTopicByName|TestTopicNameUnique"`
Expected: PASS

**Step 5: Commit**

```bash
git add db/core.go db/core_test.go
git commit -m "feat: add GetTopicByName and case-insensitive unique index"
```

---

### Task 7: Create TopicTool — create and list commands

**Files:**
- Create: `tools/topic/topic.go`
- Create: `tools/topic/topic_test.go`

**Step 1: Write the failing tests for create and list**

```go
// tools/topic/topic_test.go
package topic

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
)

func setupTestDB(t *testing.T) *db.CoreDB {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	return coreDB
}

func ctxForUser(userID int64, role string) context.Context {
	return auth.ContextWithUserData(context.Background(), auth.UserData{
		UserID: userID,
		Role:   role,
	})
}

func TestTopicTool_Create(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "create General"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(result, `"General"`) {
		t.Errorf("expected topic name in result, got: %s", result)
	}

	// Verify topic exists and user is a member
	topic, _ := coreDB.GetTopicByName("General")
	if topic == nil {
		t.Fatal("expected topic to exist")
	}
	if topic.OwnerID != user.ID {
		t.Errorf("expected owner to be user, got %d", topic.OwnerID)
	}
	isMember, _ := coreDB.IsTopicMember(topic.ID, user.ID)
	if !isMember {
		t.Error("expected creator to be a member")
	}
}

func TestTopicTool_CreateMissingName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "create"})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestTopicTool_CreateDuplicateName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "create General"})
	_, err := tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "create general"})
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestTopicTool_List(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	// Create two topics
	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	tool.Execute(ctxForUser(bob.ID, "user"), tools.ExecuteInput{Args: "create Random"})

	// Alice should see only General
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "General") {
		t.Errorf("expected General in list, got: %s", result)
	}
	if strings.Contains(result, "Random") {
		t.Errorf("expected Random NOT in alice's list, got: %s", result)
	}
}

func TestTopicTool_ListEmpty(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	result, err := tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "No topics") {
		t.Errorf("expected 'No topics' message, got: %s", result)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./tools/topic/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Create the TopicTool with create and list**

Create `tools/topic/topic.go`:

```go
package topic

import (
	"context"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools"
)

type TopicTool struct {
	db *db.CoreDB
}

func NewTopicTool(db *db.CoreDB) *TopicTool {
	return &TopicTool{db: db}
}

func (t *TopicTool) Name() string {
	return "topic"
}

func (t *TopicTool) Description() string {
	return "Manage topics: create, delete, leave, add/remove members, list"
}

func (t *TopicTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "delete", "leave", "add", "remove", "list"},
				"description": "The operation to perform",
			},
		},
		"required": []string{"command"},
	}
}

func (t *TopicTool) AdminOnly() bool {
	return false
}

func (t *TopicTool) Execute(ctx context.Context, input tools.ExecuteInput) (string, error) {
	userData := auth.UserDataFromContext(ctx)

	parts := strings.Fields(input.Args)
	if len(parts) == 0 {
		return "", fmt.Errorf("missing command. Usage: /topic <command>")
	}

	command := parts[0]
	rest := strings.TrimSpace(strings.TrimPrefix(input.Args, command))

	switch command {
	case "create":
		return t.create(userData.UserID, rest)
	case "delete":
		return t.deleteTopic(userData.UserID, rest, input.TopicID)
	case "leave":
		return t.leave(userData.UserID, rest, input.TopicID)
	case "add":
		return t.addMember(userData.UserID, rest, input.TopicID)
	case "remove":
		return t.removeMember(userData.UserID, rest, input.TopicID)
	case "list":
		return t.list(userData.UserID)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *TopicTool) create(userID int64, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing topic name. Usage: /topic create <name>")
	}

	topic, err := t.db.CreateTopic(name, userID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a topic with this name already exists")
		}
		return "", fmt.Errorf("failed to create topic: %w", err)
	}

	if err := t.db.AddTopicMember(topic.ID, userID); err != nil {
		return "", fmt.Errorf("failed to add creator as member: %w", err)
	}

	return fmt.Sprintf("Topic %q created.", topic.Name), nil
}

func (t *TopicTool) list(userID int64) (string, error) {
	topics, err := t.db.GetUserTopics(userID)
	if err != nil {
		return "", fmt.Errorf("failed to list topics: %w", err)
	}

	if len(topics) == 0 {
		return "No topics found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Your topics:\n")
	for _, topic := range topics {
		owner, _ := t.db.GetUserByID(topic.OwnerID)
		ownerName := "unknown"
		if owner != nil {
			if owner.ID == userID {
				ownerName = "you"
			} else {
				ownerName = owner.Username
			}
		}
		members, _ := t.db.GetTopicMembers(topic.ID)
		sb.WriteString(fmt.Sprintf("- %s (owner: %s, %d members)\n", topic.Name, ownerName, len(members)))
	}
	return sb.String(), nil
}

// resolveTopic resolves a topic from either explicit name or current topic ID.
// When both are available, explicit name takes precedence.
func (t *TopicTool) resolveTopic(name string, topicID *int64) (*db.Topic, error) {
	name = strings.TrimSpace(name)
	if name != "" {
		topic, err := t.db.GetTopicByName(name)
		if err == db.ErrNotFound {
			return nil, fmt.Errorf("topic not found: %s", name)
		}
		return topic, err
	}
	if topicID != nil {
		topic, err := t.db.GetTopicByID(*topicID)
		if err == db.ErrNotFound {
			return nil, fmt.Errorf("topic not found")
		}
		return topic, err
	}
	return nil, fmt.Errorf("topic name is required when not in a topic chat")
}

func (t *TopicTool) deleteTopic(userID int64, name string, topicID *int64) (string, error) {
	topic, err := t.resolveTopic(name, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can delete a topic")
	}
	if err := t.db.SoftDeleteTopic(topic.ID); err != nil {
		return "", fmt.Errorf("failed to delete topic: %w", err)
	}
	return fmt.Sprintf("Topic %q deleted.", topic.Name), nil
}

func (t *TopicTool) leave(userID int64, name string, topicID *int64) (string, error) {
	topic, err := t.resolveTopic(name, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID == userID {
		return "", fmt.Errorf("the owner cannot leave a topic. Use /topic delete instead")
	}
	isMember, _ := t.db.IsTopicMember(topic.ID, userID)
	if !isMember {
		return "", fmt.Errorf("you are not a member of this topic")
	}
	if err := t.db.RemoveTopicMember(topic.ID, userID); err != nil {
		return "", fmt.Errorf("failed to leave topic: %w", err)
	}
	return fmt.Sprintf("You have left the topic %q.", topic.Name), nil
}

func (t *TopicTool) addMember(userID int64, args string, topicID *int64) (string, error) {
	// Parse: <username> [topic-name]
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", fmt.Errorf("missing username. Usage: /topic add <username> [topic-name]")
	}
	username := parts[0]
	topicName := strings.TrimSpace(strings.TrimPrefix(args, username))

	topic, err := t.resolveTopic(topicName, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can add members")
	}

	targetUser, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if err := t.db.AddTopicMember(topic.ID, targetUser.ID); err != nil {
		// Idempotent — if already a member, that's fine
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Sprintf("%s is already a member of %q.", username, topic.Name), nil
		}
		return "", fmt.Errorf("failed to add member: %w", err)
	}

	return fmt.Sprintf("%s has been added to %q.", username, topic.Name), nil
}

func (t *TopicTool) removeMember(userID int64, args string, topicID *int64) (string, error) {
	// Parse: <username> [topic-name]
	parts := strings.Fields(args)
	if len(parts) == 0 {
		return "", fmt.Errorf("missing username. Usage: /topic remove <username> [topic-name]")
	}
	username := parts[0]
	topicName := strings.TrimSpace(strings.TrimPrefix(args, username))

	topic, err := t.resolveTopic(topicName, topicID)
	if err != nil {
		return "", err
	}
	if topic.OwnerID != userID {
		return "", fmt.Errorf("only the topic owner can remove members")
	}

	targetUser, err := t.db.GetUserByUsername(username)
	if err == db.ErrNotFound {
		return "", fmt.Errorf("user not found: %s", username)
	}
	if err != nil {
		return "", err
	}

	if targetUser.ID == userID {
		return "", fmt.Errorf("the owner cannot remove themselves. Use /topic delete instead")
	}

	if err := t.db.RemoveTopicMember(topic.ID, targetUser.ID); err != nil {
		return "", fmt.Errorf("failed to remove member: %w", err)
	}

	return fmt.Sprintf("%s has been removed from %q.", username, topic.Name), nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./tools/topic/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add tools/topic/topic.go tools/topic/topic_test.go
git commit -m "feat: add TopicTool with create and list commands"
```

---

### Task 8: Add TopicTool tests for delete, leave, add, remove commands

**Files:**
- Modify: `tools/topic/topic_test.go`

**Step 1: Write tests for all remaining commands**

Add to `tools/topic/topic_test.go`:

```go
func TestTopicTool_Delete(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")

	// Delete from within topic chat (no name needed)
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "delete",
		TopicID: &topic.ID,
	})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected deletion confirmation, got: %s", result)
	}

	// Verify deleted
	_, err = coreDB.GetTopicByName("General")
	if err != db.ErrNotFound {
		t.Error("expected topic to be deleted")
	}
}

func TestTopicTool_DeleteByName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})

	// Delete by name (from private chat)
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "delete General"})
	if err != nil {
		t.Fatalf("delete by name failed: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected deletion confirmation, got: %s", result)
	}
}

func TestTopicTool_DeleteNotOwner(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})

	_, err := tool.Execute(ctxForUser(bob.ID, "user"), tools.ExecuteInput{Args: "delete General"})
	if err == nil {
		t.Error("expected error for non-owner delete")
	}
	if !strings.Contains(err.Error(), "owner") {
		t.Errorf("expected owner error, got: %v", err)
	}
}

func TestTopicTool_Leave(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Bob leaves
	result, err := tool.Execute(ctxForUser(bob.ID, "user"), tools.ExecuteInput{
		Args:    "leave",
		TopicID: &topic.ID,
	})
	if err != nil {
		t.Fatalf("leave failed: %v", err)
	}
	if !strings.Contains(result, "left") {
		t.Errorf("expected leave confirmation, got: %s", result)
	}

	// Verify bob is no longer a member
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if isMember {
		t.Error("expected bob to no longer be a member")
	}
}

func TestTopicTool_LeaveOwnerDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "leave",
		TopicID: &topic.ID,
	})
	if err == nil {
		t.Error("expected error when owner tries to leave")
	}
	if !strings.Contains(err.Error(), "owner cannot leave") {
		t.Errorf("expected owner error, got: %v", err)
	}
}

func TestTopicTool_AddMember(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")

	// Add bob from within topic chat
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "add bob",
		TopicID: &topic.ID,
	})
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if !strings.Contains(result, "bob") && !strings.Contains(result, "added") {
		t.Errorf("expected add confirmation, got: %s", result)
	}

	// Verify bob is a member
	bob, _ := coreDB.GetUserByUsername("bob")
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if !isMember {
		t.Error("expected bob to be a member")
	}
}

func TestTopicTool_AddMemberByTopicName(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})

	// Add bob from private chat (specify topic name)
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args: "add bob General",
	})
	if err != nil {
		t.Fatalf("add by topic name failed: %v", err)
	}
	if !strings.Contains(result, "bob") {
		t.Errorf("expected add confirmation, got: %s", result)
	}
}

func TestTopicTool_AddMemberNotOwner(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	coreDB.CreateUserFull("charlie", "hash", "Charlie", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Bob tries to add charlie — should fail
	_, err := tool.Execute(ctxForUser(bob.ID, "user"), tools.ExecuteInput{
		Args:    "add charlie",
		TopicID: &topic.ID,
	})
	if err == nil {
		t.Error("expected error for non-owner add")
	}
}

func TestTopicTool_AddMemberUserNotFound(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "add nonexistent",
		TopicID: &topic.ID,
	})
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestTopicTool_RemoveMember(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	bob, _ := coreDB.CreateUserFull("bob", "hash", "Bob", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")
	coreDB.AddTopicMember(topic.ID, bob.ID)

	// Remove bob
	result, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "remove bob",
		TopicID: &topic.ID,
	})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !strings.Contains(result, "removed") {
		t.Errorf("expected remove confirmation, got: %s", result)
	}

	// Verify bob is removed
	isMember, _ := coreDB.IsTopicMember(topic.ID, bob.ID)
	if isMember {
		t.Error("expected bob to be removed")
	}
}

func TestTopicTool_RemoveSelfDenied(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})
	topic, _ := coreDB.GetTopicByName("General")

	_, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{
		Args:    "remove alice",
		TopicID: &topic.ID,
	})
	if err == nil {
		t.Error("expected error when owner removes self")
	}
}

func TestTopicTool_UnknownCommand(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	user, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	_, err := tool.Execute(ctxForUser(user.ID, "user"), tools.ExecuteInput{Args: "foobar"})
	if err == nil {
		t.Error("expected error for unknown command")
	}
}

func TestTopicTool_NoTopicContext(t *testing.T) {
	coreDB := setupTestDB(t)
	defer coreDB.Close()

	alice, _ := coreDB.CreateUserFull("alice", "hash", "Alice", "user")
	tool := NewTopicTool(coreDB)

	tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "create General"})

	// Try to delete without name and without topic context
	_, err := tool.Execute(ctxForUser(alice.ID, "user"), tools.ExecuteInput{Args: "delete"})
	if err == nil {
		t.Error("expected error when no topic context and no name")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' in error, got: %v", err)
	}
}
```

**Step 2: Run tests**

Run: `go test ./tools/topic/ -v`
Expected: PASS (implementation already covers these commands from Task 7 step 3)

**Step 3: Commit**

```bash
git add tools/topic/topic_test.go
git commit -m "test: add comprehensive TopicTool tests for all commands"
```

---

### Task 9: Register TopicTool and run full test suite

**Files:**
- Modify: `main.go:19-21` (imports), `main.go:61-63` (registration)

**Step 1: Register the tool**

Add import:
```go
"github.com/esnunes/bobot/tools/topic"
```

Add registration after line 63:
```go
registry.Register(topic.NewTopicTool(coreDB))
```

**Step 2: Update chat_test.go if needed**

The `TestChatWebSocket_SlashCommand` test uses `/user list` — this still works. No changes needed.

**Step 3: Run full test suite**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add main.go
git commit -m "feat: register TopicTool in main"
```

---

### Task 10: Final verification and cleanup

**Step 1: Run full test suite one final time**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 2: Build the binary to verify compilation**

Run: `go build -o /dev/null .`
Expected: Success

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: No issues

**Step 4: Commit any remaining changes (if any)**

```bash
git status
```
