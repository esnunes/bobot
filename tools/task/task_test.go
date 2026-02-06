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

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	result, err := tool.Execute(ctx, map[string]any{"command": "create", "project": "groceries", "title": "milk"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") {
		t.Errorf("expected result to contain 'milk', got: %s", result)
	}
}

func TestTaskTool_List(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	tool.Execute(ctx, map[string]any{"command": "create", "project": "groceries", "title": "milk"})
	tool.Execute(ctx, map[string]any{"command": "create", "project": "groceries", "title": "eggs"})

	result, err := tool.Execute(ctx, map[string]any{"command": "list", "project": "groceries"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "milk") || !strings.Contains(result, "eggs") {
		t.Errorf("expected result to contain milk and eggs, got: %s", result)
	}
}

func TestTaskTool_Update(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	tool.Execute(ctx, map[string]any{"command": "create", "project": "groceries", "title": "milk"})

	result, err := tool.Execute(ctx, map[string]any{"command": "update", "project": "groceries", "title": "milk", "status": "done"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "done") {
		t.Errorf("expected result to indicate done status, got: %s", result)
	}
}

func TestTaskTool_Delete(t *testing.T) {
	tool := setupTestTool(t)

	ctx := auth.ContextWithUserData(context.Background(), auth.UserData{UserID: 1})
	tool.Execute(ctx, map[string]any{"command": "create", "project": "groceries", "title": "milk"})

	result, err := tool.Execute(ctx, map[string]any{"command": "delete", "project": "groceries", "title": "milk"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "deleted") {
		t.Errorf("expected result to indicate deletion, got: %s", result)
	}
}

func TestTaskTool_ParseArgs(t *testing.T) {
	tool := &TaskTool{}

	tests := []struct {
		name    string
		raw     string
		want    map[string]any
		wantErr bool
	}{
		{
			name:    "empty input",
			raw:     "",
			wantErr: true,
		},
		{
			name:    "only command, missing project",
			raw:     "create",
			wantErr: true,
		},
		{
			name: "create with project and title",
			raw:  "create groceries buy milk",
			want: map[string]any{"command": "create", "project": "groceries", "title": "buy milk"},
		},
		{
			name: "list with project only",
			raw:  "list groceries",
			want: map[string]any{"command": "list", "project": "groceries"},
		},
		{
			name: "update with status flag",
			raw:  "update groceries buy milk --status=done",
			want: map[string]any{"command": "update", "project": "groceries", "title": "buy milk", "status": "done"},
		},
		{
			name: "list with status filter",
			raw:  "list groceries --status=pending",
			want: map[string]any{"command": "list", "project": "groceries", "status": "pending"},
		},
		{
			name: "delete with multi-word title",
			raw:  "delete groceries organic whole milk",
			want: map[string]any{"command": "delete", "project": "groceries", "title": "organic whole milk"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("key %q: got %v, want %v", k, got[k], v)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d keys, want %d keys", len(got), len(tt.want))
			}
		})
	}
}
