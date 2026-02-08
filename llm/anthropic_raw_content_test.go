package llm

import (
	"encoding/json"
	"testing"
)

func TestChatResponse_RawContent_TextOnly(t *testing.T) {
	apiResp := anthropicResponse{
		Content: []anthropicContent{
			{Type: "text", Text: "Hello!"},
		},
		StopReason: "end_turn",
	}

	result := buildChatResponse(&apiResp)

	if result.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got '%s'", result.Content)
	}

	// RawContent should be the JSON array of content blocks
	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal([]byte(result.RawContent), &rawBlocks); err != nil {
		t.Fatalf("failed to parse RawContent as JSON: %v", err)
	}
	if len(rawBlocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(rawBlocks))
	}
	if rawBlocks[0]["type"] != "text" {
		t.Errorf("expected type 'text', got '%v'", rawBlocks[0]["type"])
	}
}

func TestChatResponse_RawContent_WithToolUse(t *testing.T) {
	apiResp := anthropicResponse{
		Content: []anthropicContent{
			{Type: "text", Text: "Let me check."},
			{Type: "tool_use", ID: "call_1", Name: "get_weather", Input: map[string]interface{}{"location": "Paris"}},
		},
		StopReason: "tool_use",
	}

	result := buildChatResponse(&apiResp)

	if result.Content != "Let me check." {
		t.Errorf("expected content 'Let me check.', got '%s'", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}

	var rawBlocks []map[string]interface{}
	if err := json.Unmarshal([]byte(result.RawContent), &rawBlocks); err != nil {
		t.Fatalf("failed to parse RawContent as JSON: %v", err)
	}
	if len(rawBlocks) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(rawBlocks))
	}
}
