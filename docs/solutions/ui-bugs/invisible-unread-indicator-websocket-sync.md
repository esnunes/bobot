---
title: Invisible Unread Indicator and Missing Real-Time WebSocket Sync
category: ui-bugs
severity: medium
component: web/static, server
symptoms:
  - unread dot rendered in HTML but invisible
  - no real-time unread indicators when messages arrive on chat list
  - CSS variable --colors-primary not found
root_cause: invalid CSS token + missing client-side WebSocket event handling
date: 2026-02-20
---

# Invisible Unread Indicator and Missing Real-Time WebSocket Sync

## Problem

Two related issues with unread message indicators:

1. **Invisible dots**: The `.unread-dot` element was rendered in the HTML (server-side template worked correctly) but had no visible background color because the CSS used `var(--colors-primary)`, a token that does not exist in `tokens.css`.

2. **No real-time updates**: When viewing the chat list and a new message arrived via WebSocket, no unread dot appeared dynamically. The dots only showed on full page reload because they depended on server-side rendering in `chats.html`.

## Investigation

### CSS Token Issue

The `.unread-dot` class in `style.css` used `background: var(--colors-primary)`. Inspecting `web/static/tokens.css` revealed no `--colors-primary` token exists. Available color tokens include `--colors-danger`, `--colors-warning`, `--colors-success`, etc.

### Missing Real-Time Sync

The WebSocket manager (`ws-manager.js`) dispatched custom events (`bobot:chat-message`, `bobot:topic-message`, `bobot:chat-read`) but nothing listened to these events to update the chat list UI. The unread dots were only rendered server-side in Go templates.

### Scheduler Read Event Investigation

During testing with scheduled reminders, a `{"topic_id":0,"type":"read"}` WebSocket event appeared between the reminder message and assistant response. Investigation traced through:

- `scheduler/scheduler.go` calls `pipeline.SendPrivateMessage` directly (no mark-as-read)
- `server/pipeline.go` broadcasts messages but does not call `markChatReadImplicit`
- The "read" event originated from `handleChatPage` in `pages.go`, fired when the user previously opened the private chat

This confirmed the scheduler path correctly does NOT mark messages as read.

## Solution

### 1. Fix CSS Token

In `web/static/style.css`, change the invalid token:

```css
/* Before */
.unread-dot {
  background: var(--colors-primary);
}

/* After */
.unread-dot {
  background: var(--colors-danger);
}
```

### 2. Add Real-Time Unread Dot Management

In `web/static/ws-manager.js`, add functions inside the initialized guard (persists across HTMX body swaps since `#ws-connection` has `hx-preserve`):

```javascript
function getCurrentChatId() {
    var chatPage = document.querySelector('[data-page="chat"]');
    if (chatPage) return 0;
    var topicPage = document.querySelector('[data-page="topic-chat"]');
    if (topicPage) return parseInt(topicPage.dataset.topicId, 10);
    return null;
}

function addUnreadDot(chatId) {
    if (getCurrentChatId() === chatId) return; // skip if viewing this chat
    var item = document.querySelector('[data-chat-id="' + chatId + '"]');
    if (!item || item.querySelector('.unread-dot')) return;
    var dot = document.createElement('span');
    dot.className = 'unread-dot';
    var topicRight = item.querySelector('.topic-right');
    if (topicRight) {
        topicRight.insertBefore(dot, topicRight.firstChild);
    } else {
        item.appendChild(dot);
    }
}

function removeUnreadDot(chatId) {
    var item = document.querySelector('[data-chat-id="' + chatId + '"]');
    if (!item) return;
    var dot = item.querySelector('.unread-dot');
    if (dot) dot.remove();
}

document.addEventListener('bobot:chat-message', function() { addUnreadDot(0); });
document.addEventListener('bobot:topic-message', function(e) { addUnreadDot(e.detail.topic_id); });
document.addEventListener('bobot:chat-read', function(e) { removeUnreadDot(e.detail.topic_id); });
```

### 3. Add data-chat-id Attributes to Chat List

In `web/templates/chats.html`, add `data-chat-id` attributes so JavaScript can target the correct elements:

```html
<button hx-get="/chat" hx-target="body" class="bobot-chat-item" data-chat-id="0">
<!-- ... -->
<button hx-get="/chats/{{.ID}}" hx-target="body" class="topic-item" data-chat-id="{{.ID}}">
```

### 4. Move Mark-as-Read to Server-Side Page Handlers

Removed client-side `fetch('/api/chats/{id}/read')` calls from `chat.js` and `topic_chat.js`. Instead, `markChatReadImplicit` is called in `handleChatPage` and `handleTopicChatPage` in `pages.go` when the page is served. The HTTP endpoint `PUT /api/chats/{id}/read` was then removed as dead code.

## Key Design Decisions

- **`getCurrentChatId()` returns `null` on the chat list page**: This ensures dots are always added when on the list view, since `null !== chatId` is always true.
- **Private chat uses `chatId = 0`**: Maps to `db.PrivateChatTopicID` (Go sentinel value stored as NULL in SQLite).
- **Dot insertion respects `.topic-right` container**: For topic items, the dot is inserted inside `.topic-right` before the member count. For the bobot item (no `.topic-right`), appended directly.
- **Event listeners on `document`**: Since `ws-manager.js` runs once and persists via `hx-preserve`, `document`-level listeners survive HTMX body swaps.

## Prevention

- **Validate CSS tokens**: When using `var(--token-name)`, verify the token exists in `tokens.css`. Consider a CSS linting step or a reference comment listing available tokens.
- **Test with real-time scenarios**: Unread indicators require testing with actual WebSocket message delivery, not just page reload verification.
- **HTMX-aware JavaScript**: Code in `hx-preserve` elements persists across swaps but the DOM it targets gets replaced. Use `document`-level event listeners and query selectors that re-find elements after swaps.

## Related Files

- `web/static/tokens.css` - CSS design token definitions
- `web/static/style.css` - `.unread-dot` styles
- `web/static/ws-manager.js` - WebSocket manager with unread dot logic
- `web/templates/chats.html` - Chat list template with `data-chat-id` attributes
- `server/pages.go` - Page handlers with `markChatReadImplicit` calls
- `server/read_status.go` - `markChatReadImplicit` and `broadcastReadEvent` helpers
- `server/chat.go` - WebSocket message handlers with mark-read on user messages
- `db/core.go` - `GetUnreadChats`, `MarkChatRead`, `GetLatestPrivateMessageID`
