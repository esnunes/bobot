package skill

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type SkillTool struct {
	db *db.CoreDB
}

func NewSkillTool(db *db.CoreDB) *SkillTool {
	return &SkillTool{db: db}
}

func (s *SkillTool) Name() string {
	return "skill"
}

func (s *SkillTool) Description() string {
	return "Manage skills: create, update, delete, list custom skills for this chat"
}

func (s *SkillTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "update", "delete", "list"},
				"description": "The operation to perform",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Skill name",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short description of what the skill does",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Markdown content with instructions for the skill",
			},
		},
		"required": []string{"command"},
	}
}

func (s *SkillTool) AdminOnly() bool {
	return false
}

func (s *SkillTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing command. Usage: /skill <command>")
	}

	result := map[string]any{"command": parts[0]}
	if len(parts) > 1 {
		result["name"] = strings.Join(parts[1:], " ")
	}
	return result, nil
}

func (s *SkillTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	chatData := auth.ChatDataFromContext(ctx)

	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command. Usage: /skill <command>")
	}

	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	content, _ := input["content"].(string)

	switch command {
	case "create":
		return s.create(userData, chatData, name, description, content)
	case "update":
		return s.update(userData, chatData, name, description, content)
	case "delete":
		return s.deleteSk(userData, chatData, name)
	case "list":
		return s.list(userData, chatData)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (s *SkillTool) canManageTopicSkills(userID int64, role string, topicID int64) error {
	if role == "admin" {
		return nil
	}
	topic, err := s.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	if topic.OwnerID != userID {
		return fmt.Errorf("only the topic owner or admins can manage topic skills")
	}
	return nil
}

func (s *SkillTool) create(userData auth.UserData, chatData auth.ChatData, name, description, content string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill create <name>")
	}

	topicID := *chatData.TopicID
	if err := s.canManageTopicSkills(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	if len(content) > 4096 {
		slog.Warn("skill content exceeds 4KB", "name", name, "size", len(content))
	}

	skill, err := s.db.CreateSkill(userData.UserID, chatData.TopicID, name, description, content)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a skill named %q already exists in this scope", name)
		}
		return "", fmt.Errorf("failed to create skill: %w", err)
	}

	s.warnIfTooManySkills(topicID)

	return fmt.Sprintf("Skill %q created.", skill.Name), nil
}

func (s *SkillTool) update(userData auth.UserData, chatData auth.ChatData, name, description, content string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill update <name>")
	}

	topicID := *chatData.TopicID
	skill, err := s.resolveSkill(topicID, name)
	if err != nil {
		return "", err
	}

	if err := s.canManageTopicSkills(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	if len(content) > 4096 {
		slog.Warn("skill content exceeds 4KB", "name", name, "size", len(content))
	}

	// Keep existing values if not provided
	if description == "" {
		description = skill.Description
	}
	if content == "" {
		content = skill.Content
	}

	if err := s.db.UpdateSkill(skill.ID, description, content); err != nil {
		return "", fmt.Errorf("failed to update skill: %w", err)
	}

	return fmt.Sprintf("Skill %q updated.", name), nil
}

func (s *SkillTool) deleteSk(userData auth.UserData, chatData auth.ChatData, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("missing skill name. Usage: /skill delete <name>")
	}

	topicID := *chatData.TopicID
	skill, err := s.resolveSkill(topicID, name)
	if err != nil {
		return "", err
	}

	if err := s.canManageTopicSkills(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	if err := s.db.DeleteSkill(skill.ID); err != nil {
		return "", fmt.Errorf("failed to delete skill: %w", err)
	}

	return fmt.Sprintf("Skill %q deleted.", name), nil
}

func (s *SkillTool) list(userData auth.UserData, chatData auth.ChatData) (string, error) {
	var skills []db.SkillRow
	var err error

	skills, err = s.db.GetTopicSkills(*chatData.TopicID)
	if err != nil {
		return "", fmt.Errorf("failed to list skills: %w", err)
	}

	if len(skills) == 0 {
		return "No skills found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Skills:\n")
	for _, sk := range skills {
		if sk.Description != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", sk.Name, sk.Description))
		} else {
			sb.WriteString(fmt.Sprintf("- %s\n", sk.Name))
		}
	}
	return sb.String(), nil
}

func (s *SkillTool) resolveSkill(topicID int64, name string) (*db.SkillRow, error) {
	skill, err := s.db.GetTopicSkillByName(topicID, name)
	if err == db.ErrNotFound {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	return skill, err
}

func (s *SkillTool) warnIfTooManySkills(topicID int64) {
	skills, err := s.db.GetTopicSkills(topicID)
	if err == nil && len(skills) > 10 {
		slog.Warn("scope exceeds 10 skills", "topicID", topicID, "count", len(skills))
	}
}
