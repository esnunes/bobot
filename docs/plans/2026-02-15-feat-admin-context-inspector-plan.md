---
title: "feat: Admin Context Inspector"
type: feat
date: 2026-02-15
---

# Admin Context Inspector

## Overview

A read-only admin dashboard that lets administrators inspect the full LLM conversation context for any user's private chat or any topic chat. Shows the system prompt, message history (including tool_use/tool_result blocks), tool definitions, and token usage. Accessible via a new `/admin` route, protected by admin middleware.

## Problem Statement / Motivation

When debugging LLM behavior or investigating user-reported issues, admins currently have no way to see what the LLM actually receives as its context. This means blind troubleshooting — admins can't verify whether the system prompt, tool definitions, skills, or message history are constructed correctly for a given user or topic.

## Proposed Solution

Server-rendered pages using Go templates and HTMX, consistent with the existing app architecture. A new `adminMiddleware` gates access. The context is reconstructed using the same engine functions (`BuildSystemPrompt`, `GetContextMessages`, `ToLLMToolsForRole`) to ensure the inspector shows exactly what the LLM sees.

## Technical Approach

### Key Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Role for `ToLLMToolsForRole` | Inspected user's role | Shows accurate tool set the LLM receives for that user |
| Dashboard user list | All human users (exclude bobot ID 0), including blocked | Admin needs full visibility for debugging |
| Dashboard topic list | Active (non-deleted) topics only | Deleted topics have no active context window |
| "Last activity" column | Show `created_at` as "Joined" | Last activity is not tracked; avoids expensive subquery |
| Missing user/topic ID | Return 404 | Consistent with existing `handleTopicChatPage` pattern |
| Empty `raw_content` | Fall back to `content` field | Handles pre-migration messages gracefully |
| Raw JSON scope | Full LLM provider request structure | More useful for debugging (includes model, max_tokens) |
| Audit logging | Skip for initial implementation | Can be added later if needed |
| Context freshness | Static snapshot, use browser refresh | Sufficient for debugging; avoids complexity |
| Admin link location | Both `chat.html` and `topic_chat.html` menus | Consistent access from any page |

### Architecture

```
/admin (dashboard)
  ├── Lists users (from ListUsers, excluding ID 0)
  └── Lists topics (from new ListAllTopics)

/admin/users/{id}/context (private chat context)
  ├── System prompt (BuildSystemPrompt with user's skills + user's role tools)
  ├── Messages (GetPrivateChatContextMessages)
  ├── Tools (ToLLMToolsForRole with user's role)
  └── Summary (token counts, window info)

/admin/topics/{id}/context (topic chat context)
  ├── System prompt (BuildSystemPrompt with topic's skills + "user" role tools)
  ├── Messages (GetTopicContextMessages)
  ├── Tools (ToLLMToolsForRole with "user" role)
  └── Summary (token counts, window info)
```

### Implementation Phases

#### Phase 1: Backend Foundation

**1a. Add `ListAllTopics` DB method**

File: `db/core.go`

```go
func (c *CoreDB) ListAllTopics() ([]Topic, error) {
    rows, err := c.db.Query(`
        SELECT id, name, owner_id, deleted_at, created_at
        FROM topics WHERE deleted_at IS NULL
        ORDER BY created_at DESC
    `)
    // ...
}
```

**1b. Add `adminMiddleware`**

File: `server/server.go`

```go
func (s *Server) adminMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        userData := auth.UserDataFromContext(r.Context())
        if userData.Role != "admin" {
            http.Error(w, "forbidden", http.StatusForbidden)
            return
        }
        next(w, r)
    }
}
```

Chained inside `sessionMiddleware`: `s.sessionMiddleware(s.adminMiddleware(handler))`.

**1c. Add `InspectContext` method on `Engine`**

File: `assistant/engine.go`

New method that builds the full LLM request payload without calling the LLM. Returns the system prompt, messages (with raw_content parsed), tool definitions, and token summary.

```go
type ContextInspection struct {
    SystemPrompt string
    Messages     []ContextMessage
    Tools        []llm.Tool
    TotalTokens  int
    MaxTokens    int
}

func (e *Engine) InspectPrivateContext(userID int64, role string) (*ContextInspection, error)
func (e *Engine) InspectTopicContext(topicID int64) (*ContextInspection, error)
```

For the raw JSON view, add a method on the LLM provider to build the request payload without sending it, or construct it in the handler from the `ContextInspection` data.

**1d. Add `IsAdmin` to `PageData`**

File: `server/pages.go`

```go
type PageData struct {
    // ... existing fields ...
    IsAdmin bool
}
```

Update `render()` or each handler to set `IsAdmin` based on `userData.Role == "admin"`. Consider setting it in a helper to avoid repetition.

Also add admin-specific view fields:

```go
type PageData struct {
    // ... existing fields ...
    IsAdmin            bool
    AdminUsers         []AdminUserView
    AdminTopics        []AdminTopicView
    ContextInspection  *ContextInspectionView
}
```

**1e. Register routes**

File: `server/server.go` in `routes()`

```go
// Admin routes
s.router.HandleFunc("GET /admin", s.sessionMiddleware(s.adminMiddleware(s.handleAdminPage)))
s.router.HandleFunc("GET /admin/users/{id}/context", s.sessionMiddleware(s.adminMiddleware(s.handleAdminUserContextPage)))
s.router.HandleFunc("GET /admin/topics/{id}/context", s.sessionMiddleware(s.adminMiddleware(s.handleAdminTopicContextPage)))
```

**1f. Add `/admin` to `validateNavigatePath`**

File: `server/pages.go`

Add `/admin` prefix to the valid paths so post-login redirect works.

#### Phase 2: Handlers

**2a. Create `server/admin.go`**

New file with three handler methods:

- `handleAdminPage` — fetches users via `ListUsers()` (filter out ID 0), topics via `ListAllTopics()`, renders `admin` template
- `handleAdminUserContextPage` — parses user ID from path, validates user exists, calls `engine.InspectPrivateContext()`, renders `admin_context` template
- `handleAdminTopicContextPage` — parses topic ID from path, validates topic exists, calls `engine.InspectTopicContext()`, renders `admin_context` template

Error handling:
- Invalid ID format → 400
- User/topic not found → 404 (redirect to `/admin` with error, or show error page)

#### Phase 3: Templates and UI

**3a. Create `web/templates/admin.html`**

Dashboard with two sections: users table and topics table. Each row links via `hx-get` to the context page. Uses the existing list/container CSS patterns (`.skills-container`, `.skills-list`, `.skill-item`).

Header with back button pointing to `/chat`, title "Admin".

**3b. Create `web/templates/admin_context.html`**

Shared template for both user and topic context views. Three collapsible `<details>` sections:

1. **System Prompt** — rendered in a `<pre><code>` block, HTML-escaped
2. **Messages** — list of messages with role badges, content, expandable raw_content
3. **Tools** — list of tool definitions (collapsed by default)

Summary bar showing: total tokens, max threshold, message count.

Toggle button for Structured vs Raw JSON view. The raw JSON is embedded as a hidden `<pre><code>` block, toggled via JavaScript.

Header with back button to `/admin`, title showing user/topic name.

**3c. Register templates in `loadTemplates()`**

File: `server/pages.go`

```go
adminTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin.html")
s.templates["admin"] = adminTmpl

adminContextTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/admin_context.html")
s.templates["admin_context"] = adminContextTmpl
```

**3d. Add admin link to hamburger menus**

Files: `web/templates/chat.html`, `web/templates/topic_chat.html`

```html
{{if .IsAdmin}}
<button class="menu-item" hx-get="/admin" hx-target="body">Admin</button>
{{end}}
```

Add before the Skills button in both templates. Update all handlers that render these templates to set `IsAdmin`.

**3e. Add CSS for admin pages**

File: `web/static/style.css`

Follow existing patterns:
- `.admin-container` mirroring `.skills-container`
- `.admin-list` mirroring `.skills-list`
- `.admin-item` mirroring `.skill-item`
- Role badges (`.role-badge.user`, `.role-badge.assistant`, `.role-badge.admin`)
- Collapsible sections using `<details>`/`<summary>` elements
- Raw JSON view styling (monospace, syntax highlighting via existing `highlight.js`)
- Toggle button styling

**3f. Add JS for view toggle**

Minimal JavaScript (inline or in a small file) to toggle between structured and raw JSON views. No framework needed — just toggling CSS classes/display.

## Acceptance Criteria

- [x] Admin can navigate to `/admin` from the hamburger menu in chat and topic_chat pages
- [x] Non-admin users do not see the "Admin" link in the menu
- [x] Non-admin users receive 403 when navigating to any `/admin/*` route
- [x] Dashboard lists all human users (excluding bobot system user) with display name, role, and joined date
- [x] Dashboard lists all active topics with name, owner, and member count
- [x] Clicking a user row shows their private chat LLM context
- [x] Clicking a topic row shows that topic's LLM context
- [x] Context detail page shows the full system prompt (with skills, tools, profiles)
- [x] Context detail page shows the message history with role badges and token counts
- [x] Context detail page shows tool definitions (collapsible, collapsed by default)
- [x] Context detail page shows a token summary (total tokens, max threshold)
- [x] Toggle switches between structured HTML view and raw JSON view
- [x] Context uses the inspected user's role for tool filtering (not the admin's role)
- [x] Invalid user/topic IDs return appropriate error responses (400/404)
- [x] All user-supplied content is HTML-escaped (no XSS via raw_content)
- [x] Messages with empty `raw_content` fall back to displaying the `content` field

## Dependencies & Risks

**Dependencies:**
- No external dependencies. All changes are within the existing codebase.
- Requires access to engine internals (currently unexported fields). Solution: add public methods to `Engine`.

**Risks:**
- **Engine method exposure**: Adding `InspectPrivateContext`/`InspectTopicContext` increases the Engine's public API surface. Mitigated by making these read-only methods that don't modify state.
- **PageData growth**: Adding admin fields to the shared `PageData` struct. This is an existing pattern (the struct already has topic, skill, and schedule fields). Acceptable trade-off.
- **Template changes**: Modifying `chat.html` and `topic_chat.html` for the admin link requires care to not break existing functionality. The change is additive (conditional `{{if .IsAdmin}}` block).

## References

- Brainstorm: `docs/brainstorms/2026-02-15-admin-context-inspector-brainstorm.md`
- Server routes: `server/server.go:87-163`
- Session middleware: `server/server.go:165-217`
- Page handlers: `server/pages.go`
- Engine.Chat: `assistant/engine.go:87-245`
- BuildSystemPrompt: `assistant/prompt.go:19-47`
- Context messages: `db/core.go:828-862` (private), `db/core.go:1397-1424` (topic)
- User listing: `db/core.go:1018` (ListUsers)
- Template loading: `server/pages.go:81-143`
- CSS tokens: `web/static/tokens.css`
- Existing admin role plan: `docs/plans/2026-01-30-admin-user-roles-implementation.md`
