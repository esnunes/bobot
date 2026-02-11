# Skills Web UI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add web pages for managing user-defined skills (list, create, edit, delete) using HTMX, replacing the JSON REST API endpoints.

**Design:** See `docs/plans/2026-02-11-skills-web-ui-design.md`

**Tech Stack:** Go templates, HTMX, vanilla CSS (design tokens from tokens.css).

---

### Task 1: Remove JSON API and Add Page Routes

Remove the `/api/skills` REST endpoints and replace with page routes.

**Files:**
- Modify: `server/server.go` (replace API routes with page routes)
- Delete contents of: `server/skills.go` (will be rewritten in Task 3)
- Delete contents of: `server/skills_test.go` (will be rewritten in Task 5)

**Step 1: Update routes in `server/server.go`**

Replace the skill routes block:

```go
	// Skill routes (require auth)
	s.router.HandleFunc("GET /api/skills", s.sessionMiddleware(s.handleListSkills))
	s.router.HandleFunc("POST /api/skills", s.sessionMiddleware(s.handleCreateSkill))
	s.router.HandleFunc("GET /api/skills/{id}", s.sessionMiddleware(s.handleGetSkill))
	s.router.HandleFunc("PUT /api/skills/{id}", s.sessionMiddleware(s.handleUpdateSkill))
	s.router.HandleFunc("DELETE /api/skills/{id}", s.sessionMiddleware(s.handleDeleteSkill))
```

With:

```go
	// Skill routes (require auth)
	s.router.HandleFunc("GET /skills", s.sessionMiddleware(s.handleSkillsPage))
	s.router.HandleFunc("GET /skills/new", s.sessionMiddleware(s.handleSkillFormPage))
	s.router.HandleFunc("GET /skills/{id}/edit", s.sessionMiddleware(s.handleSkillFormPage))
	s.router.HandleFunc("POST /skills", s.sessionMiddleware(s.handleCreateSkillForm))
	s.router.HandleFunc("POST /skills/{id}", s.sessionMiddleware(s.handleUpdateSkillForm))
	s.router.HandleFunc("DELETE /skills/{id}", s.sessionMiddleware(s.handleDeleteSkillForm))
```

**Step 2: Replace `server/skills.go` with a placeholder**

Replace the entire contents of `server/skills.go` with a minimal placeholder so the project compiles (handlers will be implemented in Task 3):

```go
package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleSkillsPage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleSkillFormPage(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleCreateSkillForm(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleUpdateSkillForm(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleDeleteSkillForm(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
```

**Step 3: Delete `server/skills_test.go` contents**

Replace with a placeholder:

```go
package server
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 5: Commit**

```bash
git add server/server.go server/skills.go server/skills_test.go
git commit -m "refactor(server): replace JSON API routes with page routes for skills

Removes /api/skills REST endpoints, adds page routes for skills
list, form, create, update, delete. Handlers are stubs — will be
implemented in subsequent commits."
```

---

### Task 2: Create Templates

Add the skills list and skill form HTML templates.

**Files:**
- Create: `web/templates/skills.html`
- Create: `web/templates/skill_form.html`
- Modify: `web/templates/chat.html` (add Skills menu link)
- Modify: `web/templates/topic_chat.html` (add Skills menu link)
- Modify: `web/static/style.css` (add skill form styles)

**Step 1: Create `web/templates/skills.html`**

Follow the topics.html pattern:

```html
{{define "content"}}
<div class="skills-container" data-page="skills">
    <header>
        {{if .TopicID}}
        <button hx-get="/topics/{{.TopicID}}" hx-target="body" aria-label="Back"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg></button>
        {{else}}
        <button hx-get="/chat" hx-target="body" aria-label="Back"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg></button>
        {{end}}
        <h1>{{if .TopicName}}Skills - {{.TopicName}}{{else}}Skills{{end}}</h1>
        <div></div>
    </header>

    <main class="skills-list">
        {{if .Skills}}
            {{range .Skills}}
            <button hx-get="/skills/{{.ID}}/edit" hx-target="body" class="skill-item">
                <span class="skill-name">{{.Name}}</span>
                {{if .Description}}<span class="skill-description">{{.Description}}</span>{{end}}
            </button>
            {{end}}
        {{else}}
            <div class="empty">No skills yet.</div>
        {{end}}
        {{if .TopicID}}
        <button hx-get="/skills/new?topic_id={{.TopicID}}" hx-target="body" class="card-action">+ Create new skill</button>
        {{else}}
        <button hx-get="/skills/new" hx-target="body" class="card-action">+ Create new skill</button>
        {{end}}
    </main>
</div>
{{end}}
```

**Step 2: Create `web/templates/skill_form.html`**

```html
{{define "content"}}
<div class="skill-form-container" data-page="skill-form">
    <header>
        {{if .TopicID}}
        <button hx-get="/skills?topic_id={{.TopicID}}" hx-target="body" aria-label="Back"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg></button>
        {{else}}
        <button hx-get="/skills" hx-target="body" aria-label="Back"><svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 18l-6-6 6-6"/></svg></button>
        {{end}}
        <h1>{{if .Skill}}Edit Skill{{else}}New Skill{{end}}</h1>
        <div></div>
    </header>

    <main class="skill-form-body">
        {{if .Skill}}
        <form hx-post="/skills/{{.Skill.ID}}" hx-target="body" class="skill-form">
        {{else}}
        <form hx-post="/skills" hx-target="body" class="skill-form">
        {{end}}
            {{if .TopicID}}<input type="hidden" name="topic_id" value="{{.TopicID}}">{{end}}

            <label for="skill-name">Name</label>
            <input type="text" id="skill-name" name="name" placeholder="Skill name" required maxlength="100" {{if .Skill}}value="{{.Skill.Name}}" disabled{{else}}autofocus{{end}}>

            <label for="skill-description">Description</label>
            <input type="text" id="skill-description" name="description" placeholder="Short description (optional)" maxlength="200" {{if .Skill}}value="{{.Skill.Description}}"{{end}}>

            <label for="skill-content">Content</label>
            <textarea id="skill-content" name="content" placeholder="Markdown instructions for the skill..." rows="20">{{if .Skill}}{{.Skill.Content}}{{end}}</textarea>

            <div class="skill-form-actions">
                {{if .Skill}}
                <button type="button" class="danger-btn" hx-delete="/skills/{{.Skill.ID}}" hx-confirm="Delete skill '{{.Skill.Name}}'?" hx-target="body">Delete</button>
                <div class="skill-form-actions-right">
                    <button type="button" hx-get="/skills{{if .TopicID}}?topic_id={{.TopicID}}{{end}}" hx-target="body">Cancel</button>
                    <button type="submit" class="primary-btn">Save</button>
                </div>
                {{else}}
                <div></div>
                <div class="skill-form-actions-right">
                    <button type="button" hx-get="/skills{{if .TopicID}}?topic_id={{.TopicID}}{{end}}" hx-target="body">Cancel</button>
                    <button type="submit" class="primary-btn">Create</button>
                </div>
                {{end}}
            </div>
        </form>
    </main>
</div>
{{end}}
```

**Step 3: Add Skills link to `web/templates/chat.html` menu overlay**

In the menu overlay div, add a Skills button before the Logout button:

```html
<div id="menu-overlay" class="menu-overlay hidden">
    <div class="menu">
        <button class="menu-item" hx-get="/skills" hx-target="body">Skills</button>
        <button id="logout-btn" hx-post="/logout" hx-on::before-request="htmx.trigger(document.body, 'bobot:logout')">Logout</button>
    </div>
</div>
```

**Step 4: Add Skills link to `web/templates/topic_chat.html` menu overlay**

In the menu overlay div, add a Skills button after the members list and before the delete/leave button:

```html
        <hr>
        <button class="menu-item" hx-get="/skills?topic_id={{.TopicID}}" hx-target="body">Skills</button>
        <hr>
```

(Insert between the members list `</div>` and the existing `<hr>`)

**Step 5: Add styles to `web/static/style.css`**

Add at the end of the file, before the BOBOT ACTION BUTTONS section:

```css
/* ============================================
   SKILLS PAGE
   ============================================ */
.skills-container {
  display: flex;
  flex-direction: column;
}

.skills-list {
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  padding: var(--space-3) calc(var(--space-3) + env(safe-area-inset-right)) var(--space-3) calc(var(--space-3) + env(safe-area-inset-left));
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.skill-item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--space-0);
  padding: var(--space-2) var(--space-3);
  background: var(--colors-surface);
  border-radius: var(--radii-lg);
  box-shadow: var(--shadows-low);
  text-decoration: none;
  color: inherit;
  transition: box-shadow var(--transitions-fast);
  border: 0;
  text-align: left;
}

.skill-item:hover {
  box-shadow: var(--shadows-medium);
}

.skill-name {
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text);
}

.skill-description {
  font-size: var(--font-sizes-1);
  color: var(--colors-text-secondary);
}

/* ============================================
   SKILL FORM
   ============================================ */
.skill-form-container {
  display: flex;
  flex-direction: column;
  height: 100dvh;
}

.skill-form-body {
  flex: 1;
  overflow-y: auto;
  padding: var(--space-3) calc(var(--space-3) + env(safe-area-inset-right)) var(--space-3) calc(var(--space-3) + env(safe-area-inset-left));
}

.skill-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  height: 100%;
}

.skill-form label {
  font-size: var(--font-sizes-1);
  font-weight: var(--font-weights-medium);
  color: var(--colors-text-secondary);
  text-transform: uppercase;
  letter-spacing: var(--letter-spacings-wide);
}

.skill-form input[type="text"] {
  padding: var(--space-1) var(--space-2);
  height: var(--sizes-input-height);
  border: var(--borders-thin);
  border-radius: var(--radii-md);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  background: var(--colors-surface);
  color: var(--colors-text);
  outline: none;
  transition: border-color var(--transitions-fast);
}

.skill-form input[type="text"]:focus {
  border-color: var(--colors-accent);
}

.skill-form input[type="text"]:disabled {
  opacity: var(--opacities-disabled);
  cursor: not-allowed;
}

.skill-form input[type="text"]::placeholder {
  color: var(--colors-text-secondary);
}

.skill-form textarea {
  flex: 1;
  min-height: 200px;
  padding: var(--space-2);
  border: var(--borders-thin);
  border-radius: var(--radii-md);
  font-family: var(--fonts-mono);
  font-size: var(--font-sizes-1);
  line-height: var(--line-heights-relaxed);
  background: var(--colors-surface);
  color: var(--colors-text);
  outline: none;
  resize: vertical;
  transition: border-color var(--transitions-fast);
}

.skill-form textarea:focus {
  border-color: var(--colors-accent);
}

.skill-form textarea::placeholder {
  color: var(--colors-text-secondary);
  font-family: var(--fonts-body);
}

.skill-form-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--space-2) 0 calc(var(--space-2) + env(safe-area-inset-bottom));
}

.skill-form-actions-right {
  display: flex;
  gap: var(--space-2);
}

.primary-btn {
  padding: var(--space-1) var(--space-3);
  background: var(--colors-accent);
  color: var(--colors-background);
  border: none;
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  transition: background-color var(--transitions-fast);
}

.primary-btn:hover {
  background-color: var(--colors-accent-hover);
}

.skill-form-actions button[type="button"]:not(.danger-btn):not(.primary-btn) {
  padding: var(--space-1) var(--space-3);
  background: transparent;
  color: var(--colors-accent);
  border: none;
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
}

.skill-form-actions .danger-btn {
  padding: var(--space-1) var(--space-3);
  border-radius: var(--radii-pill);
  font-family: var(--fonts-body);
  font-size: var(--font-sizes-2);
  font-weight: var(--font-weights-medium);
  border: none;
}
```

**Step 6: Verify build**

Run: `go build ./...`
Expected: BUILD SUCCESS (templates are embedded, handlers exist as stubs)

**Step 7: Commit**

```bash
git add web/templates/skills.html web/templates/skill_form.html web/templates/chat.html web/templates/topic_chat.html web/static/style.css
git commit -m "feat(web): add skills templates and styles

Adds skills list and skill form templates following existing patterns.
Adds Skills link to menu overlay on chat and topic chat pages.
Adds CSS for skills list items and skill form (textarea, actions)."
```

---

### Task 3: Implement Page Handlers

Implement the skill page handlers and form processing.

**Files:**
- Modify: `server/pages.go` (add SkillView to PageData, load templates)
- Rewrite: `server/skills.go` (page handlers + form handlers)

**Step 1: Extend PageData and load templates in `server/pages.go`**

Add `SkillView` struct after `MemberView`:

```go
type SkillView struct {
	ID          int64
	Name        string
	Description string
	Content     string
}
```

Add fields to `PageData`:

```go
	Skills    []SkillView
	Skill     *SkillView
```

Add template loading in `loadTemplates()` (after the `authenticated` template):

```go
	skillsTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/skills.html")
	if err != nil {
		return err
	}
	s.templates["skills"] = skillsTmpl

	skillFormTmpl, err := template.ParseFS(web.FS, "templates/layout.html", "templates/skill_form.html")
	if err != nil {
		return err
	}
	s.templates["skill_form"] = skillFormTmpl
```

**Step 2: Rewrite `server/skills.go`**

Replace the entire file with the full page and form handlers:

```go
package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

func (s *Server) handleSkillsPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")

	var skills []db.SkillRow
	var err error
	var topicID int64
	var topicName string

	if topicIDStr != "" {
		topicID, err = strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		isMember, memberErr := s.db.IsTopicMember(topicID, userData.UserID)
		if memberErr != nil || !isMember {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		topic, topicErr := s.db.GetTopicByID(topicID)
		if topicErr != nil {
			http.Error(w, "topic not found", http.StatusNotFound)
			return
		}
		topicName = topic.Name
		skills, err = s.db.GetTopicSkills(topicID)
	} else {
		skills, err = s.db.GetPrivateChatSkills(userData.UserID)
	}

	if err != nil {
		http.Error(w, "failed to load skills", http.StatusInternalServerError)
		return
	}

	skillViews := make([]SkillView, 0, len(skills))
	for _, sk := range skills {
		skillViews = append(skillViews, SkillView{
			ID:          sk.ID,
			Name:        sk.Name,
			Description: sk.Description,
		})
	}

	s.templates["skills"].Execute(w, PageData{
		Title:     "Skills",
		TopicID:   topicID,
		TopicName: topicName,
		Skills:    skillViews,
	})
}

func (s *Server) handleSkillFormPage(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	topicIDStr := r.URL.Query().Get("topic_id")
	var topicID int64
	if topicIDStr != "" {
		var err error
		topicID, err = strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
	}

	// Check if editing an existing skill
	idStr := r.PathValue("id")
	if idStr != "" {
		skillID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid skill id", http.StatusBadRequest)
			return
		}

		skill, err := s.db.GetSkillByID(skillID)
		if err == db.ErrNotFound {
			http.Error(w, "skill not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "failed to load skill", http.StatusInternalServerError)
			return
		}

		// Verify ownership
		if err := s.canViewSkill(userData, skill); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if skill.TopicID != nil {
			topicID = *skill.TopicID
		}

		s.templates["skill_form"].Execute(w, PageData{
			Title:   "Edit Skill",
			TopicID: topicID,
			Skill: &SkillView{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Content:     skill.Content,
			},
		})
		return
	}

	// New skill form
	s.templates["skill_form"].Execute(w, PageData{
		Title:   "New Skill",
		TopicID: topicID,
	})
}

func (s *Server) handleCreateSkillForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))
	content := r.FormValue("content")
	topicIDStr := r.FormValue("topic_id")

	var topicID *int64
	redirectPath := "/skills"

	if topicIDStr != "" {
		tid, err := strconv.ParseInt(topicIDStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid topic_id", http.StatusBadRequest)
			return
		}
		if err := s.canManageTopicSkills(userData, tid); err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		topicID = &tid
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", tid)
	}

	_, err := s.db.CreateSkill(userData.UserID, topicID, name, description, content)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "a skill with that name already exists", http.StatusConflict)
			return
		}
		http.Error(w, "failed to create skill", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleUpdateSkillForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	description := strings.TrimSpace(r.FormValue("description"))
	content := r.FormValue("content")

	if err := s.db.UpdateSkill(skillID, description, content); err != nil {
		http.Error(w, "failed to update skill", http.StatusInternalServerError)
		return
	}

	redirectPath := "/skills"
	if skill.TopicID != nil {
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", *skill.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteSkillForm(w http.ResponseWriter, r *http.Request) {
	userData := auth.UserDataFromContext(r.Context())

	skillID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skill id", http.StatusBadRequest)
		return
	}

	skill, err := s.db.GetSkillByID(skillID)
	if err == db.ErrNotFound {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load skill", http.StatusInternalServerError)
		return
	}

	if err := s.canManageSkill(userData, skill); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if err := s.db.DeleteSkill(skillID); err != nil {
		http.Error(w, "failed to delete skill", http.StatusInternalServerError)
		return
	}

	redirectPath := "/skills"
	if skill.TopicID != nil {
		redirectPath = fmt.Sprintf("/skills?topic_id=%d", *skill.TopicID)
	}

	w.Header().Set("HX-Trigger", `{"bobot:redirect": {"path": "`+redirectPath+`"}}`)
	w.WriteHeader(http.StatusNoContent)
}

// canManageTopicSkills checks if a user can create/modify topic skills.
func (s *Server) canManageTopicSkills(userData auth.UserData, topicID int64) error {
	if userData.Role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userData.UserID {
		return fmt.Errorf("only the topic owner or admins can manage topic skills")
	}
	return nil
}

// canManageSkill checks if a user can update/delete a specific skill.
func (s *Server) canManageSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		return s.canManageTopicSkills(userData, *skill.TopicID)
	}
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}

// canViewSkill checks if a user can view a specific skill.
func (s *Server) canViewSkill(userData auth.UserData, skill *db.SkillRow) error {
	if skill.TopicID != nil {
		isMember, err := s.db.IsTopicMember(*skill.TopicID, userData.UserID)
		if err != nil || !isMember {
			return fmt.Errorf("forbidden")
		}
		return nil
	}
	if skill.UserID != userData.UserID {
		return fmt.Errorf("forbidden")
	}
	return nil
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```bash
git add server/pages.go server/skills.go
git commit -m "feat(server): implement skill page and form handlers

Adds handleSkillsPage (list), handleSkillFormPage (create/edit form),
and form submission handlers for create, update, delete. Uses HTMX
redirect pattern (bobot:redirect trigger) for navigation after mutations."
```

---

### Task 4: Write Tests

Write tests for the skill page handlers.

**Files:**
- Rewrite: `server/skills_test.go`

**Step 1: Write tests**

```go
package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
)

func TestSkillsPagePrivate(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	coreDB.CreateSkill(user.ID, nil, "groceries", "Manage groceries", "content")

	req := httptest.NewRequest("GET", "/skills", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleSkillsPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "groceries") {
		t.Error("expected page to contain skill name")
	}
	if !strings.Contains(body, "Manage groceries") {
		t.Error("expected page to contain skill description")
	}
}

func TestSkillsPageTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)
	coreDB.CreateSkill(user.ID, &topic.ID, "notes", "Meeting notes", "content")

	req := httptest.NewRequest("GET", "/skills?topic_id="+strconv.FormatInt(topic.ID, 10), nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleSkillsPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "notes") {
		t.Error("expected page to contain skill name")
	}
	if !strings.Contains(body, "General") {
		t.Error("expected page to contain topic name")
	}
}

func TestSkillsPageEmpty(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")

	req := httptest.NewRequest("GET", "/skills", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleSkillsPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No skills yet") {
		t.Error("expected empty state message")
	}
}

func TestSkillFormPageNew(t *testing.T) {
	s, _, cleanup := setupTopicTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/skills/new", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: 1}))
	w := httptest.NewRecorder()

	s.handleSkillFormPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "New Skill") {
		t.Error("expected 'New Skill' heading")
	}
}

func TestSkillFormPageEdit(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "my content")

	req := httptest.NewRequest("GET", "/skills/"+strconv.FormatInt(skill.ID, 10)+"/edit", nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleSkillFormPage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Edit Skill") {
		t.Error("expected 'Edit Skill' heading")
	}
	if !strings.Contains(body, "my content") {
		t.Error("expected skill content in form")
	}
}

func TestCreateSkillForm(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")

	form := url.Values{}
	form.Set("name", "groceries")
	form.Set("description", "Manage grocery lists")
	form.Set("content", "Use task tool")

	req := httptest.NewRequest("POST", "/skills", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkillForm(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	trigger := w.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, "/skills") {
		t.Errorf("expected HX-Trigger with redirect, got %q", trigger)
	}

	skills, _ := coreDB.GetPrivateChatSkills(user.ID)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill in DB, got %d", len(skills))
	}
}

func TestCreateSkillFormTopicForbidden(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	topic, _ := coreDB.CreateTopic("General", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	form := url.Values{}
	form.Set("name", "notes")
	form.Set("content", "content")
	form.Set("topic_id", strconv.FormatInt(topic.ID, 10))

	req := httptest.NewRequest("POST", "/skills", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkillForm(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestUpdateSkillForm(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "old", "old content")

	form := url.Values{}
	form.Set("description", "new desc")
	form.Set("content", "new content")

	req := httptest.NewRequest("POST", "/skills/"+strconv.FormatInt(skill.ID, 10), strings.NewReader(form.Encode()))
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleUpdateSkillForm(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := coreDB.GetSkillByID(skill.ID)
	if updated.Content != "new content" {
		t.Errorf("expected updated content, got %q", updated.Content)
	}
}

func TestDeleteSkillForm(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	req := httptest.NewRequest("DELETE", "/skills/"+strconv.FormatInt(skill.ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteSkillForm(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	_, err := coreDB.GetSkillByID(skill.ID)
	if err == nil {
		t.Error("expected skill to be deleted")
	}
}
```

**Step 2: Run tests**

Run: `go test ./server/ -run "Test.*Skill" -v`
Expected: ALL PASS

**Step 3: Run all tests**

Run: `go test ./...`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add server/skills_test.go
git commit -m "test(server): add tests for skill page and form handlers

Tests cover skills list (private, topic, empty), form pages (new, edit),
create, update, delete, and permission checks."
```

---

### Task 5: Final Verification

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: ALL PASS

**Step 2: Verify build**

Run: `go build ./...`
Expected: BUILD SUCCESS
