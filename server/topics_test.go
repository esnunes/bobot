// server/topics_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupTopicTestServer(t *testing.T) (*Server, *db.CoreDB, func()) {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	cfg := &config.Config{
		JWT:     config.JWTConfig{Secret: "test-secret-key-32-chars-min!!"},
		Session: config.SessionConfig{},
	}
	s := New(cfg, coreDB)

	return s, coreDB, func() { coreDB.Close() }
}

func TestCreateTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	form := "name=My+Topic"
	req := httptest.NewRequest("POST", "/api/topics", bytes.NewBufferString(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateTopic(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}

	// HTMX pattern: redirect to the new topic
	redirect := w.Header().Get("HX-Location")
	if redirect != "/topics/1" {
		t.Errorf("expected HX-Location '/topics/1', got %q", redirect)
	}

	// Verify topic was created in DB
	topic, err := coreDB.GetTopicByID(1)
	if err != nil {
		t.Fatalf("expected topic to be created: %v", err)
	}
	if topic.Name != "My Topic" {
		t.Errorf("expected topic name 'My Topic', got %q", topic.Name)
	}
}

func TestListTopics(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a topic first
	topic, _ := coreDB.CreateTopic("Test Topic", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/topics", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListTopics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var topics []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &topics)
	if len(topics) != 1 {
		t.Errorf("expected 1 topic, got %d", len(topics))
	}
}

func TestGetTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/topics/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleGetTopic(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", user.ID)
	coreDB.AddTopicMember(topic.ID, user.ID)

	req := httptest.NewRequest("DELETE", "/api/topics/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteTopic(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify soft deleted
	_, err := coreDB.GetTopicByID(topic.ID)
	if err != db.ErrNotFound {
		t.Error("expected topic to be soft deleted")
	}
}

func TestDeleteTopicNotOwner(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	req := httptest.NewRequest("DELETE", "/api/topics/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteTopic(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestAddTopicMember(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	newMember, _ := coreDB.CreateUser("newmember", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)

	body := `{"username": "newmember"}`
	req := httptest.NewRequest("POST", "/api/topics/1/members", bytes.NewBufferString(body))
	req.SetPathValue("id", "1")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleAddTopicMember(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify member was added
	isMember, _ := coreDB.IsTopicMember(topic.ID, newMember.ID)
	if !isMember {
		t.Error("expected new member to be added")
	}
}

func TestRemoveTopicMember(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	topicIDStr := strconv.FormatInt(topic.ID, 10)
	memberIDStr := strconv.FormatInt(member.ID, 10)

	req := httptest.NewRequest("DELETE", "/api/topics/"+topicIDStr+"/members/"+memberIDStr, nil)
	req.SetPathValue("id", topicIDStr)
	req.SetPathValue("userId", memberIDStr)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveTopicMember(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLeaveTopic(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)
	coreDB.AddTopicMember(topic.ID, member.ID)

	topicIDStr := strconv.FormatInt(topic.ID, 10)
	memberIDStr := strconv.FormatInt(member.ID, 10)

	// Member removes self
	req := httptest.NewRequest("DELETE", "/api/topics/"+topicIDStr+"/members/"+memberIDStr, nil)
	req.SetPathValue("id", topicIDStr)
	req.SetPathValue("userId", memberIDStr)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveTopicMember(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestOwnerCannotLeave(t *testing.T) {
	s, coreDB, cleanup := setupTopicTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")

	topic, _ := coreDB.CreateTopic("Test Topic", owner.ID)
	coreDB.AddTopicMember(topic.ID, owner.ID)

	topicIDStr := strconv.FormatInt(topic.ID, 10)
	ownerIDStr := strconv.FormatInt(owner.ID, 10)

	req := httptest.NewRequest("DELETE", "/api/topics/"+topicIDStr+"/members/"+ownerIDStr, nil)
	req.SetPathValue("id", topicIDStr)
	req.SetPathValue("userId", ownerIDStr)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveTopicMember(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}
