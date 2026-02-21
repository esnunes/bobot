---
title: "feat: Add unread message indicators to chat list"
type: feat
date: 2026-02-20
brainstorm: docs/brainstorms/2026-02-20-unread-indicators-brainstorm.md
---

# feat: Add unread message indicators to chat list

## Overview

Add a colored dot indicator next to chat names in the sidebar when there are unread messages. The dot appears in real-time via WebSocket when messages arrive in other chats, clears when the user opens that chat, and syncs across devices.

## Problem Statement / Motivation

Users have no way to know which chats have new messages without opening each one. This is especially important as the number of topic chats grows — users need a quick visual cue to know where activity is happening.

## Proposed Solution

Track `last_read_message_id` per user per chat in a new `chat_read_status` table. Compare against each chat's latest message ID to determine unread status. Render a dot in the template, update it in real-time via WebSocket events, and sync across devices by broadcasting "read" events.

## Technical Approach

### Phase 1: Database Layer

**New table** in `db/core.go` `migrate()`:

```sql
CREATE TABLE IF NOT EXISTS chat_read_status (
    user_id INTEGER NOT NULL REFERENCES users(id),
    topic_id INTEGER REFERENCES topics(id),
    last_read_message_id INTEGER NOT NULL DEFAULT 0
);
-- Two partial indexes to enforce uniqueness (SQLite treats NULLs as distinct in UNIQUE):
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_read_status_private
    ON chat_read_status(user_id) WHERE topic_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_read_status_topic
    ON chat_read_status(user_id, topic_id) WHERE topic_id IS NOT NULL;
```

`topic_id = NULL` represents the private Bobot chat. Two partial unique indexes handle uniqueness correctly: one for the private chat (one row per user where topic_id IS NULL) and one for topic chats (one row per user per topic).

**New DB methods on `CoreDB`:**

Define a constant `PrivateChatTopicID int64 = 0` to represent the private Bobot chat (0 is not a valid topic ID).

- `MarkChatRead(userID int64, topicID int64, messageID int64) error` — upsert `chat_read_status` row. Pass `PrivateChatTopicID` (0) for the Bobot chat; the method maps 0 to NULL internally.
- `GetUnreadChats(userID int64) (bobotUnread bool, topicUnreads map[int64]bool, error)` — returns whether Bobot chat is unread + map of topicID → hasUnread. Joins `chat_read_status` against latest message per chat. Missing rows treated as "all read".

**Query for unread status** (used in chat list rendering):

```sql
-- For private chat (topicID IS NULL):
SELECT COALESCE(MAX(m.id), 0) > COALESCE(crs.last_read_message_id, 0) AS has_unread
FROM messages m
LEFT JOIN chat_read_status crs ON crs.user_id = ? AND crs.topic_id IS NULL
WHERE m.topic_id IS NULL
  AND ((m.sender_id = 0 AND m.receiver_id = ?) OR (m.sender_id = ? AND m.receiver_id = 0))

-- For each topic:
SELECT t.id AS topic_id,
       COALESCE(MAX(m.id), 0) > COALESCE(crs.last_read_message_id, 0) AS has_unread
FROM topics t
JOIN topic_members tm ON tm.topic_id = t.id AND tm.user_id = ?
LEFT JOIN messages m ON m.topic_id = t.id
LEFT JOIN chat_read_status crs ON crs.user_id = ? AND crs.topic_id = t.id
WHERE t.deleted_at IS NULL
GROUP BY t.id
```

### Phase 2: Server — Mark as Read Endpoint

**New endpoint** in `server/server.go`:

```
PUT /api/chats/{id}/read
```

Where `{id}` is `0` for the private Bobot chat (mapped to `topic_id = NULL` internally), or the topic ID for topic chats.

**Handler** in a new file `server/read_status.go`:

1. Extract `userData` from context
2. Parse `{id}` from path — `0` means Bobot (NULL topic), otherwise validate topic membership
3. Get the latest message ID for that chat
4. Call `db.MarkChatRead(userID, topicID, latestMessageID)`
5. Broadcast a `{"type": "read", "topic_id": id}` event to the user's other connections
6. Return 204 No Content

### Phase 3: Server — Chat List with Unread Status

**Modify `handleChatsPage`** in `server/pages.go`:

1. Call `db.GetUnreadChats(userID)` to get the unread map
2. Add `HasUnread bool` field to `TopicView` struct
3. Pass `BobotHasUnread bool` in `PageData`
4. Template uses these to conditionally render the dot

### Phase 4: Template + CSS

**Modify `web/templates/chats.html`:**

Add a dot element inside each chat button:

```html
<!-- Bobot entry -->
<button hx-get="/chat" hx-target="body" class="bobot-chat-item">
    <!-- existing content -->
    {{if .BobotHasUnread}}<span class="unread-dot" data-chat-id="0"></span>{{end}}
</button>

<!-- Topic entries -->
{{range .Topics}}
<button hx-get="/chats/{{.ID}}" hx-target="body" class="topic-item">
    <!-- existing content -->
    {{if .HasUnread}}<span class="unread-dot" data-chat-id="{{.ID}}"></span>{{end}}
</button>
{{end}}
```

**CSS** for `.unread-dot`: small colored circle (8px), positioned to the right of the chat name.

### Phase 5: Client-Side — Real-Time Updates

**Modify `ws-manager.js`** to handle a new `"type": "read"` message format. Existing messages don't use a `type` field (they have `role`, `content`, `topic_id`), so this introduces a new convention for control messages vs chat messages:

```javascript
function dispatchMessage(data) {
    if (data.type === 'read') {
        document.dispatchEvent(new CustomEvent('bobot:chat-read', { detail: data }));
        return;
    }
    // ... existing dispatch logic (unchanged)
}
```

**Unread dots are server-rendered only.** When the user navigates to the chat list, `handleChatsPage` renders the correct dot state. There is no client-side JS to manage dots in real-time because the chat list is not in the DOM when viewing a specific chat (HTMX swaps `hx-target="body"`). If a persistent sidebar is added later, `unread.js` can be introduced then.

**Mark as read on chat open:** In `chat.js` init and `topic_chat.js` init, call `PUT /api/chats/{id}/read` to mark the chat as read. This handles the case where the user navigates to a chat that had unread messages.

**Multi-device sync:** When the mark-read broadcast (`bobot:chat-read`) arrives on another device that is viewing the chat list, the page can be refreshed to reflect the change, or simply ignored (the dot will clear on next navigation). Keep it simple — no client-side dot manipulation for now.

### Phase 6: Implicit Mark-Read on Message Send

When a user sends a message in a chat, they're clearly "reading" it. In the server message pipeline (`server/pipeline.go` or `server/chat.go`), after saving a user's message, call `db.MarkChatRead(userID, topicID, newMessageID)` and broadcast the "read" event to other devices.

## Edge Cases

| Case | Behavior |
|------|----------|
| No messages in chat | No dot (nothing to be unread) |
| User sends a message | Implicitly marks chat as read |
| New topic member joins | Set `last_read_message_id` to latest message on join |
| No `chat_read_status` row | Treated as "all read" (no dot) |
| Bobot async response | Dot appears if user navigated away before response arrived |
| HTMX page swap to chat list | Server renders dots correctly; JS attaches listeners |
| WebSocket reconnect | Sync endpoint already exists; chat list re-render shows correct state |
| Multiple tabs same chat | Both tabs call mark-read; idempotent upsert handles it |
| Deleted topic | Not shown in chat list, no unread tracking needed |

## Acceptance Criteria

- [x] Colored dot appears next to Bobot chat when there are unread private messages
- [x] Colored dot appears next to topic chats when there are unread topic messages
- [x] Dot clears when user opens the chat
- [x] Dot appears in real-time when a message arrives in another chat (WebSocket)
- [x] Reading on one device clears dot on all other devices
- [x] Sending a message implicitly marks chat as read
- [x] New topic members don't see old messages as unread
- [x] First-time users see no unread dots
- [x] Tests cover DB methods, API endpoint, and mark-read-on-send

## Implementation Order

1. `db/core.go` — migration + `MarkChatRead` + `GetUnreadChats` methods + tests
2. `server/read_status.go` — PUT endpoint + route registration + tests
3. `server/pages.go` — modify `handleChatsPage` + `TopicView`/`PageData` changes
4. `web/templates/chats.html` + CSS — dot rendering
5. `web/static/ws-manager.js` — dispatch `bobot:chat-read` for new `"type": "read"` messages
6. `web/static/chat.js` + `web/static/topic_chat.js` — mark-read on open
7. `server/chat.go` or `server/pipeline.go` — implicit mark-read on send + broadcast to other devices
8. Integration testing

## Files to Modify

| File | Changes |
|------|---------|
| `db/core.go` | New table migration, new struct, `MarkChatRead`, `GetUnreadChats` |
| `db/core_test.go` | Tests for new DB methods |
| `server/server.go` | Register new route |
| `server/read_status.go` | New file — mark-read handler |
| `server/pages.go` | `TopicView.HasUnread`, `PageData.BobotHasUnread`, updated `handleChatsPage` |
| `web/templates/chats.html` | Unread dot elements |
| `web/static/ws-manager.js` | Dispatch `bobot:chat-read` event for `"type": "read"` messages |
| `web/static/chat.js` | Mark-read on init |
| `web/static/topic_chat.js` | Mark-read on init |
| `server/chat.go` or `server/pipeline.go` | Implicit mark-read on send |
| `server/chat.go` or `server/pipeline.go` | Implicit mark-read on send + broadcast "read" to other devices |
| CSS (inline or separate) | `.unread-dot` styling |

## References

- Brainstorm: `docs/brainstorms/2026-02-20-unread-indicators-brainstorm.md`
- Database patterns: `db/core.go:117-413` (migrations)
- WebSocket dispatch: `web/static/ws-manager.js:61-71`
- Chat list template: `web/templates/chats.html`
- Chat list handler: `server/pages.go:325-345`
- Connection registry: `server/connections.go`
- Message pipeline: `server/pipeline.go`
