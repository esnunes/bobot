# Quick Actions Prototype - Brainstorm

**Date:** 2026-02-25
**Issue:** [esnunes/bobot#31](https://github.com/esnunes/bobot/issues/31)
**Status:** Draft

## What We're Building

A fake/prototype quick actions feature for admin users to evaluate the look and feel before building the real thing. Quick actions are pre-defined prompts that users can trigger with a tap instead of typing repetitive messages.

**Scope:** Frontend-only, no backend changes, no data persistence. Hardcoded quick actions visible only to admin users.

## Why This Approach

**Frontend-only with hardcoded JS data** was chosen because:

- Zero backend changes needed for a visual prototype
- Simplest to implement and remove later
- The issue explicitly asks for fake data with no persistence
- When the real version comes, the data source will change anyway (YAGNI)

## Key Decisions

### 1. Trigger: Lightning bolt button in the input bar
A lightning bolt icon button sits on the right side of the chat input bar (alongside the existing send button). It uses the existing `.input-action` CSS pattern.

### 2. Overlay: Full-screen scrollable panel
Tapping the trigger opens a full-screen overlay displaying all quick actions. The overlay has a header ("Quick Actions" + close button) and a scrollable list.

### 3. Card style: Label + message preview
Each quick action in the overlay shows:
- **Label** - a short descriptive name (e.g., "Turn on AC")
- **Preview** - the actual message that will be sent (e.g., "@bobot turn on the AC in the living room")

### 4. Per-action send behavior (configurable)
Each quick action defines its behavior:
- **Send immediately** - taps the action, sends the message, closes the overlay
- **Fill input** - taps the action, puts the text in the input field for editing, closes the overlay

Most actions will be "send immediately."

### 5. Messages are actually sent
The prototype reuses the existing WebSocket `sendMessage()` infrastructure. Quick actions aren't just visual - they actually send messages to the chat.

### 6. Same set of hardcoded actions in every chat
All chats show the same hardcoded quick actions. No per-topic customization in the prototype.

### 7. Admin-only gating
The trigger button is rendered only when `{{if .IsAdmin}}` is true in the template, following the existing admin gating pattern.

## Resolved Questions

1. **What hardcoded quick actions should be included?** A mix of categories - some home automation, some reminders, some general queries to show versatility.

2. **Overlay transition:** No animation - appears instantly. Simplest for a prototype.

## Files to Touch

| File | Change |
|---|---|
| `web/templates/topic_chat.html` | Add lightning bolt trigger button (admin-gated), overlay HTML |
| `web/static/topic_chat.js` | Hardcoded actions data, overlay open/close logic, action tap handlers |
| `web/static/style.css` | Overlay styles, quick action card styles |
