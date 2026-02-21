# Unread Message Indicators

**Date:** 2026-02-20
**Status:** Brainstorm

## What We're Building

A visual indicator (colored dot) in the chat list sidebar that shows when a private chat (Bobot) or topic chat has unread messages. The indicator appears in real-time when new messages arrive while the user is viewing a different chat, and clears when the user opens that chat. Read status syncs across devices.

## Why This Approach

**Message ID tracking** over timestamps because:
- SQLite auto-increment IDs are monotonic — simple integer comparison
- No timestamp precision issues
- Fast indexed lookups

**Server-side source of truth** over client-side storage because:
- Multi-device sync requires server state anyway
- Consistent behavior across sessions and devices
- Chat list rendering can include unread status in a single query

## Key Decisions

1. **Read trigger:** Message is considered read when the user opens/views that specific chat
2. **Visual indicator:** Simple colored dot next to the chat name in the sidebar
3. **Real-time updates:** Dot appears instantly via WebSocket when a new message arrives in another chat
4. **Multi-device sync:** Reading on one device clears the dot on all devices via broadcast
5. **Tracking mechanism:** Server-side `last_read_message_id` per user per chat

## Design Overview

### Database

New table `chat_read_status`:
- `user_id` INTEGER NOT NULL
- `topic_id` INTEGER (NULL = private Bobot chat)
- `last_read_message_id` INTEGER NOT NULL DEFAULT 0
- Composite unique key on `(user_id, topic_id)`

### Chat List Rendering

When rendering the chat list (`handleChatsPage`), query each chat's latest message ID and compare against the user's `last_read_message_id`. Pass an "unread" boolean per chat to the template. The template renders a dot when unread is true.

### Mark as Read

When a user opens a chat:
- Client sends a REST API call (`PUT /api/chats/read`) with the chat identifier
- Server updates `chat_read_status.last_read_message_id` to the latest message ID
- Server broadcasts a "read" event via WebSocket to the user's other active connections
- Other devices remove the dot for that chat

### Real-Time Unread Detection

When a new message arrives via WebSocket for a chat the user is NOT currently viewing:
- Client-side JS checks if the message's chat matches the currently active chat
- If not, shows the unread dot on the corresponding chat list entry

### Edge Cases

- **No messages yet:** No dot shown (nothing to be unread about)
- **User's own messages:** Sending a message implicitly marks the chat as read
- **New topic member:** When joining a topic, set `last_read_message_id` to the latest message (don't show old messages as unread)
- **First visit:** If no `chat_read_status` row exists, treat as "all read" (no dot) to avoid overwhelming new users

## Open Questions

None — all key decisions resolved during brainstorming.
