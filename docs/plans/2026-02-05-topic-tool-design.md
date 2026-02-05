# Topic Management Tool Design

## Overview

Add a `/topic` slash command tool that lets users manage topics from within any
chat (private or topic). The tool handles: create, delete, leave, add member,
remove member, and list topics.

## Tool Interface Refactor

### New ExecuteInput struct

Replace `map[string]interface{}` with a typed struct:

```go
type ExecuteInput struct {
    Args       string // raw string after tool name, e.g. "create hello world"
    ReceiverID *int64 // set in private chat, nil in topic chat
    TopicID    *int64 // set in topic chat, nil in private chat
}
```

### Updated Tool interface

```go
type Tool interface {
    Name() string
    Description() string
    Schema() interface{}
    Execute(ctx context.Context, input ExecuteInput) (string, error)
    AdminOnly() bool
}
```

### handleSlashCommand simplification

The function in `server/chat.go` stops doing per-command argument parsing.
Instead it:

1. Strips the leading `/` and splits on first space to get tool name
2. Passes everything after the tool name as `Args`
3. Forwards `ReceiverID` and `TopicID` from the chat context

Existing tools (`/user`, `/task`) update to parse `input.Args` internally.

## Database Changes

### New partial unique index (case-insensitive)

```sql
CREATE UNIQUE INDEX idx_topics_name_active ON topics(LOWER(name)) WHERE deleted_at IS NULL
```

Ensures no two active topics share a name (case-insensitive). Deleted topics may
have duplicate names.

### New DB method

`GetTopicByName(name string) (*Topic, error)` — queries:

```sql
SELECT * FROM topics WHERE LOWER(name) = LOWER(?) AND deleted_at IS NULL
```

## Commands

| Command | Syntax | Who can run | Context |
|---------|--------|-------------|---------|
| create | `/topic create <name>` | Any user | Anywhere |
| delete | `/topic delete [name]` | Topic owner | Name optional in topic chat |
| leave | `/topic leave [name]` | Any member except owner | Name optional in topic chat |
| add | `/topic add <username> [topic-name]` | Topic owner | Topic name optional in topic chat |
| remove | `/topic remove <username> [topic-name]` | Topic owner | Topic name optional in topic chat |
| list | `/topic list` | Any user | Anywhere |

### Context resolution

Commands that act on a topic resolve the target topic as follows:

1. If `input.TopicID` is set (running in topic chat) — use current topic
2. If a topic name argument is provided — look up by name (case-insensitive)
3. If neither — return usage error

When in topic chat and a name is also provided, the explicit name takes
precedence.

### Authorization rules

- **create**: Any authenticated user. Creator becomes owner and first member.
- **delete**: Owner only. Soft-deletes the topic.
- **leave**: Any member except the owner. Owner must use delete instead.
- **add**: Owner only. Adds user immediately (idempotent).
- **remove**: Owner only. Cannot remove self (use delete instead).
- **list**: Any user. Shows only topics the user belongs to.

### Error messages

- Missing arguments: `"Usage: /topic create <name>"`
- Not a member: `"You are not a member of this topic"`
- Not the owner: `"Only the topic owner can <action> a topic"`
- Owner trying to leave: `"The owner cannot leave a topic. Use /topic delete instead"`
- User not found: `"User not found: <username>"`
- Topic not found: `"Topic not found: <name>"`
- Duplicate topic name: `"A topic with this name already exists"`

## File Changes

| File | Change |
|------|--------|
| `tools/registry.go` | Add `ExecuteInput` struct, update `Tool` interface |
| `tools/user/user.go` | Update `Execute` to use `ExecuteInput`, parse `Args` |
| `tools/task/task.go` | Update `Execute` to use `ExecuteInput`, parse `Args` |
| `tools/topic/topic.go` | New file — `TopicTool` with all 6 commands |
| `server/chat.go` | Simplify `handleSlashCommand()` to pass raw args + context IDs |
| `db/core.go` | Add `GetTopicByName()`, add unique index in schema init |
| `main.go` | Register `TopicTool` |
| `assistant/engine.go` | Update tool call to use new `ExecuteInput` |

## Response Format

Command responses use the existing `"system"` role. In topic chat, responses are
broadcast to all members via `broadcastToTopic()`. In private chat, responses go
only to the invoking user.

### Output format examples

**`/topic list`**:
```
Your topics:
- General (owner: alice, 3 members)
- Project X (owner: you, 5 members)
```

**`/topic create Design`**:
```
Topic "Design" created.
```

**`/topic add bob`** (in topic chat):
```
bob has been added to this topic.
```

**`/topic leave`** (in topic chat):
```
You have left the topic "General".
```
