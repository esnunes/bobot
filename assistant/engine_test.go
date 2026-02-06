// assistant/engine_test.go
package assistant

import (
	"context"
	"testing"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type mockLLM struct {
	responses []*llm.ChatResponse
	callCount int
}

func (m *mockLLM) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.callCount >= len(m.responses) {
		return &llm.ChatResponse{Content: "default response"}, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return resp, nil
}

type mockTool struct {
	result string
}

func (m *mockTool) Name() string        { return "task" }
func (m *mockTool) Description() string { return "Manage tasks" }
func (m *mockTool) Schema() interface{} { return map[string]interface{}{"type": "object"} }
func (m *mockTool) ParseArgs(raw string) (map[string]any, error) {
	return map[string]any{"command": raw}, nil
}
func (m *mockTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	return m.result, nil
}
func (m *mockTool) AdminOnly() bool { return false }

// Verify mockTool satisfies the Tool interface at compile time
var _ tools.Tool = (*mockTool)(nil)

func TestEngine_Chat_SimpleResponse(t *testing.T) {
	mockProvider := &mockLLM{
		responses: []*llm.ChatResponse{
			{Content: "Hello!", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProvider, registry, nil, mockCtxProvider)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("expected 'Hello!', got '%s'", result)
	}
}

func TestEngine_Chat_WithToolUse(t *testing.T) {
	mockProvider := &mockLLM{
		responses: []*llm.ChatResponse{
			{
				StopType: "tool_use",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "task", Input: map[string]interface{}{"command": "list"}},
				},
			},
			{Content: "Here are your tasks: milk, eggs", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk, eggs"})

	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProvider, registry, nil, mockCtxProvider)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, "What's on my list?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are your tasks: milk, eggs" {
		t.Errorf("unexpected result: %s", result)
	}
}

// mockProvider with custom chatFunc for flexible testing
type mockProvider struct {
	chatFunc func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
}

func (m *mockProvider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return m.chatFunc(ctx, req)
}

type mockContextProvider struct {
	messages []ContextMessage
}

func (m *mockContextProvider) GetContextMessages(userID int64) ([]ContextMessage, error) {
	return m.messages, nil
}

func TestEngine_ChatWithContext(t *testing.T) {
	// Create a mock provider that captures the messages sent
	var capturedMessages []llm.Message
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedMessages = req.Messages
			return &llm.ChatResponse{Content: "response"}, nil
		},
	}

	// Create mock context provider with context messages
	mockCtxProvider := &mockContextProvider{
		messages: []ContextMessage{
			{Role: "user", Content: "previous question"},
			{Role: "assistant", Content: "previous answer"},
		},
	}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, "new question")
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	// Should have 3 messages: 2 from context + 1 new
	if len(capturedMessages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(capturedMessages))
	}

	// Last message should be the new question
	if capturedMessages[2].Content != "new question" {
		t.Errorf("expected last message to be 'new question', got %v", capturedMessages[2].Content)
	}
}

func TestEngine_ChatWithConversation(t *testing.T) {
	// Create a mock provider that captures the messages sent
	var capturedMessages []llm.Message
	var capturedSystemPrompt string
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedMessages = req.Messages
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "Test response"}, nil
		},
	}

	mockCtxProvider := &mockContextProvider{messages: nil}
	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider)

	conversation := []string{
		"[Alice]: Hello @bobot",
		"[Bob]: Yes, please help us",
	}

	response, err := engine.ChatWithContext(context.Background(), conversation)
	if err != nil {
		t.Fatalf("ChatWithContext failed: %v", err)
	}
	if response == "" {
		t.Error("expected non-empty response")
	}

	// Check system prompt contains group chat instructions
	if capturedSystemPrompt == "" {
		t.Error("expected non-empty system prompt")
	}

	// Should have 2 messages from conversation
	if len(capturedMessages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(capturedMessages))
	}
}
