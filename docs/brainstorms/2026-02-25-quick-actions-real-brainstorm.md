# Quick Actions (Real Implementation) - Brainstorm

**Date:** 2026-02-25
**Issue:** [esnunes/bobot#35](https://github.com/esnunes/bobot/issues/35)
**Prior art:** [esnunes/bobot#31](https://github.com/esnunes/bobot/issues/31) (prototype), `docs/brainstorms/2026-02-25-quick-actions-brainstorm.md`
**Status:** Draft

## What We're Building

A real, persistent quick actions feature that lets users trigger pre-defined prompts with a tap instead of typing repetitive messages. Building on the validated prototype from issue #31, this adds backend persistence, per-topic scoping, CRUD via both settings UI and a `/quickaction` tool/slash command.

**What stays from the prototype:** Lightning bolt trigger button, full-screen overlay, card style (label + message preview), send/fill modes, accessibility.

**What changes:** Actions are stored in the database (not hardcoded), scoped per-topic, manageable by topic owners and admins, visible and usable by all topic members.

## Why This Approach

**Dedicated `quick_actions` table** was chosen because:

- Follows the exact same pattern as the existing `skills` feature (dedicated table, per-topic scoping, tool + UI for CRUD)
- Clean data model with individual IDs for each action (needed for edit/delete)
- Tracks creator (`user_id`) for attribution
- Supports the `/quickaction` tool pattern naturally (same as `/skill`, `/topic`)

Rejected alternatives:
- *Reusing skills*: Poor semantic fit — skills are system-prompt instructions, quick actions are user-facing UI shortcuts
- *JSON blob on topics*: No individual action IDs, harder to validate, doesn't support tool-based management

## Key Decisions

### 1. Per-topic scoping
Each topic has its own set of quick actions. Different topics serve different purposes (groceries, home automation, etc.), so their quick actions should differ. Mirrors how skills already work.

### 2. All members can use, owners + admins can manage
Any topic member can see and trigger quick actions. Only the topic owner and admin users can create, edit, or delete them. Same permission model as skills and topic settings.

### 3. CRUD via settings page
Quick actions management lives in the topic settings page under "Topic Tools", alongside Skills and Schedules. A dedicated list page (`/topics/{id}/quick-actions`) shows all actions with add/edit/delete controls.

### 4. LLM tool + slash command
A `/quickaction` tool (following the `/skill` pattern) lets users manage quick actions through chat. Supports `create`, `update`, `delete`, `list` subcommands. Also available as an LLM tool so bobot can create quick actions on behalf of users.

### 5. Data model: label + message + mode
Three fields per action, same as the prototype:
- **Label**: Short name shown on the card (e.g., "Turn on AC")
- **Message**: The text that gets sent or filled (e.g., "@bobot turn on the AC in the living room")
- **Mode**: `send` (immediate) or `fill` (populate input for editing)

No icons, categories, or ordering — keep it simple. Actions display in creation order.

### 6. Lightning bolt always visible
The button appears for all users on all topics (not gated to admin anymore). If a topic has no quick actions, the overlay shows an empty state with guidance: "No quick actions yet" with a link to settings (for owners/admins) or a hint to ask the topic owner.

### 7. Frontend loads actions from API
Replace the hardcoded `QUICK_ACTIONS` array with a fetch from `GET /api/topics/{id}/quick-actions`. Actions are loaded when the overlay opens (or cached on page load via template data).

## Data Model

```sql
CREATE TABLE IF NOT EXISTS quick_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    topic_id INTEGER NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    message TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'send',  -- 'send' or 'fill'
    user_id INTEGER NOT NULL REFERENCES users(id),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_quick_actions_topic_id ON quick_actions(topic_id);
```

## API Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| GET | `/api/topics/{id}/quick-actions` | List actions for topic | Topic member |
| POST | `/api/topics/{id}/quick-actions` | Create action | Owner/admin |
| PUT | `/api/topics/{id}/quick-actions/{actionId}` | Update action | Owner/admin |
| DELETE | `/api/topics/{id}/quick-actions/{actionId}` | Delete action | Owner/admin |

## Tool Interface

```
/quickaction create "Turn on AC" "@bobot turn on the AC" send
/quickaction update <id> label "New Label"
/quickaction delete <id>
/quickaction list
```

The tool follows the same `ParseArgs` / `Execute` pattern as existing tools. Scoped to the current topic via `ChatData` context.

## UI Changes

### Settings Page
- Add "Quick Actions" row to the "Topic Tools" section (alongside Skills, Schedules)
- Shows count + links to a dedicated list page

### Quick Actions List Page (`/topics/{id}/quick-actions`)
- Lists all quick actions for the topic
- Each row shows label, message preview, mode badge
- Add button opens an inline form or a simple create view
- Edit/delete controls on each row (owner/admin only)

### Chat Overlay
- Remove admin gating from the lightning bolt button — show for all users
- Replace hardcoded `QUICK_ACTIONS` with server-rendered data via `pageData.quick_actions`
- Add empty state for topics with no actions
- Behavior unchanged: send mode sends immediately, fill mode populates input

### Data Loading
Quick actions are server-rendered into the page data JSON (same pattern as messages, auto_respond, etc.). No client-side API fetch needed for the overlay — it renders instantly from `pageData.quick_actions`.

## Files to Touch

| File | Change |
|---|---|
| `db/core.go` | Add `quick_actions` table migration, QuickAction struct, CRUD methods |
| `tools/quickaction/quickaction.go` | New tool: `/quickaction` slash command + LLM tool |
| `server/topics.go` | API handlers for quick action CRUD |
| `server/pages.go` | Quick actions list page handler, pass actions to chat template |
| `web/templates/settings.html` | Add Quick Actions row to Topic Tools section |
| `web/templates/quick_actions.html` | New template: list page with CRUD controls |
| `web/templates/topic_chat.html` | Remove admin gating, update overlay to use dynamic data |
| `web/static/topic_chat.js` | Replace hardcoded array with API fetch, add empty state |
