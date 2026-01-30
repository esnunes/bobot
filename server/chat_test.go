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
	jwtSvc := auth.NewJWTService("test-secret-32-chars-minimum!!")

	cfg := &config.Config{
		Server: config.ServerConfig{Host: "localhost", Port: 8080},
	}

	registry := tools.NewRegistry()
	engine := assistant.NewEngine(&mockLLMProvider{}, registry, nil, &mockContextProvider{})

	srv := NewWithAssistant(cfg, coreDB, jwtSvc, engine)

	// Create test user and get token
	hash, _ := auth.HashPassword("testpass")
	user, _ := coreDB.CreateUser("testuser", hash)
	token, _ := jwtSvc.GenerateAccessToken(user.ID)

	return srv, token
}

func TestChatWebSocket_Connect(t *testing.T) {
	srv, token := setupChatTestServer(t)

	server := httptest.NewServer(srv)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat?token=" + token

	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, nil)
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

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/chat?token=" + token

	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
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
