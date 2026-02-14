---
title: Styled UI Components for Reminder & Cron Messages
type: feat
date: 2026-02-14
brainstorm: docs/brainstorms/2026-02-14-reminder-cron-ui-brainstorm.md
---

# Styled UI Components for Reminder & Cron Messages

## Overview

The scheduler wraps reminder and cron messages in `<bobot-remind>` and `<bobot-cron>` XML tags and sends them as `user` role messages. Since user messages are rendered as plain `textContent`, the raw tags appear literally in the chat. This plan adds client-side parsing to render these as styled message bubbles with emoji icons, labels, and color accents.

## Proposed Solution

Add a new `parseScheduledMessage(content)` method to `MessageRenderer` that detects the tags via regex and returns structured data. Modify the `addMessage`/`prependMessage` methods in both `chat.js` and `topic_chat.js` to check for scheduled messages before falling back to plain `textContent`. Add CSS classes for the two message types.

## Acceptance Criteria

- [x] `<bobot-remind>pay the bills</bobot-remind>` renders as a styled bubble with bell emoji and "Reminder" label
- [x] `<bobot-cron>check the weather</bobot-cron>` renders as a styled bubble with clock emoji and "Scheduled" label
- [x] Styled bubbles retain right-alignment (`.self` class) since they are `user` role messages
- [x] Both private chat and topic chat render consistently
- [x] Regular user messages (without tags) are unaffected
- [x] History and prepended messages also render correctly

## Implementation Steps

### Step 1: Add `parseScheduledMessage` to `MessageRenderer`

**File:** `web/static/message-renderer.js`

Add a new method to the `MessageRenderer` object that detects `<bobot-remind>` or `<bobot-cron>` wrappers and extracts the inner content.

```javascript
// message-renderer.js — new method
parseScheduledMessage(content) {
    if (!content) return null;
    var match;
    match = content.match(/^<bobot-remind>([\s\S]*)<\/bobot-remind>$/);
    if (match) return { type: 'reminder', content: match[1] };
    match = content.match(/^<bobot-cron>([\s\S]*)<\/bobot-cron>$/);
    if (match) return { type: 'cron', content: match[1] };
    return null;
}
```

**Returns:** `{ type: 'reminder' | 'cron', content: string }` or `null`.

### Step 2: Add `renderScheduledMessage` to `MessageRenderer`

**File:** `web/static/message-renderer.js`

Add a method that builds the styled DOM structure for a parsed scheduled message.

```javascript
// message-renderer.js — new method
renderScheduledMessage(parsed) {
    var wrapper = document.createElement('div');
    wrapper.className = 'message-scheduled message-scheduled--' + parsed.type;

    var labelEl = document.createElement('div');
    labelEl.className = 'message-scheduled-label';
    if (parsed.type === 'reminder') {
        labelEl.textContent = '\uD83D\uDD14 Reminder';
    } else {
        labelEl.textContent = '\u23F0 Scheduled';
    }
    wrapper.appendChild(labelEl);

    var contentEl = document.createElement('div');
    contentEl.className = 'message-scheduled-content';
    contentEl.textContent = parsed.content;
    wrapper.appendChild(contentEl);

    return wrapper;
}
```

### Step 3: Modify `chat.js` — `addMessage` and `prependMessage`

**File:** `web/static/chat.js`

In `addMessage()` (line 193-194), replace the plain `textContent` fallback with a check for scheduled messages:

```javascript
// Before (line 193-194):
} else {
    msgEl.textContent = content;
}

// After:
} else {
    var scheduled = MessageRenderer.parseScheduledMessage(content);
    if (scheduled) {
        msgEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
    } else {
        msgEl.textContent = content;
    }
}
```

Same change in `prependMessage()` (line 219-220):

```javascript
// Before (line 219-220):
} else {
    msgEl.textContent = content;
}

// After:
} else {
    var scheduled = MessageRenderer.parseScheduledMessage(content);
    if (scheduled) {
        msgEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
    } else {
        msgEl.textContent = content;
    }
}
```

### Step 4: Modify `topic_chat.js` — `addMessage` and `prependMessage`

**File:** `web/static/topic_chat.js`

In `addMessage()` (line 171-173), replace the plain `textContent` fallback:

```javascript
// Before (line 171-173):
} else {
    contentEl.textContent = content;
    msgEl.appendChild(contentEl);
}

// After:
} else {
    var scheduled = MessageRenderer.parseScheduledMessage(content);
    if (scheduled) {
        contentEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
    } else {
        contentEl.textContent = content;
    }
    msgEl.appendChild(contentEl);
}
```

Same change in `prependMessage()` (line 263-265):

```javascript
// Before (line 263-265):
} else {
    contentEl.textContent = content;
    msgEl.appendChild(contentEl);
}

// After:
} else {
    var scheduled = MessageRenderer.parseScheduledMessage(content);
    if (scheduled) {
        contentEl.appendChild(MessageRenderer.renderScheduledMessage(scheduled));
    } else {
        contentEl.textContent = content;
    }
    msgEl.appendChild(contentEl);
}
```

### Step 5: Add CSS styles

**File:** `web/static/style.css`

Add styles after the existing `.bobot-action-btn` section. Use new CSS custom properties for the warm/cool accents, fitting the Catppuccin Latte palette already in `tokens.css`.

```css
/* Scheduled message components (reminders & cron) */
.message-scheduled {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
}

.message-scheduled-label {
    font-size: var(--font-sizes-0);
    font-weight: var(--font-weights-bold);
    letter-spacing: var(--letter-spacings-wide);
    text-transform: uppercase;
}

.message-scheduled-content {
    line-height: var(--line-heights-normal);
}

/* Reminder: warm accent (Catppuccin Peach #fe640b / light bg) */
.message-scheduled--reminder .message-scheduled-label {
    color: #fe640b;
}

.message:has(.message-scheduled--reminder) {
    background: #fef0e4;
    border-left: 3px solid #fe640b;
}

/* Cron/Scheduled: cool accent (Catppuccin Teal #179299 / light bg) */
.message-scheduled--cron .message-scheduled-label {
    color: #179299;
}

.message:has(.message-scheduled--cron) {
    background: #e0f5f5;
    border-left: 3px solid #179299;
}
```

**Note:** The `.message:has(...)` selectors override the default `.message.self` background (`--colors-accent-light`) for scheduled messages, giving them their distinct warm/cool look while retaining right-alignment.

## Files Modified

| File | Change |
|------|--------|
| `web/static/message-renderer.js` | Add `parseScheduledMessage()` and `renderScheduledMessage()` methods |
| `web/static/chat.js` | Modify `addMessage()` and `prependMessage()` else branches |
| `web/static/topic_chat.js` | Modify `addMessage()` and `prependMessage()` else branches |
| `web/static/style.css` | Add `.message-scheduled` component styles |

## Not In Scope

- Server-side changes to message format or database schema
- Changes to how the LLM receives or processes these tags
- New message roles or WebSocket event types
