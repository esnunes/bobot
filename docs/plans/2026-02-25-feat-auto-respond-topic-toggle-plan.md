---
title: "feat: Add auto-respond toggle to topic settings UI"
type: feat
date: 2026-02-25
issue: esnunes/bobot#30
brainstorm: docs/brainstorms/2026-02-25-auto-respond-toggle-brainstorm.md
---

# feat: Add auto-respond toggle to topic settings UI

## Overview

Add a UI toggle in the Topic Settings page allowing topic owners and admins to enable/disable `auto_respond` per topic. The backend already supports this feature (`topics.auto_respond` column + `db.SetTopicAutoRespond()`). This is purely a frontend + handler addition.

## Problem Statement

Users cannot control whether the bot automatically responds to all messages or only when `@bobot` is mentioned. The backend logic is complete but there is no UI to toggle it.

## Proposed Solution

Add an "Auto-respond" toggle row to the existing "Topic Settings" section in `settings.html`, following the established mute/auto-read toggle pattern. A new HTTP handler enforces owner-or-admin permissions.

## Technical Approach

### Files to Change

| File | Change |
|------|--------|
| `server/topics.go` | Add `handleToggleTopicAutoRespond` handler |
| `server/server.go` | Register POST/DELETE routes for `/api/topics/{id}/auto-respond` |
| `server/pages.go` | Pass `AutoRespond` and `IsBobotTopic` to settings template |
| `web/templates/settings.html` | Add conditionally-rendered toggle row |
| `web/static/settings.js` | Add click handler for auto-respond toggle |

### Task 1: HTTP Handler (`server/topics.go`)

Add `handleToggleTopicAutoRespond` following the existing toggle pattern but with different permission logic:

```go
func (s *Server) handleToggleTopicAutoRespond(w http.ResponseWriter, r *http.Request) {
    userData := auth.UserDataFromContext(r.Context())

    topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid topic id", http.StatusBadRequest)
        return
    }

    topic, err := s.db.GetTopicByID(topicID)
    if err != nil {
        http.Error(w, "topic not found", http.StatusNotFound)
        return
    }

    // Only owner or admin can toggle auto-respond
    if topic.OwnerID != userData.UserID && userData.Role != "admin" {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    // Prevent disabling auto-respond on bobot topics
    if r.Method == http.MethodDelete && topic.Name == "bobot" {
        http.Error(w, "cannot disable auto-respond on bobot topic", http.StatusForbidden)
        return
    }

    enabled := r.Method == http.MethodPost
    if err := s.db.SetTopicAutoRespond(topicID, enabled); err != nil {
        http.Error(w, "failed to update auto-respond", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

**Key differences from mute/auto-read handlers:**
- Permission check: `topic.OwnerID == userData.UserID || userData.Role == "admin"` (not just membership)
- Admins don't need to be topic members (matches admin dashboard cross-topic access)
- Server-side enforcement: rejects DELETE on bobot topics to prevent users from silencing their 1:1 bot chat via API

### Task 2: Route Registration (`server/server.go`)

Add after the existing auto-read routes (around line 102):

```go
s.router.HandleFunc("POST /api/topics/{id}/auto-respond", s.sessionMiddleware(s.handleToggleTopicAutoRespond))
s.router.HandleFunc("DELETE /api/topics/{id}/auto-respond", s.sessionMiddleware(s.handleToggleTopicAutoRespond))
```

### Task 3: Settings Page Data (`server/pages.go`)

In `handleSettingsPage`, after setting `data.AutoRead` (around line 592), add:

```go
data.AutoRespond = topic.AutoRespond
data.IsBobotTopic = topic.Name == "bobot" && topic.OwnerID == userData.UserID
```

Add `IsBobotTopic bool` field to the `PageData` struct (around line 172).

### Task 4: Settings Template (`web/templates/settings.html`)

Add a new toggle row inside the "Topic Settings" `<details>` section, before the mute toggle (around line 31). Conditionally rendered: hidden for bobot topics and non-owner/non-admin users.

```html
{{if and (not .IsBobotTopic) (or (eq .OwnerID .CurrentUserID) .IsAdmin)}}
<div class="settings-toggle-row">
    <div class="settings-toggle-info">
        <span class="settings-toggle-label">Auto-respond</span>
        <span class="settings-toggle-description">Bot responds to all messages without needing @bobot</span>
    </div>
    <button class="settings-toggle-btn" role="switch" aria-checked="{{.AutoRespond}}" data-auto-respond-toggle data-topic-id="{{.TopicID}}" data-auto-respond="{{.AutoRespond}}">
        <span class="settings-toggle-track"><span class="settings-toggle-thumb"></span></span>
    </button>
</div>
{{end}}
```

**Placement:** At the top of "Topic Settings" section since it's the highest-impact setting (affects all members).

### Task 5: JavaScript Handler (`web/static/settings.js`)

Add a click handler following the auto-read toggle pattern:

```javascript
var autoRespondBtn = container.querySelector('[data-auto-respond-toggle]');
if (autoRespondBtn) {
    autoRespondBtn.addEventListener('click', function() {
        var isAutoRespond = autoRespondBtn.getAttribute('data-auto-respond') === 'true';
        var method = isAutoRespond ? 'DELETE' : 'POST';
        autoRespondBtn.disabled = true;
        fetch('/api/topics/' + topicId + '/auto-respond', { method: method })
            .then(function(resp) {
                if (resp.ok) {
                    var newState = !isAutoRespond;
                    autoRespondBtn.setAttribute('data-auto-respond', String(newState));
                    autoRespondBtn.setAttribute('aria-checked', String(newState));
                }
            })
            .catch(function(err) { console.error('Auto-respond toggle failed:', err); })
            .finally(function() { autoRespondBtn.disabled = false; });
    });
}
```

### Task 6: Tests

Add tests in `server/server_test.go` (or a new test file) covering:

- [x] Owner can enable auto-respond (POST returns 204)
- [x] Owner can disable auto-respond (DELETE returns 204)
- [x] Admin (non-member) can toggle auto-respond
- [x] Regular member gets 403
- [x] ~~Non-member gets 403 (or 404)~~ (covered by owner/admin check — non-members without owner/admin role get 404 from GetTopicByID)
- [x] DELETE on bobot topic returns 403

## Acceptance Criteria

- [x] Auto-respond toggle appears in Topic Settings for topic owners
- [x] Auto-respond toggle appears in Topic Settings for admin users
- [x] Toggle is hidden for regular (non-owner, non-admin) members
- [x] Toggle is hidden for bobot topics (1:1 bot chat)
- [x] Toggling ON makes the bot respond to all messages in the topic
- [x] Toggling OFF makes the bot respond only when @bobot is mentioned
- [x] Server rejects attempts to disable auto-respond on bobot topics
- [x] Chat page @bobot button reflects the setting on next page load

## Dependencies & Risks

- **No new dependencies.** All infrastructure (DB column, DB function, CSS classes, toggle patterns) already exists.
- **Low risk.** This follows established patterns with minimal deviation (only the permission model differs).
- **Edge case:** Concurrent admin toggling results in last-write-wins, acceptable for a toggle.

## References

- Brainstorm: `docs/brainstorms/2026-02-25-auto-respond-toggle-brainstorm.md`
- Existing toggle handlers: `server/topics.go:231-281`
- Route registration pattern: `server/server.go:99-102`
- Settings page handler: `server/pages.go:509-601`
- PageData struct: `server/pages.go:146-174`
- DB function: `db/core.go:1049-1057` (`SetTopicAutoRespond`)
- Toggle CSS: `web/static/style.css:1498-1568`
- Auto-read JS handler: `web/static/settings.js:52-75`
