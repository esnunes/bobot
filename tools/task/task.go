// tools/task/task.go
package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/esnunes/bobot/auth"
)

type TaskTool struct {
	db *TaskDB
}

func NewTaskTool(db *TaskDB) *TaskTool {
	return &TaskTool{db: db}
}

func (t *TaskTool) Name() string {
	return "task"
}

func (t *TaskTool) Description() string {
	return "Manage tasks within projects"
}

func (t *TaskTool) Schema() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"create", "list", "update", "delete"},
				"description": "The operation to perform",
			},
			"project": map[string]interface{}{
				"type":        "string",
				"description": "Project name (e.g., 'groceries')",
			},
			"title": map[string]interface{}{
				"type":        "string",
				"description": "Task title",
			},
			"status": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"pending", "done"},
				"description": "Task status",
			},
		},
		"required": []string{"command", "project"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, input map[string]interface{}) (string, error) {
	userID := auth.UserIDFromContext(ctx)
	if userID == 0 {
		return "", fmt.Errorf("user_id not found in context")
	}

	command, _ := input["command"].(string)
	projectName, _ := input["project"].(string)
	title, _ := input["title"].(string)
	status, _ := input["status"].(string)

	project, err := t.db.GetOrCreateProject(userID, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to get/create project: %w", err)
	}

	switch command {
	case "create":
		return t.create(project.ID, title)
	case "list":
		return t.list(project.ID, status)
	case "update":
		return t.update(project.ID, title, status)
	case "delete":
		return t.delete(project.ID, title)
	default:
		return "", fmt.Errorf("unknown command: %s", command)
	}
}

func (t *TaskTool) create(projectID int64, title string) (string, error) {
	task, err := t.db.CreateTask(projectID, title)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Created task: %s (id: %d)", task.Name, task.ID), nil
}

func (t *TaskTool) list(projectID int64, status string) (string, error) {
	tasks, err := t.db.ListTasks(projectID, status)
	if err != nil {
		return "", err
	}

	if len(tasks) == 0 {
		return "No tasks found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Tasks:\n")
	for _, task := range tasks {
		sb.WriteString(fmt.Sprintf("- [%s] %s\n", task.Status, task.Name))
	}
	return sb.String(), nil
}

func (t *TaskTool) update(projectID int64, title, status string) (string, error) {
	task, err := t.db.FindTaskByName(projectID, title)
	if err != nil {
		return "", fmt.Errorf("task not found: %s", title)
	}

	if err := t.db.UpdateTaskStatus(task.ID, status); err != nil {
		return "", err
	}

	return fmt.Sprintf("Updated task '%s' to status: %s", title, status), nil
}

func (t *TaskTool) delete(projectID int64, title string) (string, error) {
	task, err := t.db.FindTaskByName(projectID, title)
	if err != nil {
		return "", fmt.Errorf("task not found: %s", title)
	}

	if err := t.db.DeleteTask(task.ID); err != nil {
		return "", err
	}

	return fmt.Sprintf("Task '%s' deleted.", title), nil
}
