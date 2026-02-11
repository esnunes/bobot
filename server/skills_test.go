package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/esnunes/bobot/auth"
)

func TestListSkillsPrivate(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	coreDB.CreateSkill(user.ID, nil, "groceries", "Manage groceries", "content")
	coreDB.CreateSkill(user.ID, nil, "recipes", "Manage recipes", "content")

	req := httptest.NewRequest("GET", "/api/skills", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListSkills(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []map[string]any
	json.Unmarshal(w.Body.Bytes(), &skills)
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
}

func TestListSkillsTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)
	coreDB.CreateSkill(user.ID, &topic.ID, "notes", "Meeting notes", "content")

	req := httptest.NewRequest("GET", "/api/skills?topic_id="+strconv.FormatInt(topic.ID, 10), nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListSkills(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var skills []map[string]any
	json.Unmarshal(w.Body.Bytes(), &skills)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
}

func TestCreateSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")

	body, _ := json.Marshal(map[string]string{
		"name":        "groceries",
		"description": "Manage grocery lists",
		"content":     "Use task tool",
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify skill exists
	skills, _ := coreDB.GetPrivateChatSkills(user.ID)
	if len(skills) != 1 {
		t.Errorf("expected 1 skill in DB, got %d", len(skills))
	}
}

func TestCreateSkillTopicAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	topic, _ := coreDB.CreateTopic("General", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)

	body, _ := json.Marshal(map[string]any{
		"name":     "notes",
		"content":  "content",
		"topic_id": topic.ID,
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateSkillTopicForbidden(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")
	topic, _ := coreDB.CreateTopic("General", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	body, _ := json.Marshal(map[string]any{
		"name":     "notes",
		"content":  "content",
		"topic_id": topic.ID,
	})
	req := httptest.NewRequest("POST", "/api/skills", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleCreateSkill(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "old", "old content")

	body, _ := json.Marshal(map[string]string{
		"description": "new desc",
		"content":     "new content",
	})
	req := httptest.NewRequest("PUT", "/api/skills/"+strconv.FormatInt(skill.ID, 10), bytes.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleUpdateSkill(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	updated, _ := coreDB.GetSkillByID(skill.ID)
	if updated.Content != "new content" {
		t.Errorf("expected updated content, got %q", updated.Content)
	}
}

func TestDeleteSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	req := httptest.NewRequest("DELETE", "/api/skills/"+strconv.FormatInt(skill.ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteSkill(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetSkillAPI(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("alice", "hash")
	skill, _ := coreDB.CreateSkill(user.ID, nil, "groceries", "desc", "content")

	req := httptest.NewRequest("GET", "/api/skills/"+strconv.FormatInt(skill.ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(skill.ID, 10))
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleGetSkill(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["name"] != "groceries" {
		t.Errorf("expected name 'groceries', got %v", result["name"])
	}
}
