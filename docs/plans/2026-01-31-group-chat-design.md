# Group Chat Design

## Overview

Add group chat capability to bobot. Users can create groups, add/remove members, and chat with multiple participants. The assistant is available in groups and responds when mentioned with `@assistant`.

## Key Decisions

- **Dual mode:** Keep existing 1:1 private chat AND add separate group chats
- **Permissions:** Any user can create groups; creator (owner) manages membership
- **History:** New members see full message history
- **Context:** Shared LLM context window per group
- **Navigation:** Separate pages (`/chat`, `/groups`, `/groups/:id`)
- **Assistant trigger:** Mention `@assistant` in message

## Data Model

### New Tables

```sql
CREATE TABLE groups (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    owner_id INTEGER NOT NULL REFERENCES users(id),
    deleted_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE group_members (
    group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, user_id)
);
```

### Modify Existing Messages Table

```sql
ALTER TABLE messages ADD COLUMN group_id INTEGER REFERENCES groups(id);
CREATE INDEX idx_messages_group ON messages(group_id, id) WHERE group_id IS NOT NULL;
```

- `group_id = NULL` → 1:1 private chat (existing behavior)
- `group_id = <id>` → group message

## API Endpoints

### Group Management

```
POST   /api/groups                      - Create a group (name in body)
GET    /api/groups                      - List groups user belongs to
GET    /api/groups/:id                  - Get group details (name, members)
DELETE /api/groups/:id                  - Soft delete group (owner only)

POST   /api/groups/:id/members          - Add member (owner only, username in body)
DELETE /api/groups/:id/members/:userId  - Remove member (owner or self)
```

### Group Messages

```
GET /api/groups/:id/messages/recent   - Recent messages
GET /api/groups/:id/messages/history  - Paginated history
GET /api/groups/:id/messages/sync     - Sync since timestamp
```

### Soft Delete

Groups table has `deleted_at` column. `DELETE /api/groups/:id` sets this timestamp rather than removing the row. All queries filter out deleted groups.

## WebSocket

### Single Connection Per User

Keep existing architecture:
- `ConnectionRegistry` stays as `userID → []WebSocketWriter`
- Single WebSocket endpoint: `/ws/chat?token=JWT` (existing)

### Message Format

Outgoing (user sends):
```json
{"content": "hello", "group_id": 5}      // group message
{"content": "hello", "group_id": null}   // 1:1 with assistant
```

Incoming (server sends):
```json
// Group message
{"group_id": 5, "role": "user", "content": "hello", "user_id": 123, "display_name": "Alice"}
{"group_id": 5, "role": "assistant", "content": "response"}

// 1:1 message (group_id null or omitted)
{"role": "user", "content": "hello"}
{"role": "assistant", "content": "response"}
```

### Broadcast Logic

When a group message is saved:
1. Get all `user_id`s from `group_members` for that `group_id`
2. For each member, call existing `Broadcast(userID, data)`
3. Frontend receives message and can show notification if viewing a different chat

## Assistant Triggering

### Detection

Check if message contains `@assistant` (case-insensitive substring match).

```go
func shouldTriggerAssistant(content string) bool {
    return strings.Contains(strings.ToLower(content), "@assistant")
}
```

### Context Building

Reuse existing `GetContextMessages` pattern filtered by `group_id`:

```go
func (db *CoreDB) GetGroupContextMessages(groupID int64) ([]Message, error)
```

### Message Format for LLM

The assistant sees messages with sender attribution:

```
[Alice]: Hey @assistant, what do you think about this idea?
[Bob]: I agree, curious to hear thoughts
[assistant]: Here's my perspective...
```

System prompt is adjusted to understand it's in a group setting.

### Token Tracking

Same sliding window logic as 1:1, tracked per group. When `context_tokens` exceeds threshold, older messages drop out of context.

## Frontend

### New Pages

- `/groups` - Group list page
- `/groups/:id` - Group chat page

### Group List Page (`/groups`)

- Shows all groups the user belongs to
- Each row: group name, member count, owner indicator
- "Create Group" button
- Click a group → navigate to `/groups/:id`

### Group Chat Page (`/groups/:id`)

- Similar layout to existing `/chat`
- Header shows group name + member count (clickable for member list)
- Message display includes sender name for each message
- Input field with `@assistant` autocomplete hint
- Settings/manage button (visible to owner)

### Navigation

Add nav links in the header:
- "Chat" → `/chat` (1:1 with assistant)
- "Groups" → `/groups`

### New Templates

```
web/templates/groups.html      - Group list page
web/templates/group_chat.html  - Group chat page
```

## Permissions

| Action | Who can do it |
|--------|---------------|
| Create group | Any user |
| View group / messages | Members only |
| Send message | Members only |
| Add member | Owner only |
| Remove member | Owner, or self (to leave) |
| Delete group (soft) | Owner only |

## Edge Cases

- **Owner leaves:** Owner cannot remove themselves. Must delete the group.
- **Blocked user:** Can see existing groups but cannot send messages.
- **Adding non-existent user:** Return 404 error.
- **Adding existing member:** Return 409 conflict or silently succeed (idempotent).

## Validation

- Group name: required, max 100 characters
- Membership: user must exist and not already be a member
