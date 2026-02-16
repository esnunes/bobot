# Admin Context Inspector

**Date:** 2026-02-15
**Status:** Draft

## What We're Building

A read-only admin dashboard that lets administrators inspect the full LLM conversation context for any private chat (user) or topic chat. The inspector shows exactly what the LLM sees when generating a response: system prompt, message history (including tool_use/tool_result blocks), and token usage.

## Why This Approach

- **Server-rendered with HTMX** — consistent with the existing app patterns (Go templates, HTMX navigation, minimal JS)
- **Reuses engine logic** — calls the same `BuildSystemPrompt()` and `GetContextMessages()`/`GetTopicContextMessages()` functions the engine uses, ensuring accuracy
- **Read-only** — simpler, safer, and sufficient for debugging/inspection needs
- **Dedicated admin routes** — clean separation from the user-facing chat UI

## Key Decisions

### Routes and Access Control

New admin-only routes protected by an `adminMiddleware` (session check + `role == "admin"`):

- `GET /admin` — Dashboard listing all users and topics
- `GET /admin/users/:id/context` — Private chat context for a specific user
- `GET /admin/topics/:id/context` — Topic chat context for a specific topic

Non-admins receive a 403 response.

### Dashboard Page (`/admin`)

Two sections:

1. **Users table** — columns: display name, role, last activity. Each row links to `/admin/users/:id/context`.
2. **Topics table** — columns: topic name, owner, member count. Each row links to `/admin/topics/:id/context`.

Navigation uses the existing HTMX pattern (`hx-get` with `hx-target="body"`).

### Navigation Entry Point

Admin users see an "Admin" link in the hamburger menu overlay (same pattern as existing Skills, Schedules, Logout links). Non-admin users do not see this link.

### Context Detail Pages

Three collapsible sections:

1. **System Prompt** — the full constructed prompt including built-in skills, user-defined skills, tool definitions, and user/member profiles.
2. **Messages** — the sliding window of context messages. Each message displays:
   - Role badge (user/assistant)
   - Human-readable content
   - Expandable raw_content for tool_use/tool_result blocks
   - Token count
3. **Summary** — total tokens in window, max threshold, window boundaries.

### Display Modes

A toggle button switches between:

- **Structured view** (default) — formatted HTML with collapsible sections, role badges, syntax-highlighted tool blocks
- **Raw JSON view** — the exact JSON payload as the LLM provider would receive it, with syntax highlighting

### Templates

- `admin.html` — Dashboard with user/topic tables
- `admin_context.html` — Context detail view (shared for both user and topic contexts, with conditional rendering)

### Context Building

A new method that constructs the full LLM request payload (system prompt + messages + tools) without calling the LLM. This reuses the existing engine functions:

- `BuildSystemPrompt()` for the system prompt
- `GetContextMessages()` / `GetTopicContextMessages()` for the message window
- `ToLLMToolsForRole()` for the tool definitions

## Open Questions

None — all questions resolved during brainstorming.
