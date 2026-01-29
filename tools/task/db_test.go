// tools/task/db_test.go
package task

import (
	"path/filepath"
	"testing"
)

func TestTaskDB_CreateProject(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	if err != nil {
		t.Fatalf("failed to create db: %v", err)
	}
	defer db.Close()

	proj, err := db.CreateProject(1, "groceries")
	if err != nil {
		t.Fatalf("failed to create project: %v", err)
	}
	if proj.Name != "groceries" {
		t.Errorf("expected name groceries, got %s", proj.Name)
	}
}

func TestTaskDB_GetOrCreateProject(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	p1, _ := db.GetOrCreateProject(1, "groceries")
	p2, _ := db.GetOrCreateProject(1, "groceries")

	if p1.ID != p2.ID {
		t.Error("expected same project ID")
	}
}

func TestTaskDB_CreateTask(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, err := db.CreateTask(proj.ID, "milk")
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if task.Name != "milk" {
		t.Errorf("expected name milk, got %s", task.Name)
	}
	if task.Status != "pending" {
		t.Errorf("expected status pending, got %s", task.Status)
	}
}

func TestTaskDB_ListTasks(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	db.CreateTask(proj.ID, "milk")
	db.CreateTask(proj.ID, "eggs")

	tasks, err := db.ListTasks(proj.ID, "")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestTaskDB_ListTasks_FilterByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	db.CreateTask(proj.ID, "milk")
	task2, _ := db.CreateTask(proj.ID, "eggs")
	db.UpdateTaskStatus(task2.ID, "done")

	pending, _ := db.ListTasks(proj.ID, "pending")
	if len(pending) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(pending))
	}

	done, _ := db.ListTasks(proj.ID, "done")
	if len(done) != 1 {
		t.Errorf("expected 1 done task, got %d", len(done))
	}
}

func TestTaskDB_UpdateTaskStatus(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, _ := db.CreateTask(proj.ID, "milk")

	err := db.UpdateTaskStatus(task.ID, "done")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, _ := db.GetTask(task.ID)
	if updated.Status != "done" {
		t.Errorf("expected status done, got %s", updated.Status)
	}
}

func TestTaskDB_DeleteTask(t *testing.T) {
	tmpDir := t.TempDir()
	db, _ := NewTaskDB(filepath.Join(tmpDir, "task.db"))
	defer db.Close()

	proj, _ := db.CreateProject(1, "groceries")
	task, _ := db.CreateTask(proj.ID, "milk")

	err := db.DeleteTask(task.ID)
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	_, err = db.GetTask(task.ID)
	if err == nil {
		t.Error("expected error getting deleted task")
	}
}
