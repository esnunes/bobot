package schedule

import (
	"context"
	"testing"
)

func TestCronTool_Name(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)
	if tool.Name() != "cron" {
		t.Errorf("got %q, want %q", tool.Name(), "cron")
	}
}

func TestCronTool_ParseArgs(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	tests := []struct {
		name    string
		raw     string
		wantCmd string
		wantErr bool
	}{
		{"list", "list", "list", false},
		{"delete", "delete 5", "delete", false},
		{"enable", "enable 3", "enable", false},
		{"disable", "disable 3", "disable", false},
		{"create", "0 9 * * 1-5 summarize my tasks", "create", false},
		{"empty", "", "", true},
		{"too few create fields", "0 9 * * prompt", "", true},
		{"delete missing id", "delete", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.ParseArgs(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cmd, _ := result["command"].(string); cmd != tt.wantCmd {
				t.Errorf("got command %q, want %q", cmd, tt.wantCmd)
			}
		})
	}
}

func TestCronTool_ParseArgs_Create(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	result, err := tool.ParseArgs("0 9 * * 1-5 summarize my open tasks")
	if err != nil {
		t.Fatal(err)
	}
	if result["cron_expr"] != "0 9 * * 1-5" {
		t.Errorf("got cron_expr %q, want %q", result["cron_expr"], "0 9 * * 1-5")
	}
	if result["prompt"] != "summarize my open tasks" {
		t.Errorf("got prompt %q, want %q", result["prompt"], "summarize my open tasks")
	}
}

func TestCronTool_Create(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 9 * * 1-5",
		"prompt":    "summarize my tasks",
		"name":      "daily tasks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Verify job was created
	jobs, _ := db.ListCronJobs(1)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "daily tasks" {
		t.Errorf("got name %q, want %q", jobs[0].Name, "daily tasks")
	}
	if jobs[0].Prompt != "summarize my tasks" {
		t.Errorf("got prompt %q", jobs[0].Prompt)
	}
	if !jobs[0].Enabled {
		t.Error("expected job to be enabled")
	}
}

func TestCronTool_CreateInvalidExpr(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	_, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "invalid cron",
		"prompt":    "test",
	})
	if err == nil {
		t.Error("expected error for invalid cron expression")
	}
}

func TestCronTool_CreateMaxJobs(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 2) // max 2 jobs

	// Create 2 jobs
	for i := 0; i < 2; i++ {
		_, err := tool.Execute(ctxWithUser(1), map[string]any{
			"command":   "create",
			"cron_expr": "0 9 * * *",
			"prompt":    "test",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Third should fail
	_, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 10 * * *",
		"prompt":    "too many",
	})
	if err == nil {
		t.Error("expected error when exceeding max jobs")
	}
}

func TestCronTool_List(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 9 * * 1-5",
		"prompt":    "weekday task",
		"name":      "daily",
	})

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "No cron jobs." {
		t.Error("expected jobs to be listed")
	}
}

func TestCronTool_ListEmpty(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "list",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "No cron jobs." {
		t.Errorf("got %q, want 'No cron jobs.'", result)
	}
}

func TestCronTool_Delete(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 9 * * *",
		"prompt":    "delete me",
	})

	result, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "delete",
		"id":      float64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	// Verify deleted
	jobs, _ := db.ListCronJobs(1)
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after delete, got %d", len(jobs))
	}
}

func TestCronTool_DeleteWrongUser(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 9 * * *",
		"prompt":    "not yours",
	})

	_, err := tool.Execute(ctxWithUser(2), map[string]any{
		"command": "delete",
		"id":      float64(1),
	})
	if err == nil {
		t.Error("expected error when deleting another user's job")
	}
}

func TestCronTool_EnableDisable(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	tool.Execute(ctxWithUser(1), map[string]any{
		"command":   "create",
		"cron_expr": "0 9 * * *",
		"prompt":    "test",
	})

	// Disable
	_, err := tool.Execute(ctxWithUser(1), map[string]any{
		"command": "disable",
		"id":      float64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	j, _ := db.GetCronJob(1)
	if j.Enabled {
		t.Error("expected disabled")
	}

	// Enable
	_, err = tool.Execute(ctxWithUser(1), map[string]any{
		"command": "enable",
		"id":      float64(1),
	})
	if err != nil {
		t.Fatal(err)
	}
	j, _ = db.GetCronJob(1)
	if !j.Enabled {
		t.Error("expected enabled")
	}
}

func TestCronTool_NoContext(t *testing.T) {
	db := newTestDB(t)
	tool := NewCronTool(db, 10)

	_, err := tool.Execute(context.Background(), map[string]any{
		"command": "list",
	})
	if err == nil {
		t.Error("expected error without user context")
	}
}
