# Per-Topic Auto-Read

**Date:** 2026-02-23
**Status:** Approved

## What We're Building

A per-topic setting that automatically marks all incoming messages as read, so the unread dot never appears for that topic. This mirrors the existing per-topic push mute toggle -- users who want to passively follow a topic without it cluttering their unread indicators can enable "Auto-read" from the topic menu.

## Why This Approach

- Follows the established `muted` column pattern on `topic_members` -- minimal new concepts
- Server-side auto-read means it works across all devices without client-side logic
- Separate from mute so users can independently control push notifications vs. unread dots
- Enabling auto-read immediately clears existing unreads for that topic

## Key Decisions

1. **Separate toggle from mute** -- auto-read and push mute are independent settings
2. **Server-side marking** -- when a topic message is broadcast, the server immediately marks it as read for members with auto-read enabled (they never see the unread dot)
3. **Immediate effect on enable** -- enabling auto-read clears current unread state for that topic right away
4. **Same UI pattern** -- toggle button in topic menu alongside the existing mute toggle

## Design

### Data Layer

- Add `auto_read INTEGER NOT NULL DEFAULT 0` column to `topic_members` (migration via `addColumnIfMissing`)
- Add `AutoRead bool` field to `TopicMember` struct
- Add `SetTopicMemberAutoRead(topicID, userID, autoRead)` DB method
- Update `GetTopicMembers` query and scan to include `auto_read`

### Server

- Routes: `POST /api/topics/{id}/auto-read` and `DELETE /api/topics/{id}/auto-read`
- Handler `handleToggleTopicAutoRead` (mirrors `handleToggleTopicMute`)
- On POST (enable): also call `markChatReadImplicit` to clear existing unreads + broadcast read event
- In `broadcastToTopic` (Server and ChatPipeline): after broadcasting, for each member with `AutoRead = true`, call `MarkChatRead` + broadcast read event
- In `GetUnreadChats`: skip topics where user has `auto_read = true`

### UI

- New "Auto-read" toggle button in topic menu (`topic_chat.html`)
- `data-auto-read` attribute pattern, same as `data-muted`
- JS toggle alongside existing mute toggle logic
- On enable: fire `bobot:chat-read` event locally to clear unread dot immediately

## Open Questions

None.
