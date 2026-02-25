package quickaction

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
)

type QuickActionTool struct {
	db *db.CoreDB
}

func NewQuickActionTool(db *db.CoreDB) *QuickActionTool {
	return &QuickActionTool{db: db}
}

func (q *QuickActionTool) Name() string {
	return "quickaction"
}

func (q *QuickActionTool) Description() string {
	return "Manage quick actions: create, update, delete, list shortcut prompts for this chat. Quick actions let users send common messages with a single tap."
}

func (q *QuickActionTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "update", "delete", "list"},
				"description": "The operation to perform",
			},
			"label": map[string]any{
				"type":        "string",
				"description": "Short button label for the quick action (e.g. 'Turn on AC'). Used to identify the action for update/delete. Label matching is case-insensitive.",
			},
			"new_label": map[string]any{
				"type":        "string",
				"description": "New label when renaming a quick action (only used with update command).",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The full message text that will be sent or filled into the input when the action is triggered (e.g. '@bobot turn on the AC in the living room')",
			},
			"mode": map[string]any{
				"type":        "string",
				"enum":        []string{"send", "fill"},
				"description": "How the action behaves: 'send' sends the message immediately, 'fill' puts it in the input field for editing. Defaults to 'send'.",
			},
		},
		"required": []string{"command"},
	}
}

func (q *QuickActionTool) AdminOnly() bool {
	return false
}

func (q *QuickActionTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("missing command. Usage: /quickaction <command>")
	}

	result := map[string]any{"command": parts[0]}
	if len(parts) > 1 {
		result["label"] = strings.Join(parts[1:], " ")
	}
	return result, nil
}

func (q *QuickActionTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	chatData := auth.ChatDataFromContext(ctx)

	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("missing command. Usage: /quickaction <command>")
	}

	label, _ := input["label"].(string)
	newLabel, _ := input["new_label"].(string)
	message, _ := input["message"].(string)
	mode, _ := input["mode"].(string)

	switch command {
	case "create":
		return q.create(userData, chatData, label, message, mode)
	case "update":
		return q.update(userData, chatData, label, newLabel, message, mode)
	case "delete":
		return q.deleteQA(userData, chatData, label)
	case "list":
		return q.list(userData, chatData)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (q *QuickActionTool) canManage(userID int64, role string, topicID int64) error {
	topic, err := q.db.GetTopicByID(topicID)
	if err != nil {
		return fmt.Errorf("topic not found")
	}
	return auth.CanManageTopicResource(role, userID, topic.OwnerID)
}

func (q *QuickActionTool) create(userData auth.UserData, chatData auth.ChatData, label, message, mode string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", fmt.Errorf("missing label. Usage: /quickaction create <label>")
	}
	if len(label) > 100 {
		return "", fmt.Errorf("label must be 100 characters or less")
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("missing message")
	}
	if len(message) > 2000 {
		return "", fmt.Errorf("message must be 2000 characters or less")
	}

	if mode == "" {
		mode = "send"
	}
	if mode != "send" && mode != "fill" {
		return "", fmt.Errorf("mode must be 'send' or 'fill'")
	}

	topicID := chatData.TopicID
	if err := q.canManage(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	qa, err := q.db.CreateQuickAction(userData.UserID, topicID, label, message, mode)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a quick action labeled %q already exists in this topic", label)
		}
		return "", fmt.Errorf("failed to create quick action: %w", err)
	}

	q.warnIfTooMany(topicID)

	return fmt.Sprintf("Quick action %q created (mode: %s).", qa.Label, qa.Mode), nil
}

func (q *QuickActionTool) update(userData auth.UserData, chatData auth.ChatData, label, newLabel, message, mode string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", fmt.Errorf("missing label. Usage: /quickaction update <label>")
	}

	topicID := chatData.TopicID
	qa, err := q.resolve(topicID, label)
	if err != nil {
		return "", err
	}

	if err := q.canManage(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	// Keep existing values if not provided
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		newLabel = qa.Label
	}
	if len(newLabel) > 100 {
		return "", fmt.Errorf("label must be 100 characters or less")
	}
	newMessage := message
	newMode := mode
	if newMessage == "" {
		newMessage = qa.Message
	}
	if newMode == "" {
		newMode = qa.Mode
	}
	if len(newMessage) > 2000 {
		return "", fmt.Errorf("message must be 2000 characters or less")
	}
	if newMode != "send" && newMode != "fill" {
		return "", fmt.Errorf("mode must be 'send' or 'fill'")
	}

	if err := q.db.UpdateQuickAction(qa.ID, newLabel, newMessage, newMode); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return "", fmt.Errorf("a quick action labeled %q already exists in this topic", newLabel)
		}
		return "", fmt.Errorf("failed to update quick action: %w", err)
	}

	return fmt.Sprintf("Quick action %q updated.", newLabel), nil
}

func (q *QuickActionTool) deleteQA(userData auth.UserData, chatData auth.ChatData, label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "", fmt.Errorf("missing label. Usage: /quickaction delete <label>")
	}

	topicID := chatData.TopicID
	qa, err := q.resolve(topicID, label)
	if err != nil {
		return "", err
	}

	if err := q.canManage(userData.UserID, userData.Role, topicID); err != nil {
		return "", err
	}

	if err := q.db.DeleteQuickAction(qa.ID); err != nil {
		return "", fmt.Errorf("failed to delete quick action: %w", err)
	}

	return fmt.Sprintf("Quick action %q deleted.", label), nil
}

func (q *QuickActionTool) list(userData auth.UserData, chatData auth.ChatData) (string, error) {
	isMember, err := q.db.IsTopicMember(chatData.TopicID, userData.UserID)
	if err != nil || !isMember {
		return "", fmt.Errorf("you are not a member of this topic")
	}

	actions, err := q.db.GetTopicQuickActions(chatData.TopicID)
	if err != nil {
		return "", fmt.Errorf("failed to list quick actions: %w", err)
	}

	if len(actions) == 0 {
		return "No quick actions found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Quick actions:\n")
	for _, qa := range actions {
		sb.WriteString(fmt.Sprintf("- %s [%s]: %s\n", qa.Label, qa.Mode, qa.Message))
	}
	return sb.String(), nil
}

func (q *QuickActionTool) resolve(topicID int64, label string) (*db.QuickActionRow, error) {
	qa, err := q.db.GetTopicQuickActionByLabel(topicID, label)
	if err == db.ErrNotFound {
		return nil, fmt.Errorf("quick action not found: %s", label)
	}
	return qa, err
}

func (q *QuickActionTool) warnIfTooMany(topicID int64) {
	count, err := q.db.CountTopicQuickActions(topicID)
	if err == nil && count > 20 {
		slog.Warn("topic exceeds 20 quick actions", "topicID", topicID, "count", count)
	}
}
