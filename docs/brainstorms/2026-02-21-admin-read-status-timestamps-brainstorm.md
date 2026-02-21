# Admin Read Status Indicators & Message Timestamps

**Date:** 2026-02-21
**Status:** Draft

## What We're Building

Two enhancements to the admin context pages (`/admin/users/{id}/context` and `/admin/topics/{id}/context`):

1. **Last-read indicators**: Visual badges on messages showing which user(s) last read up to that point. For private chats, a single indicator. For topics, badges showing each member's read position.

2. **Message timestamps**: Display date and time (e.g., `2026-02-21 14:35`) on each message in the admin context view.

## Why This Approach

### Read indicators as badges on messages

- **Compact**: Doesn't disrupt the message flow with extra dividers or sidebars
- **Scales naturally**: For private chats, just one indicator; for topics, multiple user pills cluster on the same message if several users read to the same point
- **Intuitive**: Admin can glance at any message and see the read boundary per user
- **Leverages existing data**: The `chat_read_status` table already stores `last_read_message_id` per user/topic

### Date + time format

- Full `YYYY-MM-DD HH:MM` format gives admins precise timing context without ambiguity
- No relative time ("2 hours ago") which would need client-side updates
- Consistent with existing admin date formatting patterns (already uses `2006-01-02`)

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where to show | Existing admin context pages | No need for a new page; enhances the current inspection workflow |
| User-facing timestamps | Not included | Keep scope focused; admin-only for now |
| Read indicator style | Badges/pills on last-read message | Compact, scalable, intuitive |
| Timestamp format | `YYYY-MM-DD HH:MM` | Precise, no client-side updates needed |
| Data source for read status | Existing `chat_read_status` table | Already tracks `last_read_message_id` per user/topic |

## Scope

### In scope
- Add `created_at` timestamp display to each message on admin context pages
- Query `chat_read_status` for the inspected user (private chat) or all topic members (topic chat)
- Render "Read by: username1, username2" badges on the last-read message for each user
- Style badges to be visually distinct but non-intrusive

### Out of scope
- User-facing chat timestamps
- New admin pages
- Real-time updates of read status on admin pages (static on page load is sufficient)
- Read receipts or notifications to users
