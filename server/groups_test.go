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
