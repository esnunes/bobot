# Multi-Language Support (i18n) for Web UI

**Date:** 2026-02-26
**Issue:** https://github.com/esnunes/bobot/issues/38
**Status:** Brainstorm

## What We're Building

Internationalization (i18n) for the bobot-web UI, translating all static UI elements (buttons, labels, navigation, system messages, error messages) into the user's preferred language. Starting with English and Brazilian Portuguese, designed for easy addition of new languages.

**Scope:** Web UI only. Bot/assistant responses are NOT in scope -- those are controlled by system prompts and remain as-is.

## Why This Approach

**Approach chosen:** Embedded JSON translation files with a Go template function.

We chose this because:
- **Clean separation**: Translation strings live in JSON files, not mixed with Go code or templates
- **Follows existing patterns**: The project already uses Go's `embed` package for templates and static files
- **Easy to extend**: Adding a new language means adding a new JSON file -- no code changes needed
- **No third-party dependencies**: Aligns with the project's minimal dependency philosophy
- **Idiomatic**: A `t` template function is a well-known pattern in Go web apps

Rejected alternatives:
- **Go maps in code**: Mixes data with code, verbose diffs, harder for non-developers to contribute
- **Per-template translation**: Template duplication, every UI change must be made N times, doesn't scale

## Key Decisions

1. **Scope is Web UI only** -- bot responses, tool outputs, and push notifications are out of scope
2. **Embedded JSON files** (`en.json`, `pt-BR.json`) compiled into the binary via `go:embed`
3. **Language stored in user profile** -- auto-set during profile update when not defined, changeable in settings page
4. **Translation access priority for JS strings:**
   - First: render directly from backend in templates
   - Second: inline via data attributes on elements
   - Last resort: JSON blob in `<script type="application/json" data-i18n>` tag
5. **Start with 2 languages** (en, pt-BR), but structure for easy addition
6. **Template function `t`** registered in Go template FuncMap, usage: `{{t .Lang "key.path"}}`
7. **`i18n` package** with a `T(lang, key string) string` function for server-side translation outside templates

## Design Sketch

### Translation file structure
```
i18n/
  locales/
    en.json
    pt-BR.json
  i18n.go        # Package with T() function, embed, and template FuncMap
```

### JSON format (flat keys with dot notation)
```json
{
  "nav.topics": "Topics",
  "nav.settings": "Settings",
  "chat.send": "Send",
  "chat.typing": "typing...",
  "chat.placeholder": "Type a message...",
  "login.title": "Sign In",
  "login.email": "Email",
  "login.password": "Password",
  "login.submit": "Sign In",
  "settings.language": "Language",
  "settings.save": "Save"
}
```

### Template usage
```html
<button type="submit">{{t .Lang "chat.send"}}</button>
<input placeholder="{{t .Lang "chat.placeholder"}}">
```

### Language resolution flow
1. Middleware reads user's `language` field from session/DB
2. Sets `Lang` field on page data structs
3. Templates use `{{t .Lang "key"}}` throughout
4. Default fallback: English if key missing in target language

## Resolved Questions

1. **Fallback behavior**: Fall back to English AND log a warning. Helps catch missing translations during development while keeping the UI functional.
2. **Key naming convention**: Dot notation (`nav.topics`). Most common in i18n systems, familiar and concise.
3. **Plural handling**: Deferred. Will add when a concrete need arises (YAGNI).
