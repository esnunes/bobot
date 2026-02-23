---
title: "feat: Add per-topic auto-read toggle"
type: feat
date: 2026-02-23
---

# feat: Add per-topic auto-read toggle

## Overview

Add a per-topic setting that automatically marks all incoming messages as read, so the unread dot never appears for that topic. This mirrors the existing per-topic push mute toggle on `topic_members`.

## Problem Statement

Users who passively follow certain topics (e.g. automated notifications, low-priority channels) don't want those topics cluttering their unread indicators. Currently, the only way to clear unreads is to open the topic or send a message. Users need an independent toggle (separate from push mute) to suppress unread indicators per-topic.

## Proposed Solution

Add an `auto_read` boolean column to `topic_members` following the exact same pattern as the `muted` column. When a topic message is broadcast, the server immediately marks it as read for members with auto-read enabled and broadcasts a read event to their connected clients.

## Acceptance Criteria

- [x] New `auto_read` column on `topic_members` with migration
- [x] `POST /api/topics/{id}/auto-read` enables auto-read for current user
- [x] `DELETE /api/topics/{id}/auto-read` disables auto-read for current user
- [x] Enabling auto-read immediately clears existing unreads for that topic
- [x] New topic messages are auto-marked as read server-side for auto-read members
- [x] Read events are broadcast to auto-read members' connected clients
- [x] `GetUnreadChats` excludes topics where user has `auto_read = true`
- [x] "Auto-read" toggle button visible in topic menu (always visible, not gated by push)
- [x] Toggle updates UI state immediately on success

## Implementation Plan

### Phase 1: Database Layer

**`db/core.go`**

1. Add `AutoRead bool` field to `TopicMember` struct (after `Muted` at line 87)

2. Add migration after the `muted` migration (after line 449):
   ```go
   if err := c.addColumnIfMissing("topic_members", "auto_read", "INTEGER NOT NULL DEFAULT 0"); err != nil {
       return err
   }
   ```

3. Update `GetTopicMembers` query (line 1307) to include `tm.auto_read` in SELECT and add `&m.AutoRead` to Scan (line 1321)

4. Add `SetTopicMemberAutoRead` method (mirror `SetTopicMemberMuted` at line 1329):
   ```go
   func (c *CoreDB) SetTopicMemberAutoRead(topicID, userID int64, autoRead bool) error {
       _, err := c.db.Exec(
           "UPDATE topic_members SET auto_read = ? WHERE topic_id = ? AND user_id = ?",
           autoRead, topicID, userID,
       )
       return err
   }
   ```

5. Update `GetUnreadChats` topic query (line 1605) to filter out auto-read topics:
   ```sql
   JOIN topic_members tm ON tm.topic_id = t.id AND tm.user_id = ? AND tm.auto_read = 0
   ```
   This ensures topics with `auto_read = true` never appear in unread results.

### Phase 2: Server Handler and Routes

**`server/topics.go`**

Add `handleToggleTopicAutoRead` (mirror `handleToggleTopicMute` at line 231). Key difference: on POST (enable), also call `markChatReadImplicit` to clear existing unreads:

```go
func (s *Server) handleToggleTopicAutoRead(w http.ResponseWriter, r *http.Request) {
    userData := auth.UserDataFromContext(r.Context())
    topicID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid topic id", http.StatusBadRequest)
        return
    }
    isMember, err := s.db.IsTopicMember(topicID, userData.UserID)
    if err != nil || !isMember {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }
    autoRead := r.Method == http.MethodPost
    if err := s.db.SetTopicMemberAutoRead(topicID, userData.UserID, autoRead); err != nil {
        http.Error(w, "failed to update auto-read", http.StatusInternalServerError)
        return
    }
    if autoRead {
        s.markChatReadImplicit(userData.UserID, topicID)
    }
    w.WriteHeader(http.StatusNoContent)
}
```

**`server/server.go`**

Register routes after the mute routes (after line 104):
```go
s.router.HandleFunc("POST /api/topics/{id}/auto-read", s.sessionMiddleware(s.handleToggleTopicAutoRead))
s.router.HandleFunc("DELETE /api/topics/{id}/auto-read", s.sessionMiddleware(s.handleToggleTopicAutoRead))
```

### Phase 3: Auto-Mark on Message Broadcast

When a topic message is broadcast, auto-mark it as read for members with `AutoRead = true`.

**`server/chat.go`**

Add a helper method on `*Server`:
```go
func (s *Server) autoMarkReadForTopic(topicID int64, members []db.TopicMember) {
    for _, member := range members {
        if member.AutoRead {
            s.markChatReadImplicit(member.UserID, topicID)
        }
    }
}
```

Call `s.autoMarkReadForTopic(topicID, members)` after each `broadcastToTopic` call that creates new messages. The `broadcastToTopic` method already fetches members -- refactor to reuse those members to avoid a redundant DB call. Specifically, change the broadcast sites (lines 134, 149, 170, 205) to:
1. Fetch members once
2. Broadcast to all
3. Auto-mark for auto-read members

**`server/pipeline.go`**

The `ChatPipeline` doesn't have access to `markChatReadImplicit` (which is on `*Server`). Add a similar auto-mark helper directly on `ChatPipeline` that calls `p.db.MarkChatRead` and broadcasts the read event:

```go
func (p *ChatPipeline) autoMarkReadForTopic(topicID int64, members []db.TopicMember) {
    for _, member := range members {
        if !member.AutoRead {
            continue
        }
        latestID, err := p.db.GetLatestTopicMessageID(topicID)
        if err != nil || latestID == 0 {
            continue
        }
        p.db.MarkChatRead(member.UserID, topicID, latestID)
        readEvent, _ := json.Marshal(map[string]any{
            "type":     "read",
            "topic_id": topicID,
        })
        p.connections.Broadcast(member.UserID, readEvent)
    }
}
```

Call after each `broadcastToTopic` in the pipeline (lines 94, 114). Optimize by fetching `GetLatestTopicMessageID` once before the loop.

### Phase 4: Page Data and Template

**`server/pages.go`**

1. Add `AutoRead bool` to `PageData` struct (after `PushMuted` at line 133)

2. In `handleTopicChatPage` member loop (line 422), extract auto-read state:
   ```go
   if m.UserID == userData.UserID {
       pushMuted = m.Muted
       autoRead = m.AutoRead
   }
   ```

3. Pass `AutoRead: autoRead` in the render call (line 481)

**`web/templates/topic_chat.html`**

Add auto-read toggle button after the mute toggle (after line 31):
```html
<button class="menu-item" data-auto-read-toggle data-topic-id="{{.TopicID}}" data-auto-read="{{.AutoRead}}">{{if .AutoRead}}Disable auto-read{{else}}Enable auto-read{{end}}</button>
```

Note: Unlike the mute button, this is NOT gated by push notification status -- it's always visible.

### Phase 5: JavaScript Toggle

**`web/static/topic_chat.js`** (or inline in template, matching existing patterns)

Add auto-read toggle handler. Since this is independent of push notifications, add it directly in `topic_chat.js` or as a standalone section. Follow the same `fetch()` pattern as the mute toggle in `push.js`:

```javascript
var autoReadButtons = document.querySelectorAll("[data-auto-read-toggle]");
autoReadButtons.forEach(function(btn) {
    btn.onclick = function() {
        var topicId = btn.getAttribute("data-topic-id");
        var isAutoRead = btn.getAttribute("data-auto-read") === "true";
        var method = isAutoRead ? "DELETE" : "POST";
        btn.disabled = true;
        fetch("/api/topics/" + topicId + "/auto-read", { method: method })
            .then(function(resp) {
                if (resp.ok) {
                    btn.setAttribute("data-auto-read", isAutoRead ? "false" : "true");
                    btn.textContent = isAutoRead ? "Enable auto-read" : "Disable auto-read";
                }
            })
            .catch(function(err) { console.error("Auto-read toggle failed:", err); })
            .finally(function() { btn.disabled = false; });
    };
});
```

Place this initialization in the appropriate location -- either in `TopicChatClient` constructor or as a standalone script that runs on page load.

## References

- Brainstorm: `docs/brainstorms/2026-02-23-per-topic-auto-read-brainstorm.md`
- Mute toggle pattern: `docs/plans/2026-02-21-feat-per-topic-push-mute-plan.md`
- Key files:
  - `db/core.go:82-88` (TopicMember struct)
  - `db/core.go:446-449` (muted migration)
  - `db/core.go:1305-1327` (GetTopicMembers)
  - `db/core.go:1329-1336` (SetTopicMemberMuted)
  - `db/core.go:1593-1635` (GetUnreadChats)
  - `server/topics.go:231-253` (handleToggleTopicMute)
  - `server/server.go:103-104` (mute routes)
  - `server/chat.go:247-257` (broadcastToTopic on Server)
  - `server/pipeline.go:122-132` (broadcastToTopic on ChatPipeline)
  - `server/read_status.go:1-33` (markChatReadImplicit, broadcastReadEvent)
  - `server/pages.go:108-134` (PageData struct)
  - `web/templates/topic_chat.html:31` (mute button)
  - `web/static/push.js:140-171` (mute JS toggle)
