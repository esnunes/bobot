# Unify Private Chats and Topics

**Date:** 2026-02-23
**Issue:** [#24](https://github.com/esnunes/bobot/issues/24)
**Status:** Brainstorm

## What We're Building

Combine private chats (1:1 with Bobot) and topics into a single unified concept. Under the hood, everything becomes a "topic." A private chat is simply a topic with one member. A group chat is a topic with multiple members.

Key changes:
- Every user gets an auto-created "bobot" topic on signup (replaces the old private chat)
- Users can create multiple private topics (multiple conversation threads with Bobot)
- All messages belong to a topic (no more NULL `topic_id`)
- Single code path for WebSocket handling, pipeline, and frontend

## Why This Approach

Both motivations carry equal weight:
1. **Reduce duplication** — The codebase currently maintains parallel paths for private and topic chat (separate handlers, pipeline methods, JS files, templates, API endpoints)
2. **Enable future features** — A unified model makes it easier to add features that span both contexts without implementing them twice

## Key Decisions

### Assistant auto-respond is configurable per topic
- New `auto_respond` boolean column on the `topics` table
- **Default: OFF** for all new topics (assistant requires @bobot mention)
- **Exception:** The auto-created "bobot" topic has auto-respond ON
- This replaces the current hardcoded behavior (always respond in private, @mention in topics)

### Multiple private topics allowed
- Users can create additional private topics beyond the auto-created "bobot" one
- Enables separate conversation threads with the assistant

### Auto-created "bobot" topic is owned by the user
- The user is the `owner_id` of their "bobot" topic
- Consistent with it being the user's personal conversation space

### Migrate existing private messages in-place
- Create a "bobot" topic for each existing user (owned by that user)
- Move their private chat messages into the new topic (set `topic_id`, populate `sender_id`)
- No data loss, seamless transition

### URL scheme: `/chats/{id}` for everything
- All topics accessed via `/chats/{id}`
- Old `/chat` route redirects to the user's "bobot" topic
- Single template and JS file handles all chat views

### Naming: DB stays "topic", UI says "chat"
- Internal code and DB tables keep `topic`/`topics`/`topic_id` naming
- Only user-facing URLs and UI labels use "chat"
- Avoids a massive rename refactor for no functional gain

### Schema: keep `sender_id`, drop `receiver_id`
- `sender_id` tracks who sent each message (user's ID for user messages, BobotUserID=0 for assistant messages)
- `receiver_id` is dropped since all messages belong to a topic
- `topic_id` becomes NOT NULL

### Sidebar: no visual distinction
- All topics in one flat list, sorted by recency
- No separate sections for private vs group topics

## Chosen Approach: Incremental Migration

Break the work into phases, each leaving the app functional:

1. **Schema prep** — Add `auto_respond` to topics, create "bobot" topics for existing users, migrate messages. Then drop `receiver_id` and make `topic_id` NOT NULL only after verifying migration success.
2. **Backend unification** — Merge pipeline methods and WebSocket handlers to use topic-only paths
3. **Frontend unification** — Merge JS files and templates into a single chat view
4. **Cleanup** — Remove dead code paths, unused handlers, old routes

Each phase is independently reviewable and deployable.

## Open Questions

_None — all key decisions resolved during brainstorming and review._
