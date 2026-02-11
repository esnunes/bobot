# Skills Web UI Design

## Overview

Add web pages for managing user-defined skills. Skills are managed through dedicated pages (list, create, edit) using HTMX for navigation and form submission. No JSON REST API — all CRUD happens via page routes with form data.

## Pages

### Skills List (`/skills`, `/skills?topic_id=X`)

- **Header**: back button (to `/chat` or `/topics/{id}`) | "Skills" or "Skills - {TopicName}" | menu button
- **Body**: list of skill cards (same pattern as topic items), each showing name + description, clickable to edit via `hx-get="/skills/{id}/edit"`
- **Footer**: "+ Create new skill" button linking to `/skills/new` (or `/skills/new?topic_id=X`)
- **Empty state**: "No skills yet." message

Back button navigates to `/chat` for private skills, or `/topics/{id}` for topic skills.

### Skill Form (`/skills/new`, `/skills/new?topic_id=X`, `/skills/{id}/edit`)

- **Header**: back button (to `/skills` or `/skills?topic_id=X`) | "New Skill" or "Edit Skill" | (no menu button)
- **Body**: form with:
  - Name input (text, required, maxlength 100) — disabled in edit mode
  - Description input (text, optional)
  - Content textarea (tall, ~60vh, monospace font for prompt writing)
  - Hidden `topic_id` input when scoped to a topic
- **Footer**:
  - Create mode: Cancel + Create buttons
  - Edit mode: Delete button (danger, left-aligned) + Cancel + Save buttons

### Navigation Entry

Add "Skills" link to the menu overlay in:
- `chat.html` — links to `/skills`
- `topic_chat.html` — links to `/skills?topic_id={{.TopicID}}`

Use `hx-get` with `hx-target="body"` for HTMX navigation.

## Routes

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | /skills | handleSkillsPage | List page |
| GET | /skills/new | handleSkillFormPage | Create form |
| GET | /skills/{id}/edit | handleSkillFormPage | Edit form |
| POST | /skills | handleCreateSkillForm | Process create |
| POST | /skills/{id} | handleUpdateSkillForm | Process update |
| DELETE | /skills/{id} | handleDeleteSkillForm | Process delete |

All routes use `sessionMiddleware`.

### Form Submission

**Create/Update**: `hx-post="/skills"` or `hx-post="/skills/{id}"`. Handler parses form data via `r.ParseForm()`, calls DB, responds with:
```
HX-Trigger: {"bobot:redirect": {"path": "/skills"}}
```
(or `/skills?topic_id=X` for topic skills)

**Delete**: `hx-delete="/skills/{id}" hx-confirm="Delete this skill?"`. Handler deletes, responds with `HX-Trigger` redirect.

This follows the existing pattern used by `handleCreateTopic`.

## Permission Checks

- **Private skills**: user can only manage their own
- **Topic skills (create/update/delete)**: topic owner or admin only
- **Topic skills (list)**: any topic member can view

## Files Changed

**Remove:**
- API routes for `/api/skills` in `server/server.go`
- `server/skills.go` (JSON API handlers)
- `server/skills_test.go` (JSON API tests)

**Create:**
- `web/templates/skills.html` (list template)
- `web/templates/skill_form.html` (create/edit form template)

**Modify:**
- `server/server.go` — replace API routes with page routes
- `server/pages.go` — add page handlers, extend PageData, load templates
- `web/templates/chat.html` — add Skills link in menu overlay
- `web/templates/topic_chat.html` — add Skills link in menu overlay
- `web/static/style.css` — add styles for skill form (textarea, form layout)
