// server/groups_test.go
package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func setupGroupTestServer(t *testing.T) (*Server, *db.CoreDB, func()) {
	tmpDir := t.TempDir()
	coreDB, err := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}

	cfg := &config.Config{
		JWT: config.JWTConfig{Secret: "test-secret-key-32-chars-min!!"},
	}
	jwt := auth.NewJWTService(cfg.JWT.Secret)
	s := New(cfg, coreDB, jwt)

	return s, coreDB, func() { coreDB.Close() }
}

func TestCreateGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	body := `{"name": "My Group"}`
	req := httptest.NewRequest("POST", "/api/groups", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleCreateGroup(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "My Group" {
		t.Errorf("expected name 'My Group', got %v", resp["name"])
	}
}

func TestListGroups(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a group first
	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/groups", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleListGroups(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var groups []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &groups)
	if len(groups) != 1 {
		t.Errorf("expected 1 group, got %d", len(groups))
	}
}

func TestGetGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("GET", "/api/groups/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleGetGroup(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteGroup(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}

	// Verify soft deleted
	_, err := coreDB.GetGroupByID(group.ID)
	if err != db.ErrNotFound {
		t.Error("expected group to be soft deleted")
	}
}

func TestDeleteGroupNotOwner(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleDeleteGroup(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestAddGroupMember(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	newMember, _ := coreDB.CreateUser("newmember", "hash")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)

	body := `{"username": "newmember"}`
	req := httptest.NewRequest("POST", "/api/groups/1/members", bytes.NewBufferString(body))
	req.SetPathValue("id", "1")
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleAddGroupMember(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify member was added
	isMember, _ := coreDB.IsGroupMember(group.ID, newMember.ID)
	if !isMember {
		t.Error("expected new member to be added")
	}
}

func TestRemoveGroupMember(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/1/members/2", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("userId", "2")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveGroupMember(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLeaveGroup(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")
	member, _ := coreDB.CreateUser("member", "hash")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)
	coreDB.AddGroupMember(group.ID, member.ID)

	// Member removes self
	req := httptest.NewRequest("DELETE", "/api/groups/1/members/2", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("userId", "2")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: member.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveGroupMember(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", w.Code)
	}
}

func TestOwnerCannotLeave(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	owner, _ := coreDB.CreateUser("owner", "hash")

	group, _ := coreDB.CreateGroup("Test Group", owner.ID)
	coreDB.AddGroupMember(group.ID, owner.ID)

	req := httptest.NewRequest("DELETE", "/api/groups/1/members/1", nil)
	req.SetPathValue("id", "1")
	req.SetPathValue("userId", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: owner.ID}))
	w := httptest.NewRecorder()

	s.handleRemoveGroupMember(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestGetGroupMessages(t *testing.T) {
	s, coreDB, cleanup := setupGroupTestServer(t)
	defer cleanup()

	user, _ := coreDB.CreateUser("testuser", "hash")

	group, _ := coreDB.CreateGroup("Test Group", user.ID)
	coreDB.AddGroupMember(group.ID, user.ID)
	coreDB.CreateGroupMessage(group.ID, user.ID, "user", "Hello")
	coreDB.CreateGroupMessage(group.ID, user.ID, "assistant", "Hi there")

	req := httptest.NewRequest("GET", "/api/groups/1/messages/recent?limit=50", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	w := httptest.NewRecorder()

	s.handleGroupRecentMessages(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var messages []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &messages)
	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}
}
