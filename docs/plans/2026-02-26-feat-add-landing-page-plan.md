---
title: "feat: Add landing page and privacy policy"
type: feat
date: 2026-02-26
issue: https://github.com/esnunes/bobot/issues/41
brainstorm: docs/brainstorms/2026-02-26-landing-page-brainstorm.md
---

# feat: Add Landing Page and Privacy Policy

## Overview

Add a public landing page at `/` and a privacy policy page at `/privacy` to satisfy Google OAuth consent screen requirements. The login page moves from `/` to `/login`. Authenticated users visiting `/` are redirected to `/chat` via HTTP 302.

## Problem Statement / Motivation

Google OAuth consent screen requires a public-facing homepage and privacy policy URL that are accessible without login. Currently, all pages require authentication except login and signup.

## Proposed Solution

### Route Changes

| Route | Before | After |
|---|---|---|
| `GET /` | Login page | Landing page (302 → `/chat` if authenticated) |
| `POST /` | Login POST handler | **Removed** |
| `GET /login` | — | Login page (moved from `/`) |
| `POST /login` | — | Login POST handler (moved from `/`) |
| `GET /privacy` | — | Privacy policy page |

### Template Architecture

Use the existing `layout.html` for all pages. Add a `Public` boolean to `PageData` so `layout.html` can conditionally skip WebSocket/push scripts on public pages (landing, privacy, login, signup).

```go
// In layout.html:
{{if not .Public}}
<script src="/static/ws-manager.js"></script>
<script src="/static/push.js"></script>
<script src="/static/unread-state.js"></script>
{{end}}
```

This avoids creating a separate `layout_public.html` — simpler and easier to maintain.

### Redirect Mapping

Every reference to `/` in the codebase that assumes it's the login page must be updated:

| File | Current | New | Why |
|---|---|---|---|
| `server/server.go:152-153` | `GET/POST /{$}` → login | `GET /{$}` → landing; add `GET/POST /login` | Route migration |
| `server/pages.go:346` | `HX-Location: /` (logout) | `HX-Location: /login` | Logout should show login, not landing |
| `server/signup.go:157` | `HX-Redirect: /` (signup success) | Render `authenticated` template (same as login success) | User just got a session cookie, skip login |
| `web/templates/login.html:5` | `hx-post="/"` | `hx-post="/login"` | Form target changed |
| `web/static/ws-manager.js:37` | `window.location.href = '/'` | `window.location.href = '/login'` | Auth failure should show login |
| `web/static/sw.js:18` | `url: payload.url \|\| "/"` | `url: payload.url \|\| "/chat"` | Default notification URL should be chat |
| `web/static/sw.js:45` | `openWindow("/?navigate=...")` | `openWindow("/login?navigate=...")` | Preserve navigate-through-auth flow |
| `web/static/manifest.json:5` | `"start_url": "/"` | Keep `/` | Landing page handles auth redirect; PWA scope remains correct |

### Landing Page Handler (`handleLandingPage`)

```go
func (s *Server) handleLandingPage(w http.ResponseWriter, r *http.Request) {
    // Check for existing session — 302 redirect if authenticated
    cookie, err := r.Cookie("session")
    if err == nil {
        if _, err := s.auth.ValidateToken(cookie.Value); err == nil {
            navigateTo := validateNavigatePath(r.URL.Query().Get("navigate"))
            http.Redirect(w, r, navigateTo, http.StatusFound)
            return
        }
    }
    // Forward ?navigate= to /login for unauthenticated users
    if nav := r.URL.Query().Get("navigate"); nav != "" {
        http.Redirect(w, r, "/login?navigate="+url.QueryEscape(nav), http.StatusFound)
        return
    }
    s.render(w, r, "landing", PageData{Title: "Home", Public: true})
}
```

### Landing Page Content

Minimal hero section:
- App logo + name ("bobot")
- One-liner: "Everybody's AI Assistant" (from manifest.json)
- Feature bullets (4-5 items): Chat, Tasks, Scheduling, Calendar, Smart Home
- Data transparency section: explains Google Calendar access in plain language
- CTA buttons: "Login" → `/login`, "Privacy Policy" → `/privacy`

### Privacy Policy Content

Structured with i18n keys covering:
- What data the app collects (account info, chat messages, calendar data)
- Google Calendar scopes in plain language:
  - "View and manage your calendar events" (`CalendarEventsScope`)
  - "View your list of calendars" (`CalendarCalendarlistReadonlyScope`)
- How data is used (scheduling, reminders, AI-assisted chat)
- Data storage (self-hosted, SQLite, on your server)
- Data sharing (none — self-hosted)
- Contact info

### CSS

Both landing and privacy pages need vertical scrolling (current `body` has `overflow: hidden`). Use page-specific container classes:

- `.landing-container` — centered flex layout with sections, `overflow-y: auto`
- `.privacy-container` — readable prose layout, `overflow-y: auto`, max-width for readability

Reuse existing design tokens (Catppuccin Latte palette, Inter font, 8px spacing scale).

### i18n Keys

Add keys to both `en.json` and `pt-BR.json`:
- `landing.*` — hero title, subtitle, feature descriptions, CTA labels, data usage text
- `privacy.*` — page title, section headers, policy body paragraphs

## Acceptance Criteria

- [ ] Landing page renders at `/` for unauthenticated users
- [ ] Authenticated users visiting `/` get 302 redirected to `/chat`
- [ ] Login page works at `/login` (GET shows form, POST authenticates)
- [ ] Privacy policy page renders at `/privacy` (no login required)
- [ ] All text is translated in EN and PT-BR
- [ ] Logout redirects to `/login`
- [ ] WebSocket auth failure redirects to `/login`
- [ ] Push notification click (no window) opens `/login?navigate=...`
- [ ] Service worker default notification URL is `/chat`
- [ ] Signup success navigates to `/chat` (not back to login)
- [ ] `?navigate=` parameter on `/` forwards to `/login?navigate=` for unauthenticated users
- [ ] Public pages (landing, privacy, login, signup) do not load WebSocket/push scripts
- [ ] All existing tests updated and passing
- [ ] Landing page and privacy page are scrollable

## Technical Considerations

- **PWA flash:** Authenticated PWA users launching from home screen will briefly see the landing page before the 302 redirect. This is acceptable — the redirect is server-side and fast.
- **Session check in landing handler:** Uses the same cookie/JWT validation as the existing login handler, but returns a 302 instead of rendering `authenticated.html`. This is better for SEO (no duplicate content) and faster.
- **`validateNavigatePath` reuse:** The landing page handler reuses the existing whitelist function for the `navigate` parameter.
- **Body overflow:** The global `body { overflow: hidden }` is needed for the chat SPA layout. Public pages override this at the container level with `overflow-y: auto` and `height: 100dvh`.

## Files to Change

### Existing files
1. `server/server.go` — Route registration (move login, add landing + privacy routes)
2. `server/pages.go` — Add `Public` field to `PageData`, add landing/privacy handlers, register templates, update logout redirect
3. `server/signup.go` — Change post-signup flow to render `authenticated` template
4. `web/templates/layout.html` — Conditional script loading based on `Public` flag
5. `web/templates/login.html` — Update `hx-post` target to `/login`
6. `web/static/ws-manager.js` — Auth failure redirect to `/login`
7. `web/static/sw.js` — Update `openWindow` URL and default notification URL
8. `web/static/style.css` — Add landing and privacy page styles
9. `i18n/locales/en.json` — Add landing and privacy keys
10. `i18n/locales/pt-BR.json` — Add landing and privacy keys
11. `server/server_test.go` — Update all login/redirect assertions

### New files
12. `web/templates/landing.html` — Landing page template
13. `web/templates/privacy.html` — Privacy policy template

## References

- Brainstorm: `docs/brainstorms/2026-02-26-landing-page-brainstorm.md`
- Issue: [#41](https://github.com/esnunes/bobot/issues/41)
- Google OAuth consent screen requirements: [Google Identity docs](https://support.google.com/cloud/answer/10311615)
- Calendar scopes: `tools/calendar/oauth.go:34-36`
