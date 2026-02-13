package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
)

type RemindTool struct {
	db *ScheduleDB
}

func NewRemindTool(db *ScheduleDB) *RemindTool {
	return &RemindTool{db: db}
}

func (t *RemindTool) Name() string {
	return "remind"
}

func (t *RemindTool) Description() string {
	return "Create one-shot reminders that fire at a specific time. When the reminder fires, the message is sent back to you (the LLM) as a new chat message for you to respond to. Store the user's request verbatim — do NOT answer or resolve it now."
}

func (t *RemindTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "list", "cancel"},
				"description": "The operation to perform",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The user's original request, stored verbatim. This will be sent back as a prompt when the reminder fires. Do NOT answer or rephrase it — store exactly what the user asked.",
			},
			"run_at": map[string]any{
				"type":        "string",
				"description": "When to fire the reminder, ISO 8601 datetime in UTC (e.g. 2026-02-12T15:00:00Z). Convert from user's local time using their profile timezone.",
			},
			"id": map[string]any{
				"type":        "integer",
				"description": "Reminder ID (for cancel)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *RemindTool) AdminOnly() bool {
	return false
}

// ParseArgs handles slash command format: /remind <ISO-datetime-UTC> <message>
// For list: /remind list
// For cancel: /remind cancel <id>
func (t *RemindTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("usage: /remind <datetime> <message>, /remind list, /remind cancel <id>")
	}

	switch parts[0] {
	case "list":
		return map[string]any{"command": "list"}, nil
	case "cancel":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /remind cancel <id>")
		}
		// Parse ID
		var id int64
		if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid reminder ID: %s", parts[1])
		}
		return map[string]any{"command": "cancel", "id": id}, nil
	default:
		// create: /remind <datetime> <message>
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /remind <datetime-UTC> <message>")
		}
		runAt := parts[0]
		message := strings.Join(parts[1:], " ")
		return map[string]any{
			"command": "create",
			"run_at":  runAt,
			"message": message,
		}, nil
	}
}

func (t *RemindTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	userData := auth.UserDataFromContext(ctx)
	if userData.UserID == 0 {
		return "", fmt.Errorf("user_id not found in context")
	}

	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}

	chatData := auth.ChatDataFromContext(ctx)

	switch command {
	case "create":
		return t.create(userData.UserID, chatData.TopicID, input)
	case "list":
		return t.list(userData.UserID, chatData.TopicID)
	case "cancel":
		return t.cancel(userData.UserID, input)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *RemindTool) create(userID int64, topicID *int64, input map[string]any) (string, error) {
	message, _ := input["message"].(string)
	if message == "" {
		return "", fmt.Errorf("message is required")
	}

	runAtStr, _ := input["run_at"].(string)
	if runAtStr == "" {
		return "", fmt.Errorf("run_at is required")
	}

	runAt, err := time.Parse(time.RFC3339, runAtStr)
	if err != nil {
		// Try without timezone suffix
		runAt, err = time.Parse("2006-01-02T15:04:05Z", runAtStr)
		if err != nil {
			return "", fmt.Errorf("invalid datetime format, use ISO 8601 UTC (e.g. 2026-02-12T15:00:00Z)")
		}
	}
	runAt = runAt.UTC()

	if runAt.Before(time.Now().UTC()) {
		return "", fmt.Errorf("run_at must be in the future")
	}

	id, err := t.db.CreateReminder(userID, topicID, message, runAt)
	if err != nil {
		return "", fmt.Errorf("failed to create reminder: %w", err)
	}

	return fmt.Sprintf("Reminder #%d created. I'll remind you at %s UTC: %s",
		id, runAt.Format("2006-01-02 15:04"), message), nil
}

func (t *RemindTool) list(userID int64, topicID *int64) (string, error) {
	var reminders []Reminder
	var err error

	if topicID != nil {
		reminders, err = t.db.ListPendingRemindersByTopic(userID, *topicID)
	} else {
		reminders, err = t.db.ListPendingReminders(userID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to list reminders: %w", err)
	}

	if len(reminders) == 0 {
		return "No pending reminders.", nil
	}

	var sb strings.Builder
	sb.WriteString("Pending reminders:\n")
	for _, r := range reminders {
		sb.WriteString(fmt.Sprintf("- #%d: %s (at %s UTC)\n", r.ID, r.Message, r.RunAt.Format("2006-01-02 15:04")))
	}
	return sb.String(), nil
}

func (t *RemindTool) cancel(userID int64, input map[string]any) (string, error) {
	// Handle both float64 (from JSON) and int64 types
	var id int64
	switch v := input["id"].(type) {
	case float64:
		id = int64(v)
	case int64:
		id = v
	default:
		return "", fmt.Errorf("id is required")
	}
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	err := t.db.CancelReminder(id, userID)
	if err == ErrNotFound {
		return "", fmt.Errorf("reminder #%d not found or already cancelled", id)
	}
	if err != nil {
		return "", fmt.Errorf("failed to cancel reminder: %w", err)
	}

	return fmt.Sprintf("Reminder #%d cancelled.", id), nil
}
