// tools/registry.go
package tools

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/llm"
)

// ExecuteInput contains the input for tool execution.
type ExecuteInput struct {
	Args       string // raw string after tool name, e.g. "create hello world"
	ReceiverID *int64 // set in private chat, nil in topic chat
	TopicID    *int64 // set in topic chat, nil in private chat
}

type Tool interface {
	Name() string
	Description() string
	Schema() interface{}
	Execute(ctx context.Context, input ExecuteInput) (string, error)
	AdminOnly() bool
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
}

func (r *Registry) Get(name string) (Tool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) List() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	return result
}

func (r *Registry) Execute(ctx context.Context, name string, input ExecuteInput) (string, error) {
	tool, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return tool.Execute(ctx, input)
}

func (r *Registry) ToLLMTools() []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}

func (r *Registry) ToLLMToolsForRole(role string) []llm.Tool {
	result := make([]llm.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool.AdminOnly() && role != "admin" {
			continue
		}
		result = append(result, llm.Tool{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.Schema(),
		})
	}
	return result
}
