# Topic Mute Notifications Brainstorm

**Date:** 2026-02-21
**Status:** Approved

## What We're Building

Per-topic push notification mute. Users can mute/unmute push notifications for individual topic (group) chats. When muted, the user stops receiving Web Push notifications for that topic but still sees unread dots and messages in the app.

- Scope: Web Push notifications only (not unread dots)
- Applies to: Topic chats only (private chat with Bobot always follows global push setting)
- Default: Notifications ON for all topics (opt-out model)

## Why This Approach

Adding a `muted` boolean column to the existing `topic_members` table keeps related data together (membership + notification preference in one place). It requires a simple migration and avoids creating a new table for a single boolean preference. The opt-out model (muted defaults to FALSE) means no data migration is needed for existing members.

## Key Decisions

1. **Push only, not unread dots** - Muting a topic suppresses push notifications but unread indicators continue to work normally.
2. **Column on `topic_members`** - Add `muted BOOLEAN NOT NULL DEFAULT FALSE` to the existing join table rather than creating a separate `topic_mutes` table or a generic preferences system.
3. **ON by default** - New topic members get `muted = FALSE`, preserving current behavior. Users opt out per topic.
4. **Topic chats only** - Private chat with Bobot is not mutable; it always follows the global push setting.
5. **UI in topic chat menu only** - The "Mute topic" / "Unmute topic" toggle appears in the topic chat menu, only when global push is enabled.
6. **No visual indicator in chats list** - Muted state is only visible inside the topic chat menu, not on the chats list.

## Design Summary

### Data Layer
- Add `muted BOOLEAN NOT NULL DEFAULT FALSE` to `topic_members` table
- Add `Muted bool` field to `TopicMember` Go struct
- New DB method: `SetTopicMemberMuted(topicID, userID int64, muted bool) error`

### Server Logic
- In `pushToTopicMembers()` (`server/chat.go`): check the `muted` field when iterating members, skip muted members
- New API endpoint: `POST /api/topics/{id}/mute` and `DELETE /api/topics/{id}/mute` (or a single toggle endpoint)

### UI
- Topic chat menu (`topic_chat.html`): add "Mute topic" / "Unmute topic" button
- Only shown when global push is enabled (VAPID key present + user has push subscription)
- Clicking sends request to mute/unmute endpoint, toggles button text
