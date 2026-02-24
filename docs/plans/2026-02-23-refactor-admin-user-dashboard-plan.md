---
title: "refactor: Admin User Dashboard"
type: refactor
date: 2026-02-23
brainstorm: docs/brainstorms/2026-02-23-admin-user-dashboard-brainstorm.md
---

# refactor: Admin User Dashboard

## Overview

Redesign the admin UI to be user-centric after the "unify chats/topics" refactor. Replace the two-list layout (Users + Topics) with a users-only landing page and a new user detail/dashboard page with collapsible sections showing topics, profile, skills, push subscriptions, and read status.

## Problem Statement

After the unify refactor, every conversation is a topic. The admin landing page still lists users and topics separately, and clicking a user only shows their "bobot" topic context. The topics list is redundant (reachable through users), and the user shortcut is limiting (shows only one topic instead of all user data).

## Proposed Solution

Three changes:
1. **Simplify the landing page** — users list only, with "last active" date added
2. **New user detail page** — full dashboard with collapsible `<details>` sections
3. **Update context inspector back button** — return to originating user page via query param

## Resolved Questions from SpecFlow Analysis

These inconsistencies between the brainstorm and the actual DB schema are resolved here:

| Brainstorm says | Reality | Resolution |
|---|---|---|
| Skills show "trigger" | No `trigger` field; `description` exists | Show `description` |
| Push subs show "user agent" | No `user_agent` column in DB | Drop it; show endpoint + created date only |
| Read status shows "timestamp" | No timestamp in `chat_read_status` | Join messages table to get `created_at` of `last_read_message_id` |
| Context inspector back button "updated" | No mechanism to know originating user | Pass `?from={userId}` query param; fallback to `/admin` |

Additional decisions:
- **Skills scoping**: Show skills created by the viewed user (`skills.user_id = ?`), not all skills visible to them
- **User list sort order**: Keep `created_at ASC` (current behavior); last message date is informational
- **Bobot user (ID 0)**: Return 404 if navigated to `/admin/users/0`
- **No messages state**: Show "Never" for last message date, "0" for count
- **Read status**: Show all topics user is a member of; "Never opened" for topics without a `chat_read_status` row

## Technical Approach

### Phase 1: Database Layer

Add new DB methods to `db/core.go`:

**`GetUserMessageStats(userID int64) (lastSentAt *time.Time, totalCount int, err error)`**

```sql
SELECT MAX(created_at), COUNT(*)
FROM messages
WHERE sender_id = ? AND role = 'user'
```

Returns nil `lastSentAt` if no messages exist.

**`GetUserReadPositions(userID int64) ([]UserReadPosition, error)`**

```sql
SELECT crs.topic_id, t.name AS topic_name, crs.last_read_message_id, m.created_at AS read_at
FROM chat_read_status crs
JOIN topics t ON crs.topic_id = t.id
LEFT JOIN messages m ON m.id = crs.last_read_message_id
WHERE crs.user_id = ?
ORDER BY t.name
```

New struct:

```go
type UserReadPosition struct {
    TopicID           int64
    TopicName         string
    LastReadMessageID int64
    ReadAt            *time.Time
}
```

**Files:** `db/core.go`

### Phase 2: View Structs

Add new view structs to `server/pages.go`:

```go
type AdminUserTopicView struct {
    ID          int64
    Name        string
    IsOwner     bool
    MemberCount int
    AutoRespond bool
    CreatedAt   string
}

type AdminUserSkillView struct {
    Name        string
    Description string
    TopicName   string
}

type AdminUserPushSubView struct {
    Endpoint  string // truncated to domain + path prefix
    CreatedAt string
}

type AdminUserReadStatusView struct {
    TopicName         string
    LastReadMessageID int64
    ReadAt            string // formatted from messages.created_at, or "Never opened"
}

type AdminUserDetailView struct {
    ID            int64
    DisplayName   string
    Username      string
    Role          string
    Blocked       bool
    CreatedAt     string
    LastMessageAt string // "Never" if no messages
    MessageCount  int
    Topics        []AdminUserTopicView
    Profile       string // empty if no profile
    Skills        []AdminUserSkillView
    PushSubs      []AdminUserPushSubView
    ReadStatus    []AdminUserReadStatusView
}
```

Add to `PageData`:

```go
AdminUserDetail *AdminUserDetailView
```

**Files:** `server/pages.go`

### Phase 3: Handlers

**Modify `handleAdminPage`** (`server/admin.go`):
- Remove all topic-related code (`ListAllTopics`, `GetTopicMembers`, `GetUserByID` per topic, `AdminTopicView` mapping)
- Add `GetUserMessageStats(user.ID)` call per user to get last message date
- Update `AdminUserView` to include `LastMessageAt string` field
- Remove `AdminTopics` from `PageData`

**New `handleAdminUserPage`** (`server/admin.go`):
- Parse `{id}` from path, reject ID 0 with 404
- Call `GetUserByID(id)` for user info
- Call `GetUserMessageStats(id)` for last message + count
- Call `GetUserTopics(id)` for topics, then `GetTopicMembers` per topic for member count, compare `OwnerID` for `IsOwner`
- Call `GetUserProfile(id)` for profile text
- Iterate user topics, call `GetTopicSkills` per topic, filter by `skill.UserID == id`, collect with topic name
- Call `GetPushSubscriptions(id)` for push subs, truncate endpoint URLs
- Call `GetUserReadPositions(id)` for read status; for topics the user is a member of but has no read row, show "Never opened"
- Render template `"admin_user"` with populated `AdminUserDetailView`

**Modify `handleAdminTopicContextPage`** (`server/admin.go`):
- Read `from` query param: `r.URL.Query().Get("from")`
- Pass `BackURL` to `PageData` (or to `ContextInspectionView`): if `from` is a valid user ID, set to `/admin/users/{from}`; otherwise set to `/admin`

**Remove `handleAdminUserContextPage`** (`server/admin.go`):
- Delete the handler entirely (lines 70-117)

**Files:** `server/admin.go`

### Phase 4: Templates

**Modify `admin.html`** (`web/templates/admin.html`):
- Remove the entire Topics section (`<h2>Topics</h2>` and its `{{range .AdminTopics}}` block)
- Update user item `hx-get` from `/admin/users/{{.ID}}/context` to `/admin/users/{{.ID}}`
- Add "Last active" to each user row, showing `{{.LastMessageAt}}`

**New `admin_user.html`** (`web/templates/admin_user.html`):
- Container with `data-page="admin-user"`
- Header: back button (`hx-get="/admin" hx-target="body"`), title showing user display name
- User Info section (always visible, not collapsible):
  - Display name, username, role badge, blocked badge
  - Join date, last message sent, total message count
- Topics section (`<details open>`):
  - `{{range .AdminUserDetail.Topics}}` rendering each topic as a clickable button
  - `hx-get="/admin/topics/{{.ID}}/context?from={{$.AdminUserDetail.ID}}" hx-target="body"`
  - Each row: topic name, owner/member badge, member count, auto_respond badge, created date
  - Empty state: "No topics" message if list is empty
- Profile section (`<details>`):
  - `{{if .AdminUserDetail.Profile}}` show profile text `{{else}}` show "No profile generated yet" `{{end}}`
- Skills section (`<details>`):
  - `{{range .AdminUserDetail.Skills}}` showing name, description, topic name
  - Empty state: "No skills" if list is empty
- Push Subscriptions section (`<details>`):
  - `{{range .AdminUserDetail.PushSubs}}` showing truncated endpoint, created date
  - Empty state: "No push subscriptions" if list is empty
- Read Status section (`<details>`):
  - `{{range .AdminUserDetail.ReadStatus}}` showing topic name, last read message ID, read date
  - Empty state: "No read data" if list is empty

**Modify `admin_context.html`** (`web/templates/admin_context.html`):
- Change back button from hardcoded `hx-get="/admin"` to `hx-get="{{.Context.BackURL}}"`
- Add `BackURL` field to `ContextInspectionView` struct

**Register new template** in `server/pages.go` `loadTemplates()`:
```go
adminUserTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin_user.html")
s.templates["admin_user"] = adminUserTmpl
```

**Files:** `web/templates/admin.html`, `web/templates/admin_user.html` (new), `web/templates/admin_context.html`

### Phase 5: Routes

**Modify route registration** in `server/server.go`:
- Remove: `s.router.HandleFunc("GET /admin/users/{id}/context", ...)`
- Add: `s.router.HandleFunc("GET /admin/users/{id}", s.sessionMiddleware(s.adminMiddleware(s.handleAdminUserPage)))`
- Keep: `GET /admin` and `GET /admin/topics/{id}/context` unchanged

**Files:** `server/server.go`

### Phase 6: Styling

The new `admin_user.html` template will use the existing CSS patterns:
- `.admin-list` for the overall layout
- `<details>` / `<summary>` for collapsible sections (same as `admin_context.html`)
- Badge styles for role/blocked/owner/member (same as existing admin badges)
- Button styles for topic list items (same as user/topic items on current admin page)

Add minimal CSS to `web/static/styles.css` if needed for:
- User info grid layout (label/value pairs)
- Section spacing between `<details>` blocks

**Files:** `web/static/styles.css`

## Acceptance Criteria

- [x] Admin landing page shows users only (no topics section)
- [x] Each user row shows last message date (or "Never")
- [x] Clicking a user navigates to user detail page via HTMX body swap
- [x] User detail page shows user info (name, username, role, blocked, join date, last message, message count)
- [x] User detail page shows all user topics in an open `<details>` section
- [x] Clicking a topic navigates to context inspector; back button returns to user detail page
- [x] User detail page shows profile in collapsed `<details>` (or "No profile generated yet")
- [x] User detail page shows skills in collapsed `<details>` (or "No skills")
- [x] User detail page shows push subscriptions in collapsed `<details>` (or "No push subscriptions")
- [x] User detail page shows read status in collapsed `<details>` (or "No read data")
- [x] All sections handle empty states gracefully
- [x] Navigating to `/admin/users/0` returns 404
- [x] Old route `/admin/users/{id}/context` is removed
- [x] Context inspector back button returns to `/admin/users/{id}` when navigated from user detail page
- [x] Context inspector back button falls back to `/admin` when no `from` param

## Dependencies & Risks

- **No schema migration needed** — all data exists in current tables; only new queries are added
- **Low risk** — read-only changes, no data modification
- **N+1 queries on user detail page** — acceptable for a handful of users; can optimize later with batch queries if needed

## References

- Brainstorm: `docs/brainstorms/2026-02-23-admin-user-dashboard-brainstorm.md`
- Current admin handlers: `server/admin.go`
- Current admin templates: `web/templates/admin.html`, `web/templates/admin_context.html`
- View structs: `server/pages.go:57-134`
- DB methods: `db/core.go`, `db/skills.go`
- Route registration: `server/server.go:122-125`
- Architecture pattern: `docs/solutions/architecture-patterns/admin-context-inspection-dashboard.md`
