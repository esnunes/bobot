// server/messages_test.go
package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
)

func TestHandleMessageHistory(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT:     config.JWTConfig{Secret: "testsecret"},
		Session: config.SessionConfig{},
		History: config.HistoryConfig{
			DefaultLimit: 50,
			MaxLimit:     100,
		},
	}

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)

	// Create 5 messages in the bobot topic
	for i := 0; i < 5; i++ {
		coreDB.CreateTopicMessage(bobotTopic.ID, user.ID, "user", "msg", "msg")
	}

	srv := New(cfg, coreDB)

	req := httptest.NewRequest("GET", "/api/messages/history?before=5&limit=2", nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	rec := httptest.NewRecorder()

	srv.handleMessageHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleMessageSync(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(filepath.Join(tmpDir, "core.db"))
	defer coreDB.Close()

	cfg := &config.Config{
		JWT:     config.JWTConfig{Secret: "testsecret"},
		Session: config.SessionConfig{},
		Sync: config.SyncConfig{
			MaxLookback: 24 * time.Hour,
		},
	}

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)

	// Use a time in the past as the "since" point
	since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)

	// Create a message after the "since" time
	coreDB.CreateTopicMessage(bobotTopic.ID, db.BobotUserID, "assistant", "new message", "new message")

	srv := New(cfg, coreDB)

	req := httptest.NewRequest("GET", "/api/messages/sync?since="+since, nil)
	req = req.WithContext(auth.ContextWithUserData(req.Context(), auth.UserData{UserID: user.ID}))
	rec := httptest.NewRecorder()

	srv.handleMessageSync(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var messages []db.Message
	json.NewDecoder(rec.Body).Decode(&messages)

	if len(messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(messages))
	}
}
