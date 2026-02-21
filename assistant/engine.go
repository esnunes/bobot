// assistant/engine.go
package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/llm"
	"github.com/esnunes/bobot/tools"
)

// ContextProvider retrieves context messages for a user or topic.
type ContextProvider interface {
	GetContextMessages(userID int64) ([]ContextMessage, error)
	GetTopicContextMessages(topicID int64) ([]ContextMessage, error)
}

// ContextMessage represents a message for context (simplified from db.Message).
type ContextMessage struct {
	ID         int64
	Role       string
	Content    string
	RawContent string
	CreatedAt  time.Time
}

// ProfileProvider retrieves user or topic profile data.
type ProfileProvider interface {
	GetUserProfile(userID int64) (string, int64, error)
	GetTopicMemberProfiles(topicID int64) (string, error)
}

// SkillProvider retrieves user-defined skills for the current chat scope.
type SkillProvider interface {
	GetPrivateChatSkills(userID int64) ([]Skill, error)
	GetTopicSkills(topicID int64) ([]Skill, error)
}

// MessageSaver persists messages during the chat loop.
type MessageSaver interface {
	SaveMessage(userID int64, role, content, rawContent string) error
	SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error
}

// ChatOptions configures a Chat call.
type ChatOptions struct {
	Message     string
	TopicID     int64  // if > 0, topic chat; 0 means private chat
	DisplayName string // sender's display name (for topic message attribution)
}

type Engine struct {
	provider        llm.Provider
	registry        *tools.Registry
	skills          []Skill
	contextProvider ContextProvider
	profileProvider ProfileProvider
	skillProvider   SkillProvider
	messageSaver    MessageSaver
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

func (e *Engine) SetMessageSaver(saver MessageSaver) {
	e.messageSaver = saver
}

func (e *Engine) SetSkillProvider(provider SkillProvider) {
	e.skillProvider = provider
}

// Chat processes a user message and returns the assistant's response.
// The context must contain the user ID (set by auth middleware).
// For topic chats (TopicID > 0), it fetches topic context, injects member profiles,
// prepends [DisplayName] to user messages, and saves via SaveTopicMessage.
// For private chats (TopicID == 0), it behaves as before.
func (e *Engine) Chat(ctx context.Context, opts ChatOptions) (string, error) {
	userData := auth.UserDataFromContext(ctx)

	// Build system prompt with role-filtered tools
	llmTools := e.registry.ToLLMToolsForRole(userData.Role)

	// Merge built-in skills with user-defined skills
	allSkills := append([]Skill{}, e.skills...)
	if e.skillProvider != nil {
		var userSkills []Skill
		var skillErr error
		if opts.TopicID > 0 {
			userSkills, skillErr = e.skillProvider.GetTopicSkills(opts.TopicID)
		} else {
			userSkills, skillErr = e.skillProvider.GetPrivateChatSkills(userData.UserID)
		}
		if skillErr == nil {
			allSkills = append(allSkills, userSkills...)
		}
	}
	systemPrompt := BuildSystemPrompt(allSkills, llmTools)

	// Inject profiles
	if opts.TopicID > 0 {
		// Topic chat: inject all member profiles
		if e.profileProvider != nil {
			profiles, err := e.profileProvider.GetTopicMemberProfiles(opts.TopicID)
			if err == nil && profiles != "" {
				systemPrompt += "\n\n" + profiles
			}
		}
	} else {
		// Private chat: inject single user profile
		if e.profileProvider != nil {
			profileContent, _, err := e.profileProvider.GetUserProfile(userData.UserID)
			if err == nil && profileContent != "" {
				systemPrompt += "\n\n## User Profile\nThe following is known about the user you are chatting with:\n<user-profile>\n" + profileContent + "\n</user-profile>"
			}
		}
	}
	slog.Debug("chat system prompt", "content", systemPrompt, "topicID", opts.TopicID)

	// Get context messages
	var contextMsgs []ContextMessage
	var err error
	if opts.TopicID > 0 {
		contextMsgs, err = e.contextProvider.GetTopicContextMessages(opts.TopicID)
	} else {
		contextMsgs, err = e.contextProvider.GetContextMessages(userData.UserID)
	}

	var messages []llm.Message
	if err == nil {
		for _, cm := range contextMsgs {
			msg := llm.Message{Role: cm.Role}
			if cm.RawContent != "" {
				msg.Content = parseRawContent(cm.RawContent)
			} else {
				msg.Content = cm.Content
			}
			messages = append(messages, msg)
		}
	}

	// Add the new user message (with attribution for topic chat)
	userContent := opts.Message
	if opts.TopicID > 0 && opts.DisplayName != "" {
		userContent = fmt.Sprintf("[%s]: %s", opts.DisplayName, opts.Message)
	}
	userContent = fmt.Sprintf("[Current time (UTC): %s]\n%s", time.Now().UTC().Format("2006-01-02 15:04"), userContent)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userContent,
	})

	// Set ChatData for tool calls
	var chatData auth.ChatData
	if opts.TopicID > 0 {
		chatData.TopicID = &opts.TopicID
	}
	ctx = auth.ContextWithChatData(ctx, chatData)

	// Helper to save messages (private or topic)
	save := func(role, content, rawContent string) {
		if e.messageSaver == nil {
			return
		}
		if opts.TopicID > 0 {
			e.messageSaver.SaveTopicMessage(opts.TopicID, userData.UserID, role, content, rawContent)
		} else {
			e.messageSaver.SaveMessage(userData.UserID, role, content, rawContent)
		}
	}

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

		// If no tool calls, save final response and return
		if len(resp.ToolCalls) == 0 {
			save("assistant", resp.Content, resp.RawContent)
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

		// Save assistant tool_use message
		save("assistant", resp.Content, resp.RawContent)

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

		// Save tool_result message
		rawToolResults, _ := json.Marshal(toolResults)
		save("user", "", string(rawToolResults))
	}

	return "", fmt.Errorf("max iterations reached")
}

// ContextInspection holds the full LLM context for inspection.
type ContextInspection struct {
	SystemPrompt string
	Messages     []ContextMessage
	Tools        []llm.Tool
	TotalTokens  int
	MaxTokens    int
}

// InspectPrivateContext builds the full LLM context for a user's private chat without calling the LLM.
func (e *Engine) InspectPrivateContext(userID int64, role string) (*ContextInspection, error) {
	llmTools := e.registry.ToLLMToolsForRole(role)

	allSkills := append([]Skill{}, e.skills...)
	if e.skillProvider != nil {
		userSkills, err := e.skillProvider.GetPrivateChatSkills(userID)
		if err == nil {
			allSkills = append(allSkills, userSkills...)
		}
	}
	systemPrompt := BuildSystemPrompt(allSkills, llmTools)

	if e.profileProvider != nil {
		profileContent, _, err := e.profileProvider.GetUserProfile(userID)
		if err == nil && profileContent != "" {
			systemPrompt += "\n\n## User Profile\nThe following is known about the user you are chatting with:\n<user-profile>\n" + profileContent + "\n</user-profile>"
		}
	}

	contextMsgs, err := e.contextProvider.GetContextMessages(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get context messages: %w", err)
	}

	totalTokens := 0
	for _, cm := range contextMsgs {
		raw := cm.RawContent
		if raw == "" {
			raw = cm.Content
		}
		totalTokens += len(raw) / 4
	}

	return &ContextInspection{
		SystemPrompt: systemPrompt,
		Messages:     contextMsgs,
		Tools:        llmTools,
		TotalTokens:  totalTokens,
	}, nil
}

// InspectTopicContext builds the full LLM context for a topic chat without calling the LLM.
func (e *Engine) InspectTopicContext(topicID int64) (*ContextInspection, error) {
	llmTools := e.registry.ToLLMToolsForRole("user")

	allSkills := append([]Skill{}, e.skills...)
	if e.skillProvider != nil {
		topicSkills, err := e.skillProvider.GetTopicSkills(topicID)
		if err == nil {
			allSkills = append(allSkills, topicSkills...)
		}
	}
	systemPrompt := BuildSystemPrompt(allSkills, llmTools)

	if e.profileProvider != nil {
		profiles, err := e.profileProvider.GetTopicMemberProfiles(topicID)
		if err == nil && profiles != "" {
			systemPrompt += "\n\n" + profiles
		}
	}

	contextMsgs, err := e.contextProvider.GetTopicContextMessages(topicID)
	if err != nil {
		return nil, fmt.Errorf("failed to get topic context messages: %w", err)
	}

	totalTokens := 0
	for _, cm := range contextMsgs {
		raw := cm.RawContent
		if raw == "" {
			raw = cm.Content
		}
		totalTokens += len(raw) / 4
	}

	return &ContextInspection{
		SystemPrompt: systemPrompt,
		Messages:     contextMsgs,
		Tools:        llmTools,
		TotalTokens:  totalTokens,
	}, nil
}

// BuildRawJSON constructs the full Anthropic API request payload as JSON for inspection.
func (ci *ContextInspection) BuildRawJSON(model string, maxTokens int) (string, error) {
	type rawMsg struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}

	type rawTool struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema any    `json:"input_schema"`
	}

	type rawRequest struct {
		Model     string    `json:"model"`
		MaxTokens int       `json:"max_tokens"`
		System    string    `json:"system"`
		Messages  []rawMsg  `json:"messages"`
		Tools     []rawTool `json:"tools,omitempty"`
	}

	msgs := make([]rawMsg, 0, len(ci.Messages))
	for _, cm := range ci.Messages {
		msg := rawMsg{Role: cm.Role}
		if cm.RawContent != "" {
			msg.Content = parseRawContent(cm.RawContent)
		} else {
			msg.Content = cm.Content
		}
		msgs = append(msgs, msg)
	}

	tools := make([]rawTool, 0, len(ci.Tools))
	for _, t := range ci.Tools {
		tools = append(tools, rawTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	req := rawRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    ci.SystemPrompt,
		Messages:  msgs,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}

	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// parseRawContent converts stored raw_content back to the appropriate type
// for the LLM Message.Content field (string or []map[string]any).
func parseRawContent(raw string) interface{} {
	if len(raw) == 0 {
		return ""
	}
	// If it starts with '[', it's a JSON array (tool blocks)
	if raw[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal([]byte(raw), &arr); err == nil {
			return arr
		}
	}
	// Otherwise treat as plain string — strip surrounding quotes if present
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		return s
	}
	return raw
}
