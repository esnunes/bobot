// server/chat_test.go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/esnunes/bobot/assistant"
	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/config"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
	"github.com/esnunes/bobot/tools/user"
)

type mockLLMProvider struct{}

func (m *mockLLMProvider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{Content: "Hello from assistant!", StopType: "end_turn"}, nil
}

type mockContextProvider struct{}

func (m *mockContextProvider) GetContextMessages(userID int64) ([]assistant.ContextMessage, error) {
	return nil, nil
}

func setupChatTestServer(t *testing.T) (*Server, string) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
		JWT:    config.JWTConfig{Secret: "test-secret-32-chars-minimum!!"},
		Session: config.SessionConfig{
			Duration:         30 * time.Minute,
			MaxAge:           7 * 24 * time.Hour,
			RefreshThreshold: 5 * time.Minute,
		},
	}

	registry := tools.NewRegistry()
	engine := assistant.NewEngine(&mockLLMProvider{}, registry, nil, &mockContextProvider{})

	srv := NewWithAssistant(cfg, coreDB, engine, registry)

	// Create test user and get session token
	hash, _ := auth.HashPassword("testpass")
	user, _ := coreDB.CreateUser("testuser", hash)
	token, _ := srv.session.CreateToken(user.ID, "user")

	return srv, token
}

func TestChatWebSocket_Connect(t *testing.T) {
	srv, token := setupChatTestServer(t)

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat"
	header := http.Header{}
	header.Add("Cookie", "session="+token)

	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected 101, got %d", resp.StatusCode)
	}
}

func TestChatWebSocket_SendMessage(t *testing.T) {
	srv, token := setupChatTestServer(t)

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat"
	header := http.Header{}
	header.Add("Cookie", "session="+token)

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send message
	err = conn.WriteJSON(map[string]string{"content": "Hello"})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// First broadcast: user message echo
	var userResp map[string]string
	err = conn.ReadJSON(&userResp)
	if err != nil {
		t.Fatalf("failed to read user message echo: %v", err)
	}
	if userResp["role"] != "user" || userResp["content"] != "Hello" {
		t.Errorf("unexpected user echo: %v", userResp)
	}

	// Second broadcast: assistant response
	var assistantResp map[string]string
	err = conn.ReadJSON(&assistantResp)
	if err != nil {
		t.Fatalf("failed to read assistant response: %v", err)
	}
	if assistantResp["role"] != "assistant" || assistantResp["content"] != "Hello from assistant!" {
		t.Errorf("unexpected assistant response: %v", assistantResp)
	}
}

func TestChatWebSocket_SlashCommand(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, _ := db.NewCoreDB(tmpDir + "/core.db")

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
		JWT:    config.JWTConfig{Secret: "test-secret-32-chars-minimum!!"},
		Session: config.SessionConfig{
			Duration:         30 * time.Minute,
			MaxAge:           7 * 24 * time.Hour,
			RefreshThreshold: 5 * time.Minute,
		},
		BaseURL: "http://localhost:8080",
	}

	registry := tools.NewRegistry()
	registry.Register(user.NewUserTool(coreDB, cfg.BaseURL))
	engine := assistant.NewEngine(&mockLLMProvider{}, registry, nil, &mockContextProvider{})

	srv := NewWithAssistant(cfg, coreDB, engine, registry)

	// Create admin user and get token with role
	hash, _ := auth.HashPassword("testpass")
	adminUser, _ := coreDB.CreateUserFull("admin", hash, "Admin", "admin")
	token, _ := srv.session.CreateToken(adminUser.ID, "admin")

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat"
	header := http.Header{}
	header.Add("Cookie", "session="+token)

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Send slash command
	err = conn.WriteJSON(map[string]string{"content": "/user list"})
	if err != nil {
		t.Fatalf("failed to send message: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// First: command message echo
	var userResp map[string]string
	err = conn.ReadJSON(&userResp)
	if err != nil {
		t.Fatalf("failed to read command message echo: %v", err)
	}
	if userResp["role"] != "command" {
		t.Errorf("expected role 'command', got '%s'", userResp["role"])
	}
	if userResp["content"] != "/user list" {
		t.Errorf("expected content '/user list', got '%s'", userResp["content"])
	}

	// Second: system response with user list
	var resp map[string]string
	err = conn.ReadJSON(&resp)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if resp["role"] != "system" {
		t.Errorf("expected role 'system', got '%s'", resp["role"])
	}
	if !strings.Contains(resp["content"], "admin") {
		t.Errorf("expected response to contain 'admin', got: %s", resp["content"])
	}
}

func TestTopicMessage(t *testing.T) {
	// This test verifies the message struct accepts topic_id
	msg := chatMessage{
		Content: "Hello",
		TopicID: ptr(int64(5)),
	}
	if msg.TopicID == nil || *msg.TopicID != 5 {
		t.Error("expected topic_id to be 5")
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestWebSocket_SessionCookieAuth(t *testing.T) {
	srv, token := setupChatTestServer(t)

	// Create test server
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Connect with session cookie
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/chat"
	header := http.Header{}
	header.Add("Cookie", "session="+token)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Dial error: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Status = %d, want 101", resp.StatusCode)
	}
}
