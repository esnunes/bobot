---
title: Inconsistent Unread Indicator Rendering (SSR vs JS-only)
category: architecture-patterns
severity: medium
component: server/pages.go, web/static/unread-state.js, web/static/chat.js
symptoms:
  - back-button unread dot not appearing on private chat page initial load
  - inconsistent rendering strategy across pages (SSR on chat list, JS-only on chat pages)
  - race condition between deferred scripts and async page initialization
root_cause: mixed rendering strategies created timing dependency where async init() missed events dispatched by deferred scripts
date: 2026-02-20
---

# Inconsistent Unread Indicator Rendering (SSR vs JS-only)

## Problem

The chat list page rendered unread dots server-side via Go templates (`{{if .HasUnread}}`), but the private chat and topic chat pages relied entirely on JavaScript for the back-button unread indicator. This caused:

1. **Race condition**: In `chat.js`, the `async init()` method awaited `syncMessages()` (a fetch call) before calling `setupEventListeners()`. By the time the `handleUnreadChanged` listener was registered, the deferred `unread-state.js` had already dispatched its initial `bobot:unread-changed` event. The back-button dot never appeared on initial page load.

2. **Architectural inconsistency**: The same concern (unread indicators) used two different rendering strategies across pages, making the system harder to reason about and debug.

## Root Cause

Mixed rendering strategies (SSR + JS-only) introduced a timing dependency between deferred `<head>` scripts and async page initialization.

Execution order on page load:
1. Parser encounters `chat.js` in body → runs → `new ChatClient()` → `init()` starts
2. `await syncMessages()` yields execution (fetch call)
3. Deferred scripts run: `unread-state.js` initializes, dispatches `bobot:unread-changed`
4. Event missed — `setupEventListeners()` hasn't run yet
5. `syncMessages()` completes → `setupEventListeners()` registers listener (too late)

`topic_chat.js` didn't have this problem because its `init()` is synchronous — listeners were registered before deferred scripts ran.

## Solution

Unified all pages to use server-side rendering for initial unread state. JavaScript handles only real-time WebSocket updates.

### 1. Added `HasOtherUnreads` to PageData

```go
type PageData struct {
    // ...
    BobotHasUnread  bool        // For chats list dots
    HasOtherUnreads bool        // For back-button dot on chat pages
    UnreadJSON      template.JS // JSON array for client-side real-time tracking
}
```

### 2. Server computes unread state per page

In `handleChatPage` (on the bobot chat, only topic unreads matter):
```go
bobotUnread, topicUnreads, _ := s.db.GetUnreadChats(userData.UserID)
// ...
HasOtherUnreads: len(topicUnreads) > 0,
```

In `handleTopicChatPage` (exclude current topic, include bobot):
```go
bobotUnread, topicUnreads, _ := s.db.GetUnreadChats(userData.UserID)
otherTopicUnreads := len(topicUnreads)
if topicUnreads[topicID] {
    otherTopicUnreads--
}
// ...
HasOtherUnreads: bobotUnread || otherTopicUnreads > 0,
```

### 3. Templates render the dot server-side

In `chat.html` and `topic_chat.html`, inside the back button:
```html
{{if .HasOtherUnreads}}<span class="unread-dot"></span>{{end}}
```

### 4. Refactored unread-state.js to real-time only

Removed `dispatchChanged()` from initial setup and `htmx:afterSettle` handler. The module only dispatches when WebSocket events change the unread state:

```javascript
// Initial setup — populate set for real-time tracking only
initFromServer();
// No dispatchChanged() — server already rendered correct indicators

// htmx:afterSettle — re-initialize set, no dispatch
document.addEventListener('htmx:afterSettle', function() {
    initFromServer();
});
```

### 5. Refactored to pure function

Changed `unreadStateJSON` (method on Server that queried DB) to `buildUnreadJSON` (pure function that takes data):
```go
func buildUnreadJSON(bobotUnread bool, topicUnreads map[int64]bool) template.JS
```

This avoids duplicate `GetUnreadChats` calls in handlers that already have the data.

## Key Principle

**Pick one rendering strategy per concern and apply it consistently.**

- **Server-side rendering** for initial state: the server has the data, no race conditions, no flash of missing content.
- **JavaScript** for real-time updates only: WebSocket events trigger state changes, pages subscribe and update their own DOM (locality of behavior).

## Prevention

- **Never mix SSR and JS-only rendering for the same UI concern** across different pages. If one page renders a feature server-side, all pages should.
- **Async `init()` in page scripts can miss events** from deferred scripts. If initial state depends on events, either register listeners synchronously in the constructor or render initial state server-side.
- **In HTMX apps, prefer SSR for initial state**: the server knows the full application state at render time. Use JS to enhance with real-time changes, not to compute initial state.

## Related

- [Invisible Unread Indicator and Missing Real-Time WebSocket Sync](../ui-bugs/invisible-unread-indicator-websocket-sync.md) — the CSS token issue and WebSocket event dispatch that preceded this architectural fix
- `docs/brainstorms/2026-02-20-unread-indicators-brainstorm.md` — original feature design
- `docs/plans/2026-02-20-feat-unread-message-indicators-plan.md` — implementation plan

## Affected Files

- `server/pages.go` — `HasOtherUnreads`, `buildUnreadJSON`, page handler changes
- `web/templates/chat.html` — server-rendered dot on back button
- `web/templates/topic_chat.html` — server-rendered dot on back button
- `web/static/unread-state.js` — removed initial/afterSettle dispatches
- `web/static/chat.js` — moved listener back to `setupEventListeners()`
- `web/static/topic_chat.js` — back-button dot listener with cleanup
