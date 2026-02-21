---
title: "feat: Add read status indicators and timestamps to admin context pages"
type: feat
date: 2026-02-21
brainstorm: docs/brainstorms/2026-02-21-admin-read-status-timestamps-brainstorm.md
---

# feat: Add read status indicators and timestamps to admin context pages

## Overview

Enhance the admin context inspection pages (`/admin/users/{id}/context` and `/admin/topics/{id}/context`) with two features:

1. **Message timestamps** — display `YYYY-MM-DD HH:MM` on each message
2. **Read status badges** — show which user(s) last read up to each message

## Problem Statement

Admins inspecting user conversations have no visibility into when messages were sent or whether users have actually read them. This makes it difficult to understand conversation timelines and user engagement.

## Proposed Solution

Thread `ID` and `CreatedAt` from `db.Message` through the `ContextMessage` pipeline into the admin template. Add new DB queries for read positions. Render timestamps in message headers and "Read by" badges on the appropriate messages.

## Technical Approach

### Data Pipeline Changes

The current flow strips `ID` and `CreatedAt`:

```
db.Message (has ID, CreatedAt)
  → context/adapter.go (strips them)
    → assistant.ContextMessage (no ID, no CreatedAt)
      → server/admin.go buildContextPageData()
        → ContextMessageView (no ID, no timestamp)
          → admin_context.html
```

After changes:

```
db.Message (has ID, CreatedAt)
  → context/adapter.go (preserves them)
    → assistant.ContextMessage (ID, CreatedAt added)
      → server/admin.go buildContextPageData(readPositions)
        → ContextMessageView (ID, Timestamp, ReadByUsers added)
          → admin_context.html (renders timestamp + badges)
```

### Key Files to Modify

| File | Change |
|------|--------|
| `assistant/engine.go` | Add `ID int64` and `CreatedAt time.Time` to `ContextMessage` struct |
| `context/adapter.go` | Propagate `ID` and `CreatedAt` from `db.Message` to `ContextMessage` |
| `db/core.go` | Add `GetPrivateChatReadPosition()` and `GetTopicReadPositions()` methods |
| `server/pages.go` | Add `ID`, `Timestamp`, `ReadByUsers` fields to `ContextMessageView` |
| `server/admin.go` | Update handlers to fetch read positions; update `buildContextPageData` |
| `web/templates/admin_context.html` | Render timestamps and read badges |
| `web/static/style.css` | Add styles for read badges and timestamps |

### Phase 1: Database — New Read Position Queries

Add two new methods to `CoreDB`:

```go
// db/core.go

// GetPrivateChatReadPosition returns the last_read_message_id for a user's
// private chat with Bobot. Returns 0 if no row exists.
func (c *CoreDB) GetPrivateChatReadPosition(userID int64) (int64, error)
// Query: SELECT last_read_message_id FROM chat_read_status
//        WHERE user_id = ? AND topic_id IS NULL

// ReadPosition holds a user's read position with display info.
type ReadPosition struct {
    UserID      int64
    DisplayName string
    LastReadID  int64
}

// GetTopicReadPositions returns read positions for all members of a topic,
// excluding Bobot (user_id=0). Joins with users table for display names.
func (c *CoreDB) GetTopicReadPositions(topicID int64) ([]ReadPosition, error)
// Query: SELECT crs.user_id, u.display_name, crs.last_read_message_id
//        FROM chat_read_status crs
//        JOIN users u ON crs.user_id = u.id
//        WHERE crs.topic_id = ? AND crs.user_id != 0
```

### Phase 2: Structs — Thread ID and CreatedAt

```go
// assistant/engine.go - ContextMessage
type ContextMessage struct {
    ID         int64
    Role       string
    Content    string
    RawContent string
    CreatedAt  time.Time
}

// server/pages.go - ContextMessageView
type ContextMessageView struct {
    ID          int64
    Role        string
    Content     string
    RawContent  string
    Tokens      int
    ToolBlocks  []ToolBlockView
    Timestamp   string   // "2026-02-21 14:30"
    ReadByUsers []string // ["Alice", "Bob"] or nil
}
```

Update `context/adapter.go` to propagate the fields:

```go
// Both GetContextMessages and GetTopicContextMessages
result[i] = assistant.ContextMessage{
    ID:         m.ID,
    Role:       m.Role,
    Content:    m.Content,
    RawContent: m.RawContent,
    CreatedAt:  m.CreatedAt,
}
```

### Phase 3: Admin Handlers — Fetch and Compute

Update `buildContextPageData` to accept read positions and compute badges:

```go
// Signature change
func buildContextPageData(label string, inspection assistant.ContextInspection,
    model string, readPositions map[int64][]string) PageData
```

The `readPositions` map is keyed by message ID, with values being display name slices.

**Computing the map (in each handler before calling buildContextPageData):**

1. Fetch read positions from DB
2. For each user's `lastReadID`, find the highest context message ID where `msg.ID <= lastReadID`
3. If no context message qualifies (read position is before the context window), skip that user
4. Group users by the resolved message ID

**Private chat handler** (`handleAdminUserContextPage`):
- Call `GetPrivateChatReadPosition(userID)` → single position
- Build `readPositions` map with a single entry (display name from existing user lookup)

**Topic chat handler** (`handleAdminTopicContextPage`):
- Call `GetTopicReadPositions(topicID)` → multiple positions
- Build `readPositions` map grouping users by resolved message ID

### Phase 4: Template — Render Timestamps and Badges

In `admin_context.html`, within the `.context-message-header`:

```html
<!-- Timestamp next to role badge -->
<span class="admin-badge {{.Role}}">{{.Role}}</span>
<span class="context-message-timestamp">{{.Timestamp}}</span>
<span class="context-message-tokens">~{{.Tokens}} tokens</span>
```

After the message content (at the bottom of `.context-message`):

```html
{{if .ReadByUsers}}
<div class="context-read-badge">
  Read by: {{join .ReadByUsers ", "}}
</div>
{{end}}
```

### Phase 5: CSS Styling

```css
/* Timestamp in message header */
.context-message-timestamp {
  font-size: var(--font-sizes-0);
  color: var(--colors-text-secondary);
}

/* Read badge at bottom of message */
.context-read-badge {
  font-size: var(--font-sizes-0);
  color: var(--colors-text-secondary);
  padding: var(--space-1) var(--space-2);
  background: var(--colors-surface-raised);
  border-radius: var(--radii-sm);
  margin-top: var(--space-1);
}
```

## Edge Case Decisions

| Edge Case | Decision |
|-----------|----------|
| User never opened chat (no `chat_read_status` row) | `lastReadID = 0`, user omitted from all badges |
| `lastReadID` before context window (older messages pruned) | Skip user — their read position is not visible |
| `lastReadID` on a filtered-out message (command/system role) | Attach badge to nearest preceding visible message (`msg.ID <= lastReadID`) |
| All messages read (`lastReadID >= max context msg ID`) | Badge on the last message in context |
| Multiple users read to same message | Aggregate: "Read by: Alice, Bob" |
| Many members in topic (>3 read to same message) | Show up to 3 names: "Read by: Alice, Bob, Charlie +12 more" |
| Bobot (user_id=0) | Excluded from read positions entirely |
| Empty context (no messages) | No badges rendered; existing "No messages" empty state |
| Private chat badge text | "Read" (no username — admin already knows whose chat) |
| Topic chat badge text | "Read by: name1, name2" |
| Raw JSON view toggle | Timestamps and read status only in Structured view, not Raw JSON |

## Acceptance Criteria

- [x] Each message on admin context pages shows a timestamp formatted as `YYYY-MM-DD HH:MM`
- [x] Private chat context page shows "Read" badge on the user's last-read message
- [x] Topic chat context page shows "Read by: name1, name2" badges per user's last-read message
- [x] Bobot is excluded from read status badges
- [x] Users who never opened the chat are omitted from badges
- [x] Read positions before the context window are gracefully skipped
- [x] Topic badges truncate after 3 names with "+N more"
- [x] Raw JSON view is unaffected
- [ ] New DB methods have tests covering edge cases (no row, multiple users, Bobot exclusion)

## References

- Brainstorm: `docs/brainstorms/2026-02-21-admin-read-status-timestamps-brainstorm.md`
- Admin context handler: `server/admin.go:68-197`
- Admin context template: `web/templates/admin_context.html`
- ContextMessage struct: `assistant/engine.go:23-27`
- Context adapter: `context/adapter.go:28-43`
- chat_read_status table: `db/core.go:417-443`
- Read status methods: `db/core.go:1558-1640`
- ContextMessageView: `server/pages.go:83-89`
- Institutional learnings: `docs/solutions/architecture-patterns/admin-context-inspection-dashboard.md`
- Unread indicator patterns: `docs/solutions/ui-bugs/invisible-unread-indicator-websocket-sync.md`
