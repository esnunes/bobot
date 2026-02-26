---
title: "feat: Add multi-language support for web UI"
type: feat
date: 2026-02-26
issue: https://github.com/esnunes/bobot/issues/38
brainstorm: docs/brainstorms/2026-02-26-multi-language-support-brainstorm.md
---

# feat: Add multi-language support for web UI

## Overview

Add internationalization (i18n) to the bobot-web UI so all static elements (buttons, labels, navigation, errors, page titles) render in the user's preferred language. Starting with English and Brazilian Portuguese, structured for easy addition of new languages.

## Problem Statement / Motivation

Most current users speak Brazilian Portuguese, but the entire UI is in English. This creates friction for the primary user base. The solution must work without third-party libraries, using Go's standard library and `embed` package.

## Proposed Solution

Create an `i18n` package with embedded JSON translation files and a `T()` function. Register a `t` template function in Go's `template.FuncMap` so templates can render translated strings. Store the user's language preference in the `users` table and propagate it via the session token to avoid per-request DB queries.

## Technical Approach

### Phase 1: i18n Package Foundation

Create the `i18n/` package with embedded locale files.

**New files:**
- `i18n/i18n.go` -- Package with `T(lang, key string, args ...any) string` function, `FuncMap()`, and locale loading
- `i18n/locales/en.json` -- English translations (all current hardcoded strings)
- `i18n/locales/pt-BR.json` -- Brazilian Portuguese translations

**i18n package API:**

```go
package i18n

// T returns the translated string for the given language and key.
// Falls back to English if key is missing in the target language.
// Logs a warning when falling back.
// Supports fmt.Sprintf-style interpolation: T("pt-BR", "chat.members", 3) -> "3 membros"
func T(lang, key string, args ...any) string

// FuncMap returns a template.FuncMap with the "t" function registered.
// Usage in templates: {{t .Lang "nav.topics"}}
func FuncMap() template.FuncMap

// SupportedLanguages returns the list of available language codes.
func SupportedLanguages() []string
```

**JSON format** (flat keys with dot notation):

```json
{
  "nav.topics": "Topics",
  "nav.settings": "Settings",
  "chat.send": "Send",
  "chat.typing": "typing...",
  "chat.placeholder": "Message...",
  "login.title": "Welcome to bobot",
  "login.username": "Username",
  "login.password": "Password",
  "login.submit": "Login",
  "settings.language": "Language",
  "settings.save": "Save",
  "error.invalid_credentials": "Invalid credentials",
  "error.account_blocked": "Account blocked"
}
```

**Interpolation support:** When `args` are provided, the translated string is passed through `fmt.Sprintf`. Example: `T("en", "chat.members_count", 3)` with key value `"%d members"` -> `"3 members"`.

### Phase 2: Database & Auth Integration

**Modify `db/core.go`:**
- Add `language TEXT NOT NULL DEFAULT 'pt-BR'` column to `users` table via `addColumnIfMissing` migration
- Default to `pt-BR` because the majority of current users are Brazilian Portuguese speakers
- Add `Language string` field to `db.User` struct
- Add `UpdateUserLanguage(userID int64, lang string) error` method
- Update `GetUser`/`GetUserByUsername` to include the `language` column

**Modify `auth/` package:**
- Add `Language string` field to `auth.SessionToken` struct
- Add `Language string` field to `auth.UserData` struct
- Session token now carries language -- avoids DB query on every request
- When user changes language, reissue the session token with the new language

**Modify `server/server.go` sessionMiddleware:**
- Extract `Language` from the decoded session token
- Set `Language` on `auth.UserData` injected into context

### Phase 3: Template System Integration

**Modify `server/pages.go`:**
- Add `Lang string` field to `PageData` struct
- Update `loadTemplates()` to register `i18n.FuncMap()` on every template via `template.New("").Funcs(i18n.FuncMap()).ParseFS(...)`
- Update `server.render()` to set `data.Lang` from `auth.UserDataFromContext(ctx)` -- requires passing the request context to render, or extracting it before the call

**Modify `web/templates/layout.html`:**
- Change `<html lang="en">` to `<html lang="{{.Lang}}">`

**Modify all 16 HTML templates:**
- Replace every hardcoded English string with `{{t .Lang "key"}}`
- For strings with dynamic values, use interpolation: `{{t .Lang "admin.user.last_active" .LastMessageAt}}`
- For `hx-confirm` attributes: `hx-confirm="{{t .Lang "skill.delete_confirm" .Skill.Name}}"`

### Phase 4: JavaScript Translation Strategy

Follow the priority: server-render > data attributes > JSON blob.

**Approach per file:**

1. **settings.js** -- `confirm()` and `alert()` strings: Add `data-i18n-*` attributes to the settings container element in `settings.html`. JS reads these at init.
   ```html
   <div data-page="settings"
        data-i18n-confirm-delete="{{t .Lang "settings.confirm_delete_topic"}}"
        data-i18n-confirm-leave="{{t .Lang "settings.confirm_leave_topic"}}"
        data-i18n-error-delete="{{t .Lang "settings.error_delete_topic"}}"
        data-i18n-error-leave="{{t .Lang "settings.error_leave_topic"}}">
   ```

2. **topic_chat.js** -- Quick action empty states: Add data attributes to the quick actions container in `topic_chat.html`.

3. **message-renderer.js** -- "Reminder", "Scheduled", "Confirm?" labels: Pass via `data-i18n` JSON blob in `layout.html` since MessageRenderer is global and used across pages.
   ```html
   <script type="application/json" data-i18n>{"reminder":"{{t .Lang "chat.reminder"}}","scheduled":"{{t .Lang "chat.scheduled"}}","confirm":"{{t .Lang "chat.confirm"}}"}</script>
   ```

4. **push.js** -- Notification blocked alert: Add data attribute to the push notification toggle element in `settings.html`.

5. **signup.html inline script** -- "Passwords do not match": Use data attribute on the form element.

### Phase 5: Settings Page Language Selector

**Modify `web/templates/settings.html`:**
- Add a language selector in the Account section (below display name, above push notifications)
- Use a `<select>` element with supported languages
- Submit via `hx-post="/api/user/language"` with `hx-swap="none"`

**Modify `server/pages.go`:**
- Add `handleUpdateLanguage` handler for `POST /api/user/language`
- Validate language is in `i18n.SupportedLanguages()`
- Call `db.UpdateUserLanguage()`
- Reissue session token with new language (set new cookie)
- Return `HX-Redirect` header to force full page reload in new language

**Modify `server/server.go`:**
- Register route: `POST /api/user/language` -> `handleUpdateLanguage`

### Phase 6: Unauthenticated Pages

For login and signup pages, there is no user session. Use browser's `Accept-Language` header.

**Modify `server/pages.go`:**
- In `handleLoginPage` and `handleSignupPage`, parse `Accept-Language` header
- Extract best matching language from supported list (simple prefix match: `pt` -> `pt-BR`)
- Set `PageData.Lang` accordingly
- If no match, default to `en`

**Add helper in `i18n/i18n.go`:**
- `MatchLanguage(acceptHeader string) string` -- parses Accept-Language and returns best match

### Phase 7: Server-Side Error Messages

**Translate user-facing error messages:**
- Login/signup errors rendered via `PageData.Error` (these appear in templates): translate using `i18n.T(lang, key)`
- Validation errors in `signup.go`: return translated strings based on the request's language

**Keep internal HTTP errors in English:**
- `http.Error()` calls for API endpoints (consumed by HTMX/JS, not displayed directly): keep as-is for now
- WebSocket error messages (`"Sorry, I encountered an error"`, slash command errors): defer translation to a later phase since the chat pipeline doesn't carry language context

### Phase 8: Translation Key Validation Test

**New file: `i18n/i18n_test.go`**
- Test that all keys in `en.json` exist in `pt-BR.json` and vice versa
- Test that `T()` returns English fallback for missing keys
- Test that interpolation works correctly
- Test that `MatchLanguage()` correctly parses Accept-Language headers

## Acceptance Criteria

### Functional Requirements
- [ ] All 16 HTML templates render translated strings based on user's language
- [ ] `<html lang="...">` attribute is dynamic
- [ ] Page titles (`<title>`) are translated
- [ ] User can change language in settings page
- [ ] Language preference persists across sessions (stored in DB, carried in token)
- [ ] Login/signup pages use browser's Accept-Language for language detection
- [ ] JavaScript UI strings (confirms, alerts, empty states) are translated
- [ ] Missing translation keys fall back to English with a logged warning
- [ ] Existing users default to `pt-BR`
- [ ] New users get language auto-detected from Accept-Language on first login

### Non-Functional Requirements
- [ ] No third-party i18n libraries used
- [ ] No per-request DB query for language (carried in session token)
- [ ] Adding a new language requires only a new JSON file (no code changes)
- [ ] Translation key parity enforced by tests (en.json keys == pt-BR.json keys)

## Files to Modify

### New files
- `i18n/i18n.go`
- `i18n/locales/en.json`
- `i18n/locales/pt-BR.json`
- `i18n/i18n_test.go`

### Modified files
- `db/core.go` -- add `language` column migration, `Language` field on `User`, `UpdateUserLanguage()`
- `auth/context.go` -- add `Language` to `UserData`
- `auth/session.go` -- add `Language` to `SessionToken`
- `server/server.go` -- update `sessionMiddleware` to propagate language, add language route
- `server/pages.go` -- add `Lang` to `PageData`, update `render()`, update `loadTemplates()` with FuncMap, add `handleUpdateLanguage`, translate login/signup error strings
- `server/signup.go` -- translate validation error strings
- `web/templates/layout.html` -- dynamic `<html lang>`, add `data-i18n` script for global JS translations
- `web/templates/login.html` -- replace hardcoded strings with `{{t .Lang "key"}}`
- `web/templates/signup.html` -- replace hardcoded strings, add data attribute for inline JS
- `web/templates/chats.html` -- replace hardcoded strings
- `web/templates/topic_chat.html` -- replace hardcoded strings, add data attributes for JS
- `web/templates/settings.html` -- replace hardcoded strings, add language selector, add data attributes for JS
- `web/templates/skills.html` -- replace hardcoded strings
- `web/templates/skill_form.html` -- replace hardcoded strings
- `web/templates/schedules.html` -- replace hardcoded strings
- `web/templates/schedule_form.html` -- replace hardcoded strings
- `web/templates/quick_actions.html` -- replace hardcoded strings
- `web/templates/quick_action_form.html` -- replace hardcoded strings
- `web/templates/admin.html` -- replace hardcoded strings
- `web/templates/admin_user.html` -- replace hardcoded strings
- `web/templates/admin_context.html` -- replace hardcoded strings
- `web/static/settings.js` -- read translations from data attributes
- `web/static/topic_chat.js` -- read translations from data attributes
- `web/static/message-renderer.js` -- read translations from data-i18n JSON blob
- `web/static/push.js` -- read translation from data attribute

## Deferred Items

- Plural handling (add when a concrete need arises)
- WebSocket/chat error message translation (requires pipeline language context)
- Push notification translation
- Welcome message language detection at signup
- HTTP API error response translation (`http.Error()` calls)
- Translation management tooling (key extraction, diff checking beyond tests)

## Dependencies & Risks

- **Risk:** Large number of templates to modify (16 files, ~150+ strings). Each string replacement is mechanical but error-prone. Mitigation: systematic template-by-template approach with the test validating key parity.
- **Risk:** Session token format change. Existing sessions won't have `Language` field. Mitigation: treat missing `Language` in token as `pt-BR` default (matches DB default).
- **Risk:** HTMX full-body swap after language change. Mitigation: use `HX-Redirect` to force full page reload when language changes.

## References & Research

### Internal References
- Brainstorm: `docs/brainstorms/2026-02-26-multi-language-support-brainstorm.md`
- Template loading: `server/pages.go:204-296`
- PageData struct: `server/pages.go:153-184`
- Session middleware: `server/server.go:182-234`
- User struct: `db/core.go:38-46`
- Users table schema: `db/core.go:123-128`
- Template embed: `web/embed.go`
- Welcome message: `db/core.go:21-36`

### Institutional Learnings Applied
- Consistent rendering strategy for cross-cutting concerns (from unread indicator SSR vs JS learning)
- Extract shared logic early to avoid implementation divergence (from auto-read marking pattern)
- Middleware chaining pattern for request enrichment (from admin context inspection pattern)
