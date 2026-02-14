# Reminder & Cron Message UI Components

**Date:** 2026-02-14
**Status:** Brainstorm

## What We're Building

Styled UI components for reminder and cron messages in the chat. Currently, the scheduler sends messages wrapped in `<bobot-remind>` and `<bobot-cron>` XML tags as `user` role messages. Because user messages are rendered as plain `textContent`, the raw tags appear literally in the chat (e.g., `<bobot-remind>pay the bills</bobot-remind>`).

The goal is to parse these tags on the client side and render them as visually distinct message bubbles with icons, labels, and color accents.

## Why This Approach

Client-side parsing in `message-renderer.js` follows the established pattern of `processBobotTags()` which already handles custom `<bobot>` tags for action buttons. This keeps the server simple and avoids changes to the WebSocket message format or database schema.

## Key Decisions

1. **Display style:** Styled user bubble (right-aligned, like regular user messages) with visual indicators — not a system notification card.

2. **Visual indicators:** Icon + label + distinct color accent for each type:
   - **Reminders:** Bell emoji, "Reminder" label, warm color accent
   - **Cron:** Clock emoji, "Scheduled" label, cool color accent

3. **Parsing location:** Client-side JavaScript in `message-renderer.js`. Detect `<bobot-remind>` and `<bobot-cron>` tags in the raw message content, strip the tags, and wrap the inner text in a styled component.

4. **Icons:** Unicode emoji characters (no SVG dependencies).

## Scope

### Files to modify

- **`web/static/message-renderer.js`** — Add a new method to detect and parse `<bobot-remind>` / `<bobot-cron>` tags from user-role message content, returning structured data (type + inner text).
- **`web/static/chat.js`** — In `addMessage()`, before setting `textContent` for user messages, check for reminder/cron tags and render the styled component instead.
- **`web/static/topic_chat.js`** — Same change as `chat.js` for topic chat messages.
- **`web/static/style.css`** — Add CSS for `.message-reminder` and `.message-cron` components (label, icon, color accent).

### Rendering behavior

- User messages containing `<bobot-remind>...</bobot-remind>` get parsed: the inner content is extracted and rendered inside a styled component with a bell icon and "Reminder" label.
- User messages containing `<bobot-cron>...</bobot-cron>` get parsed similarly with a clock icon and "Scheduled" label.
- The message bubble retains the `self` class (right-aligned) since it's still a user-role message.
- Messages without these tags continue to render as plain text (no change).
- Both private chat and topic chat handle these consistently.

### What's NOT in scope

- Server-side changes to the message format or database schema.
- Changes to how the LLM receives or processes these tags.
- New message roles or WebSocket event types.

## Open Questions

None — all key decisions have been made.
