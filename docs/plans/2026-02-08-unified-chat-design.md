# Unified Chat Engine Design

## Goal

Unify `Chat` (private) and `ChatWithContext` (topic) into a single `Chat` method with shared code. The main difference between private and topic chats is that topic chats include all member profiles in the system prompt.

## ChatOptions Struct

```go
type ChatOptions struct {
    Message string
    TopicID int64 // if > 0, topic chat; 0 means private chat
}

func (e *Engine) Chat(ctx context.Context, opts ChatOptions) (string, error)
```

- `UserID` and `Role` come from `auth.UserDataFromContext(ctx)` as they do today.
- `TopicID > 0` means topic chat; `TopicID == 0` (zero value) means private chat.

## Method Flow

1. Build base system prompt via `BuildSystemPrompt()`
2. If private (`TopicID == 0`) -> append single user profile
3. If topic (`TopicID > 0`) -> append all member profiles via `GetTopicMemberProfiles(topicID)`
4. Get context messages (private or topic, based on `TopicID`)
5. Run the LLM + tool loop (shared, up to 10 iterations)
6. Persist messages (private or topic, based on `TopicID`)
7. Return response

## Extended Interfaces

### ContextProvider

```go
type ContextProvider interface {
    GetContextMessages(userID int64) ([]llm.Message, error)
    GetTopicContextMessages(topicID int64) ([]llm.Message, error)
}
```

`GetTopicContextMessages` queries topic messages and returns standard `[]llm.Message`. Attribution is already baked into `raw_content` at save time (see Message Storage below), so no joins with the users table are needed at retrieval time. Parsing logic is fully shared with private chat.

### MessageSaver

```go
type MessageSaver interface {
    SaveMessage(senderID int64, role, content, rawContent string) error
    SaveTopicMessage(topicID, senderID int64, role, content, rawContent string) error
}
```

Parameter renamed from `userID` to `senderID` for consistency with the DB schema.

### ProfileProvider

```go
type ProfileProvider interface {
    GetUserProfile(userID int64) (string, int64, error)
    GetTopicMemberProfiles(topicID int64) (string, error)
}
```

Returns a single formatted string with all member profiles:

```
## Topic Members
The following are the profiles of the members in this topic:

<member name="Alice">
Alice is a morning person who prefers quick updates.
</member>

<member name="Bob">
Bob handles the groceries and school pickups.
</member>
```

## System Prompt

Both chat types use the same `BuildSystemPrompt()` base (with role-filtered tools). The only difference is what profiles are appended:

- **Private**: `## User Profile` section with a single user's profile
- **Topic**: `## Topic Members` section with all member profiles

The `@bobot` filtering is a caller concern — the server filters for mentions before calling the engine. The engine does not need this instruction.

## Message Storage

Two fields serve distinct purposes:

- **`content`**: Clean user text for the UI (rendered with sender info from message metadata)
- **`raw_content`**: LLM-ready text with attribution baked in

For topic user messages, `raw_content` stores `[DisplayName]: <message>`. For assistant messages, `raw_content` stores tool call blocks (same as private chat). This means attribution happens once at save time, not reconstructed at read time.

## Message Attribution

For the new user message being sent in a topic chat, the engine prepends `[DisplayName]:` (from `auth.UserDataFromContext(ctx)`) before sending to the LLM and stores the prepended version in `raw_content`.

## Engine Branching

The engine branches minimally based on `TopicID`:

- **Profile injection**: single user profile vs all member profiles
- **Context retrieval**: `GetContextMessages(userID)` vs `GetTopicContextMessages(topicID)`
- **Message save**: `SaveMessage(senderID, ...)` vs `SaveTopicMessage(topicID, senderID, ...)`
- **New message attribution**: plain text vs `[DisplayName]: text`

All other logic (system prompt base, tool loop, message parsing) is fully shared.

## Callers

### Private chat (server/chat.go)

```go
e.Chat(ctx, assistant.ChatOptions{
    Message: message,
})
```

### Topic chat (server/chat.go)

```go
e.Chat(ctx, assistant.ChatOptions{
    Message: message,
    TopicID: topicID,
})
```

The topic chat handler no longer needs to build formatted conversation arrays, look up display names, or save messages after the call. The engine handles everything.
