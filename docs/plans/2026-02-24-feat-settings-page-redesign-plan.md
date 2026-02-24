---
title: "feat: Redesign sidebar as dedicated settings page"
type: feat
date: 2026-02-24
issue: https://github.com/esnunes/bobot/issues/28
brainstorm: docs/brainstorms/2026-02-24-settings-page-redesign-brainstorm.md
---

# feat: Redesign sidebar as dedicated settings page

## Overview

Replace the slide-over menu overlay in both the topic chat and chats list views with a dedicated, full settings page. The page uses collapsible `<details>` sections (all open by default) to organize settings into clear groups with descriptive helper text. The page is composable: topic-specific sections appear when there's a topic context, and a global Account section is always present.

## Problem Statement / Motivation

The current slide-over menu (issue [#28](https://github.com/esnunes/bobot/issues/28)) has three problems:

1. **Too many items** — Members list, toggles, navigation links, and destructive actions are crammed into a 240px-wide panel
2. **Unclear context** — Users can't tell which settings affect just this topic vs. the whole app
3. **Poor discoverability** — Important actions are buried; settings lack descriptions explaining what they do

## Proposed Solution

A single settings page template with conditional topic sections, using the existing `<details class="context-section">` pattern from admin pages. Route: `GET /settings?topic_id={id}` (consistent with how `/skills` and `/schedules` handle topic context).

### Page Structure

**Topic Settings Page** (from topic chat, `?topic_id={id}`):

```
[Back ←]        [Settings]        [—]

▾ Topic Details
  Members: Alice, Bob, Charlie

▾ Topic Settings
  Mute                        [toggle]
  Silence push notifications for this topic

  Auto-read                   [toggle]
  Automatically mark messages as read when you open this topic

▾ Topic Tools
  Skills: Weather lookup, Translation
  [Manage Skills →]
  Schedules: Daily standup (9am)
  [Manage Schedules →]

▾ Danger Zone
  [Delete Topic] / [Leave Topic]

────────────────────────────────────

▾ Account
  Display name: [Eduardo        ] [Save]

  Push Notifications              [toggle]
  Enable push notifications for this device

  Admin →                      (admin only)
  [Logout]
```

**Global Settings Page** (from chats list, no `topic_id`):

```
[Back ←]        [Settings]        [—]

▾ Account
  Display name: [Eduardo        ] [Save]

  Push Notifications              [toggle]
  Enable push notifications for this device

  Admin →                      (admin only)
  [Logout]
```

## Technical Considerations

### Architecture

- **Single template** (`settings.html`) with `{{if .TopicID}}` conditionals for topic sections — follows the pattern used by skills/schedules templates
- **Route**: `GET /settings?topic_id={id}` — registered with `sessionMiddleware`, topic_id is optional
- **Back navigation**: When `topic_id` is present, back goes to `/chats/{topicID}`; otherwise back goes to `/chat` (chats list redirect)
- **HTMX body swap**: Standard `hx-get` + `hx-target="body"` navigation, consistent with all other pages

### New API Endpoint

- `POST /api/user/display-name` — updates the current user's display name
- Reuse `validateDisplayName()` from `server/signup.go` (trimmed length >= 1)
- New DB method: `UpdateUserDisplayName(userID int64, displayName string) error`
- Returns 204 on success; HTMX can show inline "Saved" confirmation

### Toggle Behavior

- **Mute & Auto-read**: Keep existing `fetch()` POST/DELETE pattern with `data-*` attributes
- **Push Notifications**: Keep existing service worker + VAPID flow from `push.js`
- **Mute visibility**: Hidden when push notifications are disabled (preserve current behavior)
- **Error handling**: Revert toggle to previous state on failure; log error to console (matches current pattern)

### CSS

- Reuse `.context-section` / `.context-section-title` for collapsible sections
- Reuse `.members-list` / `.member` for member list
- Reuse `.skill-item` pattern for skills/schedules inline previews
- New `.settings-container` page class following existing container pattern
- New `.settings-toggle-row` for toggle + description layout
- New `.settings-separator` for the visual divider between topic and global sections

### State Considerations

- WebSocket connection preserved via `hx-preserve` — messages arriving while on settings are tracked as unread
- Push notification state is determined client-side by `push.js` — works on any page with the toggle button
- Skills/schedules back buttons continue pointing to `/chats/{id}` (not to settings page)
- `validateNavigatePath()` in `server/pages.go` must be updated to include `/settings`

## Acceptance Criteria

### Functional

- [x] Settings icon in topic chat header navigates to `/settings?topic_id={id}`
- [x] Settings icon in chats list header navigates to `/settings`
- [x] Topic sections (Details, Settings, Tools, Danger Zone) appear only when `topic_id` is present
- [x] Account section appears on all settings pages
- [x] All `<details>` sections are open by default
- [x] Members list displays all topic members with display names
- [x] Mute toggle works (POST/DELETE `/api/topics/{id}/mute`)
- [x] Auto-read toggle works (POST/DELETE `/api/topics/{id}/auto-read`)
- [x] Push Notifications toggle works (subscribe/unsubscribe via `push.js`)
- [x] Mute toggle hidden when push notifications are disabled
- [x] Display name editable with Save button, persists via new API
- [x] Inline "Saved" confirmation appears after successful display name update
- [x] Skills/Schedules show inline preview of names with "Manage" links
- [x] Empty state shown when no skills or schedules exist
- [x] Delete Topic button visible only to topic owner
- [x] Leave Topic button visible only to non-owner members
- [x] Admin link visible only to admin users
- [x] Logout works from settings page
- [x] Back button returns to correct page (chat or chats list)
- [x] Old slide-over menu overlay removed from both `topic_chat.html` and `chats.html`

### Non-Functional

- [x] Each toggle has a descriptive helper text explaining what it does
- [x] Page is mobile-friendly (single column, thumb-reachable)
- [x] Toggle controls use ARIA attributes (`role="switch"`, `aria-checked`)
- [x] Sections use proper heading hierarchy

## Dependencies & Risks

- **New DB method needed**: `UpdateUserDisplayName` — straightforward addition to `db/core.go`
- **Push.js refactor**: Toggle logic currently finds buttons by `data-push-toggle` attribute inside the menu overlay; needs to work on the settings page instead
- **topic_chat.js cleanup**: Delete/leave topic handlers and auto-read toggle handler currently reference the menu overlay DOM; need to be moved or adapted for the settings page
- **Menu overlay CSS**: ~50 lines of `.menu-overlay` / `.menu` CSS can be removed after migration

## Implementation Phases

### Phase 1: Backend + Template Foundation

Files to create/modify:
- `db/core.go` — add `UpdateUserDisplayName()` method
- `server/server.go` — add routes: `GET /settings`, `POST /api/user/display-name`
- `server/pages.go` — add settings page handler, update `validateNavigatePath()`, add `loadTemplates` entry
- `web/templates/settings.html` — new template with all sections

### Phase 2: Frontend — Toggles & Interactions

Files to modify:
- `web/static/push.js` — adapt toggle button selectors for settings page
- `web/static/topic_chat.js` — move delete/leave handlers; adapt auto-read toggle
- `web/static/style.css` — add settings-specific styles (toggle rows, separator, container)
- Create `web/static/settings.js` if needed for settings-page-specific JS

### Phase 3: Navigation & Cleanup

Files to modify:
- `web/templates/topic_chat.html` — replace hamburger icon + menu overlay with settings icon link
- `web/templates/chats.html` — replace hamburger icon + menu overlay with settings icon link
- `web/static/style.css` — remove `.menu-overlay` / `.menu` CSS
- `web/static/topic_chat.js` — remove menu open/close handlers

## References & Research

### Internal References

- Brainstorm: `docs/brainstorms/2026-02-24-settings-page-redesign-brainstorm.md`
- `<details>` pattern: `web/templates/admin_user.html:39-99`, `web/templates/admin_context.html:23-90`
- `.context-section` CSS: `web/static/style.css:1278-1307`
- Toggle endpoints: `server/topics.go:231-280`
- Toggle JS: `web/static/push.js:123-171`, `web/static/topic_chat.js:101-124`
- Menu overlay: `web/templates/topic_chat.html:21-43`
- Page handler pattern: `server/skills.go:13-53`
- Template loading: `server/pages.go:193-267`
- Display name validation: `server/signup.go:178-183`
- Existing learnings: `docs/solutions/architecture-patterns/admin-context-inspection-dashboard.md`

### Related Issues

- [#28 — Sidebar is bloated](https://github.com/esnunes/bobot/issues/28)
