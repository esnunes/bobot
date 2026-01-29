// tools/task/task_test.go
package task

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/esnunes/bobot/auth"
)

func setupTestTool(t *testing.T) *TaskTool {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	return NewTaskTool(db)
}

func TestTaskTool_Name(t *testing.T) {
	tool := setupTestTool(t)
	if tool.Name() != "task" {
		t.Errorf("expected name 'task', got '%s'", tool.Name())
	}
}

func TestTaskTool_Create(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") {
		t.Errorf("expected result to contain 'milk', got: %s", result)
	}
}

func TestTaskTool_List(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "eggs",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "list",
		"project": "groceries",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") || !strings.Contains(result, "eggs") {
		t.Errorf("expected result to contain milk and eggs, got: %s", result)
	}
}

func TestTaskTool_Update(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "update",
		"project": "groceries",
		"title":   "milk",
		"status":  "done",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "done") {
		t.Errorf("expected result to indicate done status, got: %s", result)
	}
}

func TestTaskTool_Delete(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserID(context.Background(), 1)
	tool.Execute(ctx, map[string]interface{}{
		"command": "create",
		"project": "groceries",
		"title":   "milk",
	})

	result, err := tool.Execute(ctx, map[string]interface{}{
		"command": "delete",
		"project": "groceries",
		"title":   "milk",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected result to indicate deletion, got: %s", result)
	}
}
