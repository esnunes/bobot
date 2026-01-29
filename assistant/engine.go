// assistant/engine.go
package assistant

import (
	"context"
	"fmt"

	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

type Engine struct {
	provider llm.Provider
	registry *tools.Registry
	skills   []Skill
}

func NewEngine(provider llm.Provider, registry *tools.Registry, skills []Skill) *Engine {
	return &Engine{
		provider: provider,
		registry: registry,
		skills:   skills,
	}
}

// Chat processes a user message and returns the assistant's response.
// The context must contain the user ID (set by auth middleware).
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
	// Build system prompt
	systemPrompt := BuildSystemPrompt(e.skills, e.registry.ToLLMTools())

	// Start with user message
	messages := []llm.Message{
		{Role: "user", Content: message},
	}

	// Loop for tool use
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        e.registry.ToLLMTools(),
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// If no tool calls, return the response
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Build assistant message with tool use
		toolUseContent := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			toolUseContent = append(toolUseContent, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Name,
				"input": tc.Input,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "assistant",
			Content: toolUseContent,
		})

		// Execute tools and add results
		toolResults := make([]map[string]interface{}, 0)
		for _, tc := range resp.ToolCalls {
			result, err := e.registry.Execute(ctx, tc.Name, tc.Input)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
			}
			toolResults = append(toolResults, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": tc.ID,
				"content":     result,
			})
		}
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	return "", fmt.Errorf("max iterations reached")
}
