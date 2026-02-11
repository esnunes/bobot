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
