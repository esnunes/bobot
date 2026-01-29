// assistant/prompt.go
package assistant

import (
	"fmt"
	"strings"

	"github.com/esnunes/bobot/llm"
)

const basePrompt = `You are bobot, a helpful AI assistant for managing daily family tasks. You are friendly, concise, and efficient.

Respond adaptively:
- Be terse for simple, clear requests (e.g., "Added milk")
- Be conversational when clarification is needed (e.g., "Added milk. Whole or skim?")

Always use available tools when appropriate to help users manage their tasks.`

func BuildSystemPrompt(skills []Skill, tools []llm.Tool) string {
	var sb strings.Builder

	sb.WriteString(basePrompt)
	sb.WriteString("\n\n")

	if len(skills) > 0 {
		sb.WriteString("## Skills\n\n")
		for _, skill := range skills {
			sb.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				sb.WriteString(fmt.Sprintf("*%s*\n\n", skill.Description))
			}
			sb.WriteString(skill.Content)
			sb.WriteString("\n\n")
		}
	}

	if len(tools) > 0 {
		sb.WriteString("## Available Tools\n\n")
		for _, tool := range tools {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, tool.Description))
		}
	}

	return sb.String()
}
