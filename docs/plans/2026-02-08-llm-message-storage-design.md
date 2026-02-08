# LLM Message Storage Design

## Problem

Messages are stored in the database as plain text (`content TEXT`). The full LLM exchange — including tool_use and tool_result turns — happens in-memory in `engine.Chat()` and is discarded. This means:

1. The LLM has no memory of having used tools in previous turns
2. There is no way to inspect the full tool loop for debugging

## Goals

- **Full conversation fidelity**: Rebuild exact LLM context including tool calls, so the LLM remembers tool usage across turns
- **Debugging/observability**: Inspect what the LLM returned and the full tool loop

## Design Decisions

- **Dual columns**: Add `raw_content TEXT` alongside existing `content TEXT`. Both written once at insert time (immutable). `content` serves the UI, `raw_content` serves LLM context building.
- **Reuse existing roles**: Keep `user` and `assistant` roles for intermediate tool messages. Differentiation is via `raw_content` structure.
- **Full tool exchange in context**: Always include complete tool_use + tool_result messages when building LLM context. No summarization or collapsing.
- **Token estimation**: Use `len(raw_content) / 4` instead of `len(content) / 4` to account for tool blocks.
- **UI display**: No changes now. Intermediate tool messages stored but UI handling deferred.

## Data Model

### Messages Table Migration

```sql
ALTER TABLE messages ADD COLUMN raw_content TEXT NOT NULL DEFAULT '';
```

Existing rows keep `raw_content = ''`. No backfill needed.

### Column Semantics

| Column | Purpose | Example values |
|--------|---------|----------------|
| `content` | UI display text | `"What's the weather?"`, `"It's 18C..."`, `""` (for tool_result turns) |
| `raw_content` | LLM API format (JSON) | `"What's the weather?"`, `[{"type":"tool_use",...}]`, `[{"type":"tool_result",...}]` |
| `role` | `user` or `assistant` | Same roles for both regular and tool messages |

### Message Examples Per Turn Type

**User text message:**
- `role = "user"`, `content = "What's the weather?"`, `raw_content = "What's the weather?"`

**Assistant with tool_use:**
- `role = "assistant"`, `content = "Let me check..."` (or `""`), `raw_content = [{"type":"text","text":"Let me check..."},{"type":"tool_use","id":"...","name":"get_weather","input":{...}}]`

**Tool result (fed back to LLM):**
- `role = "user"`, `content = ""`, `raw_content = [{"type":"tool_result","tool_use_id":"...","content":"..."}]`

**Final assistant response:**
- `role = "assistant"`, `content = "It's 18C in Paris..."`, `raw_content = [{"type":"text","text":"It's 18C in Paris..."}]`

## Architecture Changes

### Message Persistence Moves to Engine

Currently `server/chat.go` saves the user message and final assistant response. The engine runs the tool loop in-memory.

New flow: the engine (or a collaborating component) persists **each turn** as it happens:

1. User sends message -> `server/chat.go` saves user message (unchanged)
2. `engine.Chat()` calls LLM
3. LLM responds with tool_use -> engine saves assistant message with `raw_content` containing the full content array
4. Engine executes tools -> engine saves tool_result message with `content = ""` and `raw_content` containing the tool_result array
5. Engine sends tool results back to LLM -> repeat from step 3
6. LLM responds with final text -> engine saves assistant message
7. `server/chat.go` no longer saves the final assistant response (engine did it)

The engine needs a `MessageSaver` interface (or similar) injected into it so it can persist messages without depending on the DB directly.

### Context Building

`GetPrivateChatContextMessages` changes:
- Returns **all** messages in the context window (no longer filters to `role IN ('user', 'assistant')` only — it already returns both roles, but now intermediate tool messages are present too)
- Context adapter reads `raw_content` instead of `content`
- Deserializes `raw_content` into `llm.Message.Content` (string or `[]map[string]any`)

### Context Windowing

- Token estimation: `len(raw_content) / 4`
- Cumulative `context_tokens` tracking unchanged
- **Atomic exchanges**: When the window slides, chunk boundaries must land on real user messages, never inside a tool loop. A logical exchange (user message -> tool loop -> final response) is treated as an atomic unit for windowing purposes.

### Topic Chats

Topic chats currently build context as conversation strings (`[UserName]: content`). This design applies to **private chats only** for now. Topic chat changes can follow separately.

## Non-Goals

- Backfilling existing messages with `raw_content`
- UI display of tool messages
- API-based token counting (keep estimation for now)
- Topic chat raw_content support
