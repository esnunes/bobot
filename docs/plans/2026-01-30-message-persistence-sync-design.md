# Message Persistence and Cross-Device Sync Design

## Overview

This design addresses three related concerns for Bobot:

1. **LLM Context** - Provide recent messages (not all) to the LLM
2. **Message History** - Users can scroll back through conversations
3. **Cross-Device Sync** - Messages appear on all user devices

## Scope

- Single-user private chats with Bobot only (no group conversations)
- Single continuous thread per user (future `group_id` column planned)
- Unlimited message history retention

## Database Schema Changes

### Messages table additions

```sql
ALTER TABLE messages ADD COLUMN tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE messages ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 0;
```

- `tokens` - This message's token count (calculated as `len(content) / 4`)
- `context_tokens` - Cumulative tokens since chunk start. Value of `0` marks chunk start.

### Index for context queries

```sql
CREATE INDEX idx_messages_user_context ON messages(user_id, context_tokens)
WHERE context_tokens = 0;
```

Enables fast lookup of chunk start (most recent message where `context_tokens = 0`).

## LLM Context Chunk Strategy

### Concept

- Context window starts at ~30K tokens
- Grows as conversation continues (30K → 40K → ... → 80K)
- When threshold (80K) exceeded, window slides forward to ~30K again
- Chunk state embedded in message data - no separate tracking table

### On message insert

1. Calculate tokens: `len(content) / 4`
2. Get previous message's cumulative context tokens
3. If new total would exceed 80K threshold:
   - Find most recent message with `context_tokens < 50K`
   - Subtract that value from all messages from that point forward (single UPDATE)
   - This sets the new chunk start's `context_tokens` to 0
4. Insert new message with calculated `context_tokens`

### Example

Before (about to exceed 80K):
```
Msg 1: tokens=10K, context_tokens=10K
Msg 2: tokens=15K, context_tokens=25K
Msg 3: tokens=20K, context_tokens=45K  ← most recent < 50K
Msg 4: tokens=15K, context_tokens=60K
Msg 5: tokens=15K, context_tokens=75K
New msg: tokens=10K → would be 85K, exceeds 80K!
```

Reset (subtract 45K from Msg 3 onward):
```sql
UPDATE messages SET context_tokens = context_tokens - 45000
WHERE user_id = ? AND id >= msg3.id
```

After:
```
Msg 3: context_tokens = 0   ← new chunk start
Msg 4: context_tokens = 15K
Msg 5: context_tokens = 30K
New msg: context_tokens = 40K
```

### Retrieving LLM context

1. Find most recent message where `context_tokens = 0`
2. Fetch all messages from that point forward
3. Build message array for LLM API call

## Cross-Device WebSocket Sync

### Connection registry (in-memory)

```go
type ConnectionRegistry struct {
    mu    sync.RWMutex
    conns map[int64][]*websocket.Conn  // userID -> connections
}

func (r *ConnectionRegistry) Add(userID int64, conn *websocket.Conn)
func (r *ConnectionRegistry) Remove(userID int64, conn *websocket.Conn)
func (r *ConnectionRegistry) Broadcast(userID int64, msg []byte)
```

### Broadcast behavior

When a message is saved (both user and assistant), broadcast to all connected devices for that user. Sender also receives broadcast - client handles deduplication.

### Sequential processing

User messages must be processed sequentially per user to maintain context integrity. Concurrent messages would corrupt `context_tokens` calculations. Implement via per-user mutex or channel.

## Reconnect Catch-Up

### Endpoint

```
GET /api/messages/sync?since=<timestamp>
```

### Behavior

- Client stores `lastMessageTimestamp` in localStorage
- On reconnect, fetch messages since that timestamp
- Server applies configurable floor: `MAX(since, now - lookback_limit)`
- Default lookback limit: 24 hours

## Infinite Scroll Pagination

### History endpoint

```
GET /api/messages/history?before=<message_id>&limit=<count>
```

- Cursor-based pagination using message ID
- Returns messages oldest-to-newest for prepending to UI
- Configurable default (50) and max (100) limits

### Recent messages endpoint

```
GET /api/messages/recent?limit=<count>
```

- Used on initial page load
- Returns most recent N messages

### Client behavior

- Page loads → fetch recent messages
- User scrolls to top → fetch older messages via history endpoint
- WhatsApp/iMessage style: newest at bottom, scroll up for older

## Package Structure

```
├── config/
│   └── config.go           # Add new env vars
├── context/                 # NEW: LLM context management
│   └── context.go          # Chunk logic, context retrieval, future caching
├── db/
│   └── core.go             # Add tokens/context_tokens columns, new queries
├── server/
│   ├── chat.go             # Sequential processing, broadcast integration
│   ├── connections.go      # NEW: ConnectionRegistry
│   ├── messages.go         # NEW: REST endpoints (sync, history, recent)
│   └── auth.go             # Unchanged
└── web/
    └── static/
        └── chat.js         # Infinite scroll, reconnect sync, localStorage
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `BOBOT_CONTEXT_TOKENS_START` | 30000 | Target context size after reset |
| `BOBOT_CONTEXT_TOKENS_MAX` | 80000 | Threshold to trigger chunk reset |
| `BOBOT_SYNC_MAX_LOOKBACK` | 24h | Maximum lookback on reconnect sync |
| `BOBOT_HISTORY_DEFAULT_LIMIT` | 50 | Default messages per pagination request |
| `BOBOT_HISTORY_MAX_LIMIT` | 100 | Maximum allowed pagination limit |

## Key Design Decisions

1. **Token estimation via `len(content) / 4`** - Simple approximation, no tokenizer dependency
2. **Chunk state in messages table** - `context_tokens = 0` marks chunk start, no separate tracking
3. **Single UPDATE for window slide** - Efficient recalculation when threshold exceeded
4. **In-memory connection registry** - Simple map for single-server deployment
5. **Timestamp-based sync with cap** - Prevents unbounded history fetch on reconnect
6. **Cursor-based pagination** - Efficient for infinite scroll, no offset issues

## Future Considerations

- `group_id` column for group conversations
- In-memory context caching (avoid re-fetching on each message)
- Redis pub/sub for multi-server scaling
