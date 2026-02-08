package db

import (
	"path/filepath"
	"testing"
)

func TestRawContentColumn(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a message with raw_content
	msg, err := coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Hello", "\"Hello\"", 1000, 4000,
	)
	if err != nil {
		t.Fatalf("failed to create message: %v", err)
	}

	if msg.RawContent != "\"Hello\"" {
		t.Errorf("expected raw_content '\"Hello\"', got '%s'", msg.RawContent)
	}

	// Verify tokens are estimated from raw_content length
	expectedTokens := len("\"Hello\"") / 4
	if msg.Tokens != expectedTokens {
		t.Errorf("expected tokens %d (from raw_content), got %d", expectedTokens, msg.Tokens)
	}
}

func TestContextWindowing_AtomicExchange(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Use small token limits to trigger windowing
	tokensStart := 50
	tokensMax := 100

	// Turn 1: user message
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Hello", "\"Hello\"", tokensStart, tokensMax,
	)

	// Turn 2: assistant with tool_use
	toolUseRaw := `[{"type":"tool_use","id":"c1","name":"task","input":{"cmd":"list"}}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "", toolUseRaw, tokensStart, tokensMax,
	)

	// Turn 3: tool_result
	toolResultRaw := `[{"type":"tool_result","tool_use_id":"c1","content":"done"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "", toolResultRaw, tokensStart, tokensMax,
	)

	// Turn 4: assistant final response
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "Here you go", `[{"type":"text","text":"Here you go"}]`, tokensStart, tokensMax,
	)

	// Turn 5: new user message (this may trigger windowing)
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "Another question", "\"Another question\"", tokensStart, tokensMax,
	)

	// Get context messages — should never start in the middle of a tool loop
	msgs, err := coreDB.GetPrivateChatContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get context: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected at least one context message")
	}

	// The first message in context should be a real user message (content != ""),
	// not a tool_result message
	first := msgs[0]
	if first.Role == "user" && first.Content == "" {
		t.Error("context window starts with a tool_result message — should start with a real user message")
	}
}

func TestFullConversationWithToolUse(t *testing.T) {
	tmpDir := t.TempDir()
	coreDB, err := NewCoreDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer coreDB.Close()

	user, _ := coreDB.CreateUser("testuser", "hash")
	tokensStart := 1000
	tokensMax := 4000

	// Simulate: user asks about weather
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "What's the weather in Paris?",
		"\"What's the weather in Paris?\"",
		tokensStart, tokensMax,
	)

	// Assistant responds with tool_use
	assistantToolUseRaw := `[{"type":"text","text":"Let me check the weather."},{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"location":"Paris"}}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "Let me check the weather.", assistantToolUseRaw,
		tokensStart, tokensMax,
	)

	// Tool result
	toolResultRaw := `[{"type":"tool_result","tool_use_id":"toolu_01","content":"{\"temp\":18,\"condition\":\"sunny\"}"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		user.ID, BobotUserID, "user", "", toolResultRaw,
		tokensStart, tokensMax,
	)

	// Assistant final response
	finalRaw := `[{"type":"text","text":"It's 18°C and sunny in Paris!"}]`
	coreDB.CreatePrivateMessageWithContextThreshold(
		BobotUserID, user.ID, "assistant", "It's 18°C and sunny in Paris!", finalRaw,
		tokensStart, tokensMax,
	)

	// Get context messages
	msgs, err := coreDB.GetPrivateChatContextMessages(user.ID)
	if err != nil {
		t.Fatalf("failed to get context: %v", err)
	}

	// Should have 4 messages: user, assistant+tool_use, tool_result, assistant_final
	if len(msgs) != 4 {
		t.Fatalf("expected 4 context messages, got %d", len(msgs))
	}

	// Verify roles
	expectedRoles := []string{"user", "assistant", "user", "assistant"}
	for i, expected := range expectedRoles {
		if msgs[i].Role != expected {
			t.Errorf("msg %d: expected role '%s', got '%s'", i, expected, msgs[i].Role)
		}
	}

	// Verify raw_content is preserved
	if msgs[1].RawContent != assistantToolUseRaw {
		t.Errorf("assistant tool_use raw_content not preserved")
	}
	if msgs[2].RawContent != toolResultRaw {
		t.Errorf("tool_result raw_content not preserved")
	}

	// Verify tool_result has empty content
	if msgs[2].Content != "" {
		t.Errorf("tool_result should have empty content, got '%s'", msgs[2].Content)
	}
}
