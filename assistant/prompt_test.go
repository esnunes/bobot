// assistant/prompt_test.go
package assistant

import (
	"strings"
	"testing"

	"github.com/esnunes/bobot/llm"
)

func TestBuildSystemPrompt(t *testing.T) {
	skills := []Skill{
		{Name: "groceries", Description: "Manage groceries", Content: "Use task tool for groceries."},
	}
	tools := []llm.Tool{
		{Name: "task", Description: "Manage tasks"},
	}

	prompt := BuildSystemPrompt(skills, tools)

	if !strings.Contains(prompt, "groceries") {
		t.Error("expected prompt to contain skill name")
	}
	if !strings.Contains(prompt, "task") {
		t.Error("expected prompt to contain tool name")
	}
	if !strings.Contains(prompt, "Use task tool for groceries") {
		t.Error("expected prompt to contain skill content")
	}
}

func TestBuildSystemPrompt_IncludesCurrentTime(t *testing.T) {
	prompt := BuildSystemPrompt(nil, nil)
	if !strings.Contains(prompt, "Current date and time (UTC):") {
		t.Error("expected prompt to contain current UTC time")
	}
}

func TestBuildSystemPrompt_NoSkills(t *testing.T) {
	tools := []llm.Tool{
		{Name: "task", Description: "Manage tasks"},
	}

	prompt := BuildSystemPrompt(nil, tools)

	if prompt == "" {
		t.Error("expected non-empty prompt even without skills")
	}
}
