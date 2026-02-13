// assistant/engine_test.go
package assistant

import (
	"context"
	"strings"
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
	engine := NewEngine(mockProvider, registry, nil, mockCtxProvider, nil)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, ChatOptions{Message: "Hi"})
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
	engine := NewEngine(mockProvider, registry, nil, mockCtxProvider, nil)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, ChatOptions{Message: "What's on my list?"})
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

func (m *mockContextProvider) GetTopicContextMessages(topicID int64) ([]ContextMessage, error) {
	return m.messages, nil
}

func TestEngine_Chat_WithContextMessages(t *testing.T) {
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
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, nil)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, ChatOptions{Message: "new question"})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	// Should have 3 messages: 2 from context + 1 new
	if len(capturedMessages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(capturedMessages))
	}

	// Last message should contain the new question (with time prefix)
	lastContent, _ := capturedMessages[2].Content.(string)
	if !strings.Contains(lastContent, "new question") {
		t.Errorf("expected last message to contain 'new question', got %v", capturedMessages[2].Content)
	}
}

type mockTopicContextProvider struct {
	privateMessages []ContextMessage
	topicMessages   []ContextMessage
}

func (m *mockTopicContextProvider) GetContextMessages(userID int64) ([]ContextMessage, error) {
	return m.privateMessages, nil
}

func (m *mockTopicContextProvider) GetTopicContextMessages(topicID int64) ([]ContextMessage, error) {
	return m.topicMessages, nil
}

type mockTopicProfileProvider struct {
	userProfiles  map[int64]string
	topicProfiles map[int64]string
}

func (m *mockTopicProfileProvider) GetUserProfile(userID int64) (string, int64, error) {
	content := m.userProfiles[userID]
	return content, 0, nil
}

func (m *mockTopicProfileProvider) GetTopicMemberProfiles(topicID int64) (string, error) {
	return m.topicProfiles[topicID], nil
}

type mockTopicMessageSaver struct {
	privateMessages []savedMessage
	topicMessages   []savedTopicMessage
}

type savedTopicMessage struct {
	TopicID    int64
	UserID     int64
	Role       string
	Content    string
	RawContent string
}

func (m *mockTopicMessageSaver) SaveMessage(userID int64, role, content, rawContent string) error {
	m.privateMessages = append(m.privateMessages, savedMessage{
		UserID: userID, Role: role, Content: content, RawContent: rawContent,
	})
	return nil
}

func (m *mockTopicMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	m.topicMessages = append(m.topicMessages, savedTopicMessage{
		TopicID: topicID, UserID: userID, Role: role, Content: content, RawContent: rawContent,
	})
	return nil
}

func TestEngine_Chat_TopicSimpleResponse(t *testing.T) {
	var capturedSystemPrompt string
	var capturedMessages []llm.Message
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			capturedMessages = req.Messages
			return &llm.ChatResponse{Content: "Got it!"}, nil
		},
	}

	topicCtx := &mockTopicContextProvider{
		topicMessages: []ContextMessage{
			{Role: "user", Content: "hello", RawContent: "[Alice]: hello"},
			{Role: "assistant", Content: "hi there", RawContent: "hi there"},
		},
	}

	topicProfile := &mockTopicProfileProvider{
		topicProfiles: map[int64]string{
			42: "## Topic Members\n\n<member name=\"Alice\">\nLikes coffee.\n</member>",
		},
	}

	saver := &mockTopicMessageSaver{}
	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, topicCtx, topicProfile)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})
	result, err := engine.Chat(ctx, ChatOptions{
		Message:     "Hey @bobot, what's up?",
		TopicID:     42,
		DisplayName: "Bob",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Got it!" {
		t.Errorf("expected 'Got it!', got '%s'", result)
	}

	// System prompt should contain topic member profiles
	if !strings.Contains(capturedSystemPrompt, "Topic Members") {
		t.Error("expected system prompt to contain topic member profiles")
	}
	if !strings.Contains(capturedSystemPrompt, "Alice") {
		t.Error("expected system prompt to contain Alice's profile")
	}

	// Should NOT contain single user profile section
	if strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to NOT contain <user-profile> tags in topic chat")
	}

	// Context messages should use raw_content
	if len(capturedMessages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(capturedMessages))
	}
	// First context message should use raw_content (with attribution)
	if capturedMessages[0].Content != "[Alice]: hello" {
		t.Errorf("expected first context message to be '[Alice]: hello', got '%v'", capturedMessages[0].Content)
	}

	// New user message should have DisplayName prepended (with time prefix)
	lastMsg := capturedMessages[len(capturedMessages)-1]
	lastContent, _ := lastMsg.Content.(string)
	if !strings.Contains(lastContent, "[Bob]: Hey @bobot, what's up?") {
		t.Errorf("expected last message to contain '[Bob]: Hey @bobot, what's up?', got '%v'", lastMsg.Content)
	}

	// Should have saved via SaveTopicMessage, not SaveMessage
	if len(saver.topicMessages) != 1 {
		t.Fatalf("expected 1 topic message saved, got %d", len(saver.topicMessages))
	}
	if saver.topicMessages[0].TopicID != 42 {
		t.Errorf("expected topicID 42, got %d", saver.topicMessages[0].TopicID)
	}
	if len(saver.privateMessages) != 0 {
		t.Errorf("expected 0 private messages saved, got %d", len(saver.privateMessages))
	}
}

func TestEngine_Chat_TopicWithTools(t *testing.T) {
	mockProv := &mockLLM{
		responses: []*llm.ChatResponse{
			{
				Content:    "Let me check.",
				RawContent: `[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"call_1","name":"task","input":{"command":"list"}}]`,
				StopType:   "tool_use",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "task", Input: map[string]interface{}{"command": "list"}},
				},
			},
			{Content: "Here are the tasks.", StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk"})

	topicCtx := &mockTopicContextProvider{}
	saver := &mockTopicMessageSaver{}
	engine := NewEngine(mockProv, registry, nil, topicCtx, nil)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})
	result, err := engine.Chat(ctx, ChatOptions{
		Message:     "list tasks",
		TopicID:     10,
		DisplayName: "Alice",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are the tasks." {
		t.Errorf("unexpected result: %s", result)
	}

	// Should have saved 3 topic messages (tool_use, tool_result, final response)
	if len(saver.topicMessages) != 3 {
		t.Fatalf("expected 3 topic messages, got %d", len(saver.topicMessages))
	}
	for _, m := range saver.topicMessages {
		if m.TopicID != 10 {
			t.Errorf("expected topicID 10, got %d", m.TopicID)
		}
	}
}

type mockProfileProvider struct {
	profiles map[int64]string
}

func (m *mockProfileProvider) GetUserProfile(userID int64) (string, int64, error) {
	content, ok := m.profiles[userID]
	if !ok {
		return "", 0, nil
	}
	return content, 0, nil
}

func (m *mockProfileProvider) GetTopicMemberProfiles(topicID int64) (string, error) {
	return "", nil
}

func TestEngine_Chat_InjectsProfile(t *testing.T) {
	var capturedSystemPrompt string
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "Hello Eduardo!"}, nil
		},
	}

	mockCtxProvider := &mockContextProvider{messages: nil}
	mockProfile := &mockProfileProvider{
		profiles: map[int64]string{
			1: "Eduardo lives in Berlin. Prefers concise responses.",
		},
	}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, mockProfile)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, ChatOptions{Message: "Hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedSystemPrompt, "Eduardo lives in Berlin") {
		t.Error("expected system prompt to contain user profile")
	}
	if !strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to contain <user-profile> tags")
	}
}

type savedMessage struct {
	UserID     int64
	Role       string
	Content    string
	RawContent string
}

type mockMessageSaver struct {
	messages []savedMessage
}

func (m *mockMessageSaver) SaveMessage(userID int64, role, content, rawContent string) error {
	m.messages = append(m.messages, savedMessage{
		UserID:     userID,
		Role:       role,
		Content:    content,
		RawContent: rawContent,
	})
	return nil
}

func (m *mockMessageSaver) SaveTopicMessage(topicID, userID int64, role, content, rawContent string) error {
	return nil
}

func TestEngine_Chat_PersistsToolLoop(t *testing.T) {
	mockProv := &mockLLM{
		responses: []*llm.ChatResponse{
			{
				Content:    "Let me check.",
				RawContent: `[{"type":"text","text":"Let me check."},{"type":"tool_use","id":"call_1","name":"task","input":{"command":"list"}}]`,
				StopType:   "tool_use",
				ToolCalls: []llm.ToolCall{
					{ID: "call_1", Name: "task", Input: map[string]interface{}{"command": "list"}},
				},
			},
			{
				Content:    "Here are your tasks.",
				RawContent: `[{"type":"text","text":"Here are your tasks."}]`,
				StopType:   "end_turn",
			},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&mockTool{result: "Tasks: milk"})

	saver := &mockMessageSaver{}
	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, nil)
	engine.SetMessageSaver(saver)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, ChatOptions{Message: "What's on my list?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Here are your tasks." {
		t.Errorf("unexpected result: %s", result)
	}

	// Should have saved 3 messages:
	// 1. assistant tool_use message
	// 2. user tool_result message
	// 3. assistant final response
	if len(saver.messages) != 3 {
		t.Fatalf("expected 3 saved messages, got %d", len(saver.messages))
	}

	// First: assistant tool_use
	if saver.messages[0].Role != "assistant" {
		t.Errorf("msg 0: expected role 'assistant', got '%s'", saver.messages[0].Role)
	}
	if saver.messages[0].Content != "Let me check." {
		t.Errorf("msg 0: expected content 'Let me check.', got '%s'", saver.messages[0].Content)
	}

	// Second: user tool_result (content should be empty)
	if saver.messages[1].Role != "user" {
		t.Errorf("msg 1: expected role 'user', got '%s'", saver.messages[1].Role)
	}
	if saver.messages[1].Content != "" {
		t.Errorf("msg 1: expected empty content for tool_result, got '%s'", saver.messages[1].Content)
	}

	// Third: assistant final response
	if saver.messages[2].Role != "assistant" {
		t.Errorf("msg 2: expected role 'assistant', got '%s'", saver.messages[2].Role)
	}
	if saver.messages[2].Content != "Here are your tasks." {
		t.Errorf("msg 2: expected content 'Here are your tasks.', got '%s'", saver.messages[2].Content)
	}
}

func TestEngine_Chat_NoSaver_StillWorks(t *testing.T) {
	mockProv := &mockLLM{
		responses: []*llm.ChatResponse{
			{Content: "Hello!", RawContent: `[{"type":"text","text":"Hello!"}]`, StopType: "end_turn"},
		},
	}

	registry := tools.NewRegistry()
	mockCtxProvider := &mockContextProvider{messages: nil}
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, nil)
	// No saver set — should still work

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := engine.Chat(ctx, ChatOptions{Message: "Hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("expected 'Hello!', got '%s'", result)
	}
}

func TestEngine_Chat_NoProfileNoInjection(t *testing.T) {
	var capturedSystemPrompt string
	mockProv := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "Hello!"}, nil
		},
	}

	mockCtxProvider := &mockContextProvider{messages: nil}
	mockProfile := &mockProfileProvider{profiles: map[int64]string{}}

	registry := tools.NewRegistry()
	engine := NewEngine(mockProv, registry, nil, mockCtxProvider, mockProfile)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	_, err := engine.Chat(ctx, ChatOptions{Message: "Hi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(capturedSystemPrompt, "<user-profile>") {
		t.Error("expected system prompt to NOT contain profile tags when profile is empty")
	}
}

type mockSkillProvider struct {
	privateSkills map[int64][]Skill
	topicSkills   map[int64][]Skill
}

func (m *mockSkillProvider) GetPrivateChatSkills(userID int64) ([]Skill, error) {
	if m.privateSkills == nil {
		return nil, nil
	}
	return m.privateSkills[userID], nil
}

func (m *mockSkillProvider) GetTopicSkills(topicID int64) ([]Skill, error) {
	if m.topicSkills == nil {
		return nil, nil
	}
	return m.topicSkills[topicID], nil
}

func TestEngine_MergesUserSkillsIntoPrompt(t *testing.T) {
	var capturedSystemPrompt string
	prov := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "ok", RawContent: `"ok"`}, nil
		},
	}

	registry := tools.NewRegistry()
	builtinSkills := []Skill{{Name: "builtin", Description: "Built-in skill", Content: "builtin content"}}

	skillProvider := &mockSkillProvider{
		privateSkills: map[int64][]Skill{
			1: {{Name: "custom", Description: "Custom skill", Content: "custom content"}},
		},
	}

	engine := NewEngine(prov, registry, builtinSkills, &mockContextProvider{}, nil)
	engine.SetSkillProvider(skillProvider)
	engine.SetMessageSaver(&mockMessageSaver{})

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})

	engine.Chat(ctx, ChatOptions{Message: "hello"})

	if capturedSystemPrompt == "" {
		t.Fatal("expected system prompt to be set")
	}

	// Built-in skill should appear
	if !strings.Contains(capturedSystemPrompt, "builtin content") {
		t.Error("expected builtin skill in system prompt")
	}
	// User-defined skill should appear
	if !strings.Contains(capturedSystemPrompt, "custom content") {
		t.Error("expected custom skill in system prompt")
	}
	// Builtin should appear before custom
	builtinIdx := strings.Index(capturedSystemPrompt, "builtin content")
	customIdx := strings.Index(capturedSystemPrompt, "custom content")
	if builtinIdx > customIdx {
		t.Error("expected builtin skills before custom skills")
	}
}

func TestEngine_TopicSkillsInTopicChat(t *testing.T) {
	var capturedSystemPrompt string
	prov := &mockProvider{
		chatFunc: func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedSystemPrompt = req.SystemPrompt
			return &llm.ChatResponse{Content: "ok", RawContent: `"ok"`}, nil
		},
	}

	registry := tools.NewRegistry()

	skillProvider := &mockSkillProvider{
		topicSkills: map[int64][]Skill{
			42: {{Name: "topic-skill", Description: "Topic skill", Content: "topic skill content"}},
		},
	}

	engine := NewEngine(prov, registry, nil, &mockContextProvider{}, nil)
	engine.SetSkillProvider(skillProvider)
	engine.SetMessageSaver(&mockMessageSaver{})

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1, Role: "user"})

	engine.Chat(ctx, ChatOptions{Message: "hello", TopicID: 42})

	if !strings.Contains(capturedSystemPrompt, "topic skill content") {
		t.Error("expected topic skill in system prompt")
	}
}
