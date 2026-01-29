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

func (m *mockTool) Name() string                { return "task" }
func (m *mockTool) Description() string         { return "Manage tasks" }
func (m *mockTool) Schema() interface{}         { return map[string]interface{}{"type": "object"} }
func (m *mockTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	return m.result, nil
}

func TestEngine_Chat_SimpleResponse(t *testing.T) {
	mockProvider := &mockLLM{
		responses: []*llm.ChatResponse{
			{Content: "Hello!", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProvider, registry, nil)

	ctx := auth.ContextWithUserID(context.Background(), 1)
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

	engine := NewEngine(mockProvider, registry, nil)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	result, err := engine.Chat(ctx, "What's on my list?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are your tasks: milk, eggs" {
		t.Errorf("unexpected result: %s", result)
	}
}
