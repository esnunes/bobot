package schedule

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/esnunes/bobot/auth"
)

type CronTool struct {
	db      *ScheduleDB
	maxJobs int
}

func NewCronTool(db *ScheduleDB, maxJobs int) *CronTool {
	return &CronTool{db: db, maxJobs: maxJobs}
}

func (t *CronTool) Name() string {
	return "cron"
}

func (t *CronTool) Description() string {
	return "Manage recurring scheduled prompts that execute on a cron schedule. Prompts run through the full LLM pipeline as if the user typed them."
}

func (t *CronTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"enum":        []string{"create", "list", "delete", "enable", "disable"},
				"description": "The operation to perform",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt to run on schedule",
			},
			"cron_expr": map[string]any{
				"type":        "string",
				"description": "5-field cron expression (minute hour day-of-month month day-of-week)",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Optional human-readable name for the cron job",
			},
			"id": map[string]any{
				"type":        "integer",
				"description": "Cron job ID (for delete/enable/disable)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *CronTool) AdminOnly() bool {
	return false
}

// ParseArgs handles slash command format: /cron <cron-expr-5-fields> <prompt>
// Other sub-commands: /cron list, /cron delete <id>, /cron enable <id>, /cron disable <id>
func (t *CronTool) ParseArgs(raw string) (map[string]any, error) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil, fmt.Errorf("usage: /cron <minute> <hour> <dom> <month> <dow> <prompt>")
	}

	switch parts[0] {
	case "list":
		return map[string]any{"command": "list"}, nil
	case "delete", "enable", "disable":
		if len(parts) < 2 {
			return nil, fmt.Errorf("usage: /cron %s <id>", parts[0])
		}
		var id int64
		if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid ID: %s", parts[1])
		}
		return map[string]any{"command": parts[0], "id": id}, nil
	default:
		// create: first 5 tokens are cron expr, rest is prompt
		if len(parts) < 6 {
			return nil, fmt.Errorf("usage: /cron <minute> <hour> <dom> <month> <dow> <prompt>")
		}
		cronExpr := strings.Join(parts[:5], " ")
		prompt := strings.Join(parts[5:], " ")
		return map[string]any{
			"command":   "create",
			"cron_expr": cronExpr,
			"prompt":    prompt,
		}, nil
	}
}

func (t *CronTool) Execute(ctx context.Context, input map[string]any) (string, error) {
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
	case "delete":
		return t.delete(userData.UserID, input)
	case "enable":
		return t.setEnabled(userData.UserID, input, true)
	case "disable":
		return t.setEnabled(userData.UserID, input, false)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *CronTool) create(userID int64, topicID *int64, input map[string]any) (string, error) {
	cronExprStr, _ := input["cron_expr"].(string)
	if cronExprStr == "" {
		return "", fmt.Errorf("cron_expr is required")
	}

	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	name, _ := input["name"].(string)

	// Parse and validate cron expression
	expr, err := Parse(cronExprStr)
	if err != nil {
		return "", fmt.Errorf("invalid cron expression: %w", err)
	}

	// Check minimum interval (15 minutes)
	interval := MinInterval(expr)
	if interval < 15*time.Minute {
		return "", fmt.Errorf("cron interval too frequent (minimum 15 minutes, got %v)", interval)
	}

	// Check max jobs limit
	count, err := t.db.CountEnabledCronJobs(userID)
	if err != nil {
		return "", fmt.Errorf("failed to count cron jobs: %w", err)
	}
	if count >= t.maxJobs {
		return "", fmt.Errorf("maximum of %d active cron jobs reached", t.maxJobs)
	}

	// Compute next run time
	nextRunAt := expr.Next(time.Now().UTC())

	id, err := t.db.CreateCronJob(userID, topicID, name, prompt, cronExprStr, nextRunAt)
	if err != nil {
		return "", fmt.Errorf("failed to create cron job: %w", err)
	}

	displayName := name
	if displayName == "" {
		displayName = truncate(prompt, 40)
	}

	return fmt.Sprintf("Cron job #%d created: %s\nSchedule: %s\nNext run: %s UTC",
		id, displayName, cronExprStr, nextRunAt.Format("2006-01-02 15:04")), nil
}

func (t *CronTool) list(userID int64, topicID *int64) (string, error) {
	var jobs []CronJob
	var err error

	if topicID != nil {
		jobs, err = t.db.ListCronJobsByTopic(*topicID)
	} else {
		jobs, err = t.db.ListCronJobs(userID)
	}
	if err != nil {
		return "", fmt.Errorf("failed to list cron jobs: %w", err)
	}

	if len(jobs) == 0 {
		return "No cron jobs.", nil
	}

	var sb strings.Builder
	sb.WriteString("Cron jobs:\n")
	for _, j := range jobs {
		status := "enabled"
		if !j.Enabled {
			status = "disabled"
		}
		displayName := j.Name
		if displayName == "" {
			displayName = truncate(j.Prompt, 40)
		}
		sb.WriteString(fmt.Sprintf("- #%d: %s [%s] (%s) next: %s UTC\n",
			j.ID, displayName, status, j.CronExpr, j.NextRunAt.Format("2006-01-02 15:04")))
	}
	return sb.String(), nil
}

func (t *CronTool) delete(userID int64, input map[string]any) (string, error) {
	id := extractID(input)
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	err := t.db.DeleteCronJob(id, userID)
	if err == ErrNotFound {
		return "", fmt.Errorf("cron job #%d not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("failed to delete cron job: %w", err)
	}

	return fmt.Sprintf("Cron job #%d deleted.", id), nil
}

func (t *CronTool) setEnabled(userID int64, input map[string]any, enabled bool) (string, error) {
	id := extractID(input)
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	err := t.db.SetCronJobEnabled(id, userID, enabled)
	if err == ErrNotFound {
		return "", fmt.Errorf("cron job #%d not found", id)
	}
	if err != nil {
		return "", fmt.Errorf("failed to update cron job: %w", err)
	}

	action := "enabled"
	if !enabled {
		action = "disabled"
	}
	return fmt.Sprintf("Cron job #%d %s.", id, action), nil
}

func extractID(input map[string]any) int64 {
	switch v := input["id"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
