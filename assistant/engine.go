// assistant/engine.go
package assistant

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

// ContextProvider retrieves context messages for a user.
type ContextProvider interface {
	GetContextMessages(userID int64) ([]ContextMessage, error)
}

// ContextMessage represents a message for context (simplified from db.Message).
type ContextMessage struct {
	Role    string
	Content string
}

// ProfileProvider retrieves user profile data.
type ProfileProvider interface {
	GetUserProfile(userID int64) (string, int64, error)
}

type Engine struct {
	provider        llm.Provider
	registry        *tools.Registry
	skills          []Skill
	contextProvider ContextProvider
	profileProvider ProfileProvider
}

func NewEngine(provider llm.Provider, registry *tools.Registry, skills []Skill, contextProvider ContextProvider, profileProvider ProfileProvider) *Engine {
	return &Engine{
		provider:        provider,
		registry:        registry,
		skills:          skills,
		contextProvider: contextProvider,
		profileProvider: profileProvider,
	}
}

// Chat processes a user message and returns the assistant's response.
// The context must contain the user ID (set by auth middleware).
func (e *Engine) Chat(ctx context.Context, message string) (string, error) {
	// Get user data from context
	userData := auth.UserDataFromContext(ctx)

	// Build system prompt with role-filtered tools
	llmTools := e.registry.ToLLMToolsForRole(userData.Role)
	systemPrompt := BuildSystemPrompt(e.skills, llmTools)

	// Inject user profile if available
	if e.profileProvider != nil {
		profileContent, _, err := e.profileProvider.GetUserProfile(userData.UserID)
		if err == nil && profileContent != "" {
			systemPrompt += "\n\n## User Profile\nThe following is known about the user you are chatting with:\n<user-profile>\n" + profileContent + "\n</user-profile>"
		}
	}
	slog.Debug("private chat system prompt", "content", systemPrompt)

	// Build messages with context
	var messages []llm.Message

	// Get context messages
	contextMsgs, err := e.contextProvider.GetContextMessages(userData.UserID)
	if err == nil {
		for _, cm := range contextMsgs {
			messages = append(messages, llm.Message{
				Role:    cm.Role,
				Content: cm.Content,
			})
		}
	}

	// Add the new user message
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: message,
	})

	// Loop for tool use
	maxIterations := 10
	for range maxIterations {
		resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
			SystemPrompt: systemPrompt,
			Messages:     messages,
			Tools:        llmTools,
		})
		if err != nil {
			return "", fmt.Errorf("LLM error: %w", err)
		}

		// If no tool calls, return the response
		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Build assistant message with tool use
		toolUseContent := make([]map[string]any, 0)
		for _, tc := range resp.ToolCalls {
			slog.Info("llm tool call requested", "tool", tc.Name, "id", tc.ID, "input", tc.Input)
			toolUseContent = append(toolUseContent, map[string]any{
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
		toolResults := make([]map[string]any, 0)
		for _, tc := range resp.ToolCalls {
			result, err := e.registry.Execute(ctx, tc.Name, tc.Input)
			if err != nil {
				slog.Error("llm tool call failed", "tool", tc.Name, "id", tc.ID, "error", err)
				result = fmt.Sprintf("Error: %v", err)
			} else {
				slog.Info("llm tool call result", "tool", tc.Name, "id", tc.ID, "result", result)
			}
			toolResults = append(toolResults, map[string]any{
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

// ChatWithContext processes a topic conversation and returns the assistant's response.
// The conversation is a list of lines formatted as "[Name]: message content" for users
// or "[assistant]: message content" for assistant messages.
func (e *Engine) ChatWithContext(ctx context.Context, conversation []string) (string, error) {
	// Build messages from conversation
	var messages []llm.Message

	// Add conversation history
	for _, line := range conversation {
		if strings.HasPrefix(line, "[assistant]:") {
			content := strings.TrimPrefix(line, "[assistant]: ")
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: content,
			})
		} else {
			messages = append(messages, llm.Message{
				Role:    "user",
				Content: line,
			})
		}
	}

	// Add system prompt for topic context
	systemPrompt := `You are a helpful AI assistant participating in a topic chat.
Messages are formatted as [Name]: message content.
Only respond when specifically addressed with @bobot.
Keep responses concise and relevant to the conversation.`

	resp, err := e.provider.Chat(ctx, &llm.ChatRequest{
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        nil, // No tools for group chat for now
	})
	if err != nil {
		return "", fmt.Errorf("LLM error: %w", err)
	}

	return resp.Content, nil
}
