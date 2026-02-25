---
title: "feat: Quick actions prototype for chat"
type: feat
date: 2026-02-25
issue: https://github.com/esnunes/bobot/issues/31
brainstorm: docs/brainstorms/2026-02-25-quick-actions-brainstorm.md
---

# feat: Quick actions prototype for chat

## Overview

Add a fake/prototype quick actions feature to the chat UI so admin users can evaluate the look and feel before building the real thing. A lightning bolt button in the chat input bar opens a full-screen overlay with hardcoded quick action items. Each action either sends a message immediately via WebSocket or fills the input for editing. Frontend-only — no backend changes, no data persistence.

## Problem Statement / Motivation

Users send repetitive messages throughout the day (turn on/off AC, reminders, etc.). Typing these out every time is tedious. Before investing in the full feature (backend CRUD, per-topic customization, persistence), admins need to evaluate whether the UX concept works well.

## Proposed Solution

### UI Layout

```
┌────────────────────────────────┐
│  Header (topic name)       [⚙] │
├────────────────────────────────┤
│                                │
│  Chat messages...              │
│                                │
├────────────────────────────────┤
│ [@] [Type a message...]  [⚡][➤] │  ← ⚡ = quick actions trigger
└────────────────────────────────┘

Tapping ⚡ opens:

┌────────────────────────────────┐
│  Quick Actions             [X] │
├────────────────────────────────┤
│                                │
│  ┌──────────────────────────┐  │
│  │ Turn on AC               │  │
│  │ @bobot turn on the AC    │  │
│  │ in the living room       │  │
│  └──────────────────────────┘  │
│                                │
│  ┌──────────────────────────┐  │
│  │ Turn off lights           │  │
│  │ @bobot turn off all the  │  │
│  │ lights in the house      │  │
│  └──────────────────────────┘  │
│                                │
│  ┌──────────────────────────┐  │
│  │ Set a reminder ✏️         │  │
│  │ @bobot remind me to      │  │
│  └──────────────────────────┘  │
│                                │
└────────────────────────────────┘
```

### Interaction Behavior

- **Send mode** (default): tap action → message sent via WebSocket → overlay closes
- **Fill mode**: tap action → message fills the input field → overlay closes → user edits and sends
- **Close**: tap X button, tap backdrop, or press Escape → overlay closes
- **WebSocket disconnected**: silent failure (matches existing `sendMessage()` behavior)
- **Fill mode with existing text**: replaces input content (not append)
- **Focus**: returns to lightning bolt button on close; fill mode also focuses the input

### Hardcoded Quick Actions

| Label | Message | Mode |
|---|---|---|
| Turn on AC | @bobot turn on the AC in the living room | send |
| Turn off lights | @bobot turn off all the lights in the house | send |
| Check weather | @bobot what's the weather like today? | send |
| Set a reminder | @bobot remind me to | fill |
| Morning routine | @bobot start my morning routine | send |
| Grocery list | @bobot add to my grocery list: | fill |

## Technical Considerations

### DOM Placement

The overlay HTML goes inside `.chat-container` (not at body level) since `topic_chat.html` defines a single root `{{define "content"}}` block. Using `position: fixed; inset: 0; z-index: var(--z-indices-modal)` ensures it covers the full screen regardless of parent positioning.

### Admin Gating

Both the trigger button and overlay markup are wrapped in `{{if .IsAdmin}}...{{end}}`. No Go handler changes needed — `IsAdmin` is already passed to the template at `server/pages.go:501`.

### CSS Approach

- Reuse `.input-action` for the lightning bolt button (same size/style as send button)
- New `.quick-actions-overlay` class: `position: fixed; inset: 0; z-index: var(--z-indices-modal); background: var(--colors-background)` — fully opaque full-screen panel
- New `.quick-actions-header`: flexbox row with title + close button, styled like existing chat `header`
- New `.quick-actions-list`: scrollable container (`overflow-y: auto; flex: 1`)
- New `.quick-action-item`: card with label (bold) + preview (secondary text), using `var(--colors-surface)` background, `var(--radii-lg)` border radius, `var(--shadows-low)` shadow
- Fill-mode items show a small pencil indicator to visually distinguish from send-mode items

### JS Architecture

All logic lives in `TopicChatClient` class in `topic_chat.js`:

```
const QUICK_ACTIONS = [
  { label: "Turn on AC", message: "@bobot turn on the AC...", mode: "send" },
  { label: "Set a reminder", message: "@bobot remind me to", mode: "fill" },
  // ...
];
```

New methods:
- `setupQuickActions()` — called from constructor, wires up event listeners, renders action items into the overlay list
- `openQuickActions()` — removes `.hidden` from overlay, sets focus into overlay
- `closeQuickActions()` — adds `.hidden`, returns focus to trigger button
- `handleQuickAction(action)` — dispatches based on `action.mode`

### HTMX Body Swap

The overlay is not `hx-preserve`d. If a navigation swap occurs while the overlay is open, it is destroyed — acceptable for a prototype. The `TopicChatClient` constructor re-runs on every swap via `bobot:page-init`, so the overlay is re-initialized.

### Accessibility

- Overlay: `role="dialog"`, `aria-modal="true"`, `aria-label="Quick Actions"`
- Trigger button: `aria-label="Quick actions"`
- Close button: `aria-label="Close"`
- Escape key closes the overlay
- Action items: `role="button"`, `tabindex="0"`, keyboard-activatable

## Acceptance Criteria

### Functional

- [x] Lightning bolt button appears in the chat input bar for admin users only
- [x] Lightning bolt button does NOT appear for non-admin users
- [x] Tapping the lightning bolt opens a full-screen overlay with quick action items
- [x] Each action item shows a label and message preview
- [x] Tapping a "send" mode action sends the message via WebSocket and closes the overlay
- [x] Tapping a "fill" mode action populates the input field and closes the overlay
- [x] Fill mode actions are visually distinguished with a pencil indicator
- [x] Close button (X) closes the overlay
- [x] Tapping the backdrop area closes the overlay (N/A — full-screen opaque panel)
- [x] Escape key closes the overlay
- [x] The same set of 6 hardcoded actions appears in every chat
- [x] Quick actions work in both personal (bobot) and group topic chats

### Non-Functional

- [x] Overlay uses existing design tokens from `tokens.css`
- [x] Button follows `.input-action` pattern (32px, no border, icon only)
- [x] Overlay is scrollable if content exceeds viewport height
- [x] Focus returns to trigger button on close
- [x] `role="dialog"` and `aria-modal="true"` on overlay

## Implementation Phases

### Phase 1: Template + CSS

Files:
- `web/templates/topic_chat.html` — add lightning bolt button in form + overlay HTML skeleton
- `web/static/style.css` — add `.quick-actions-overlay`, `.quick-actions-header`, `.quick-actions-list`, `.quick-action-item` styles

Tasks:
- Add `{{if .IsAdmin}}` wrapped lightning bolt button before the submit button in `#chat-form`
- Add overlay div with header (title + close button) and empty list container inside `.chat-container`
- Style the overlay as a full-screen fixed panel using design tokens
- Style action items as cards with label + preview layout

### Phase 2: JS Logic

Files:
- `web/static/topic_chat.js` — hardcoded data, overlay interaction, action handlers

Tasks:
- Define `QUICK_ACTIONS` constant array at the top of the file
- Add `setupQuickActions()` method to render action items into the overlay list
- Add open/close handlers with `.hidden` toggle
- Add `handleQuickAction()` that calls `wsContainer.send()` for send mode or sets `this.input.value` for fill mode
- Wire up Escape key, backdrop click, and close button
- Add focus management (move focus into overlay on open, return to trigger on close)

## References & Research

### Internal References

- Chat input bar: `web/templates/topic_chat.html:12-18`
- `.input-action` CSS: `web/static/style.css:410-427`
- `.modal` pattern: `web/static/style.css:585-669`
- `sendMessage()`: `web/static/topic_chat.js:130-137`
- `mentionBot()` (fill pattern): `web/static/topic_chat.js:120-128`
- WebSocket send: `web/static/ws-manager.js:87-93`
- Admin gating: `server/pages.go:501` (`IsAdmin: userData.Role == "admin"`)
- Design tokens: `web/static/tokens.css`
- Existing modal toggle: `web/templates/chats.html:44-58`

### Institutional Learnings

- Validate CSS tokens exist in `tokens.css` before use (from `docs/solutions/ui-bugs/invisible-unread-indicator-websocket-sync.md`)
- Use `document`-level event listeners for code in `hx-preserve` elements (same source)
- Render initial state server-side, JS for real-time updates only (from `docs/solutions/architecture-patterns/inconsistent-unread-indicator-rendering-ssr-vs-js.md`)

### Related Issues

- [esnunes/bobot#31](https://github.com/esnunes/bobot/issues/31)
- Brainstorm: `docs/brainstorms/2026-02-25-quick-actions-brainstorm.md`
