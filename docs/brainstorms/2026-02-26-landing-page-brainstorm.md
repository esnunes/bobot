# Landing Page Brainstorm

**Date:** 2026-02-26
**Issue:** [#41 — Add a landing page](https://github.com/esnunes/bobot/issues/41)

## What We're Building

A minimal, clean public landing page for bobot that satisfies Google OAuth consent screen requirements. The page will:

- Accurately represent bobot as a self-hosted personal AI assistant
- Describe the app's functionality to users
- Explain why the app requests Google Calendar data (events read/write, calendar list read-only)
- Link to a dedicated privacy policy page
- Be accessible without login

## Why This Approach

### URL Strategy: Landing page at `/`, login moves to `/login`

The root URL is the most natural location for a public-facing page and matches what Google expects for a homepage link. The existing login page moves to `/login`, which is a conventional path. Authenticated users visiting `/` will be redirected to `/chat`, preserving current PWA behavior.

### Separate privacy policy page at `/privacy`

A dedicated `/privacy` route keeps concerns cleanly separated. Google OAuth consent screen configuration requires a direct link to the privacy policy — a standalone page is cleaner than an anchor on the landing page and easier to maintain independently.

### Minimal & clean design

Matches the existing Catppuccin Latte aesthetic. Brief hero section with app name, one-liner description, key features as icons/bullets, and links to login and privacy policy. No heavy feature showcase — just enough to satisfy Google's requirements and give visitors a clear picture of what bobot is.

## Key Decisions

1. **Landing page at `/`** — Login moves to `/login` and signup stays at `/signup`
2. **Privacy policy at `/privacy`** — Separate public page, no login required
3. **Authenticated redirect preserved** — Users with valid sessions visiting `/` go to `/chat`
4. **Minimal design** — Hero + feature bullets + data usage explanation + privacy/login links
5. **i18n required** — All text in both `en.json` and `pt-BR.json`
6. **Google Calendar scopes to document** — `CalendarEventsScope` (read/write events) and `CalendarCalendarlistReadonlyScope` (read-only calendar list)

## Scope

### In scope
- New landing page template (`landing.html`) at `/`
- New privacy policy page template (`privacy.html`) at `/privacy`
- Move login from `/` to `/login` (update routes, handler, redirects)
- Update all internal references to the login path (logout redirect, signup link, `validateNavigatePath`, etc.)
- Add i18n keys for both pages in EN and PT-BR
- Add CSS styles using existing design tokens
- Landing page content: app name, description, feature highlights, data usage transparency, login button, privacy link

### Out of scope
- Terms of service page (not required by Google OAuth)
- Signup flow changes (stays at `/signup`)
- Any changes to the Google Calendar integration itself

## Open Questions

_None — all key decisions have been resolved._
