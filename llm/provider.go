// llm/provider.go
package llm

import "context"

type Provider interface {
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

type ChatRequest struct {
	SystemPrompt string
	Messages     []Message
	Tools        []Tool
}

type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"`
	ToolUseID  string      `json:"tool_use_id,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

type ChatResponse struct {
	Content   string
	ToolCalls []ToolCall
	StopType  string
}

type ToolCall struct {
	ID    string
	Name  string
	Input map[string]interface{}
}
