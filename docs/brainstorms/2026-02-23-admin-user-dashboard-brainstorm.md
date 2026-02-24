# Admin User Dashboard Brainstorm

**Date:** 2026-02-23
**Status:** Approved

## What We're Building

A user-centric admin UI redesign that replaces the current two-list layout (Users + Topics) with a users-only landing page and a new user detail/dashboard page. After the "unify chats/topics" refactor, every conversation is a topic, making the separate topics list redundant. The admin now enters through users and drills down into their topics.

## Why This Approach

The current admin page lists users and topics separately. Clicking a user shows only their "bobot" topic context. Now that every chat is a topic, this shortcut is limiting — admins need to see all of a user's topics, plus debug info like profiles, skills, push subscriptions, and read status.

A user-centric approach was chosen because:
- The admin primarily thinks in terms of "what is user X doing?" not "what is topic Y?"
- Topics are always reachable through their owner/members
- Keeps a single entry point, reducing UI complexity
- The data volumes (family task assistant, handful of users) don't warrant lazy-loading complexity

## Key Decisions

1. **User-centric navigation** — Admin landing page shows only users. Topics list removed entirely. All topics accessible through user detail pages.

2. **Full user dashboard** — Clicking a user shows a detail page with collapsible `<details>` sections: user info, topics, profile, skills, push subscriptions, read status.

3. **Inline collapsible layout** — All sections render on the user detail page as collapsible sections. User info is always visible; Topics section is open by default; all others collapsed.

4. **Read-only** — No write actions added to the UI. Admin write operations (block/unblock, invites) remain as Bobot slash commands.

5. **Separate context inspector page** — Clicking a topic from the user detail page navigates to the existing `/admin/topics/{id}/context` page. No inline context expansion.

6. **Last message sent** — User info section includes when the user last sent a message, plus total message count.

7. **HTMX navigation** — All navigation uses `hx-get` + `hx-target="body"` body swaps. No URL changes in browser. `bobot:redirect` event for programmatic navigation.

## Design

### Admin Landing Page (`GET /admin`)

- Users list only (topics list removed)
- Each user row: display name, username, role badge, blocked badge, last message date, join date
- Clicking a user row loads `/admin/users/{id}` via HTMX body swap

### User Detail Page (`GET /admin/users/{id}`)

Back button navigates to `/admin` via HTMX.

**Sections:**

1. **User Info** (always visible, not collapsible)
   - Display name, username, role, blocked status
   - Join date, last message sent date, total message count

2. **Topics** (`<details open>`)
   - All topics the user owns or is a member of
   - Each row: topic name, role (owner/member), member count, auto_respond flag, created date
   - Clicking a topic loads `/admin/topics/{id}/context` via HTMX body swap

3. **Profile** (`<details>` collapsed)
   - LLM-generated user profile from `user_profiles` table
   - Raw text display

4. **Skills** (`<details>` collapsed)
   - Skills associated with the user's topics
   - Skill name, trigger, topic name

5. **Push Subscriptions** (`<details>` collapsed)
   - Registered push endpoints
   - Endpoint URL (truncated), user agent, created date

6. **Read Status** (`<details>` collapsed)
   - Per-topic read positions
   - Topic name, last read message ID, timestamp

### Context Inspector Page (`GET /admin/topics/{id}/context`)

Unchanged. Back button updated to return to the originating user detail page.

### Routes

| Route | Change |
|-------|--------|
| `GET /admin` | Modified — topics list removed |
| `GET /admin/users/{id}` | **New** — user detail/dashboard page |
| `GET /admin/users/{id}/context` | **Removed** — replaced by user detail page |
| `GET /admin/topics/{id}/context` | Unchanged |

## Open Questions

None — all questions resolved during brainstorming.
