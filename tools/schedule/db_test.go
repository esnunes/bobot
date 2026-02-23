package schedule

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *ScheduleDB {
	t.Helper()
	dir := t.TempDir()
	db, err := NewScheduleDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestReminderCRUD(t *testing.T) {
	db := newTestDB(t)

	runAt := time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)

	// Create
	id, err := db.CreateReminder(1, 0, "call dentist", runAt)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	// Get
	r, err := db.GetReminder(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Message != "call dentist" {
		t.Errorf("got message %q, want %q", r.Message, "call dentist")
	}
	if r.Status != "pending" {
		t.Errorf("got status %q, want %q", r.Status, "pending")
	}
	if r.UserID != 1 {
		t.Errorf("got user_id %d, want 1", r.UserID)
	}
	if r.TopicID != 0 {
		t.Errorf("got topic_id %v, want 0", r.TopicID)
	}
	if !r.RunAt.Equal(runAt) {
		t.Errorf("got run_at %v, want %v", r.RunAt, runAt)
	}

	// Create with topic
	id2, err := db.CreateReminder(1, 42, "topic reminder", runAt)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := db.GetReminder(id2)
	if r2.TopicID != 42 {
		t.Errorf("got topic_id %v, want 42", r2.TopicID)
	}

	// List pending
	reminders, err := db.ListPendingReminders(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 2 {
		t.Errorf("got %d reminders, want 2", len(reminders))
	}

	// List by topic
	topicReminders, err := db.ListPendingRemindersByTopic(1, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(topicReminders) != 1 {
		t.Errorf("got %d topic reminders, want 1", len(topicReminders))
	}

	// Cancel
	err = db.CancelReminder(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	r, _ = db.GetReminder(id)
	if r.Status != "cancelled" {
		t.Errorf("got status %q, want %q", r.Status, "cancelled")
	}

	// Cancel wrong user
	err = db.CancelReminder(id2, 999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Cancel already cancelled
	err = db.CancelReminder(id, 1)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for already cancelled, got %v", err)
	}
}

func TestReminderGetNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := db.GetReminder(999)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetDueReminders(t *testing.T) {
	db := newTestDB(t)

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	db.CreateReminder(1, 0, "past reminder", past)
	db.CreateReminder(1, 0, "future reminder", future)

	due, err := db.GetDueReminders(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due reminders, want 1", len(due))
	}
	if due[0].Message != "past reminder" {
		t.Errorf("got message %q, want %q", due[0].Message, "past reminder")
	}
}

func TestMarkReminderExecuted(t *testing.T) {
	db := newTestDB(t)

	runAt := time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)
	id, _ := db.CreateReminder(1, 0, "test", runAt)

	execAt := time.Date(2026, 2, 12, 15, 1, 0, 0, time.UTC)
	err := db.MarkReminderExecuted(id, execAt)
	if err != nil {
		t.Fatal(err)
	}

	r, _ := db.GetReminder(id)
	if r.Status != "executed" {
		t.Errorf("got status %q, want %q", r.Status, "executed")
	}
	if r.ExecutedAt == nil || !r.ExecutedAt.Equal(execAt) {
		t.Errorf("got executed_at %v, want %v", r.ExecutedAt, execAt)
	}
}

func TestMarkReminderFailed(t *testing.T) {
	db := newTestDB(t)

	runAt := time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)
	id, _ := db.CreateReminder(1, 0, "test", runAt)

	err := db.MarkReminderFailed(id, "execution timeout")
	if err != nil {
		t.Fatal(err)
	}

	r, _ := db.GetReminder(id)
	if r.Status != "failed" {
		t.Errorf("got status %q, want %q", r.Status, "failed")
	}
	if r.Error == nil || *r.Error != "execution timeout" {
		t.Errorf("got error %v, want %q", r.Error, "execution timeout")
	}
}

func TestCronJobCRUD(t *testing.T) {
	db := newTestDB(t)

	nextRun := time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC)

	// Create
	id, err := db.CreateCronJob(1, 0, "daily tasks", "summarize my tasks", "0 9 * * 1-5", nextRun)
	if err != nil {
		t.Fatal(err)
	}

	// Get
	j, err := db.GetCronJob(id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Name != "daily tasks" {
		t.Errorf("got name %q, want %q", j.Name, "daily tasks")
	}
	if j.Prompt != "summarize my tasks" {
		t.Errorf("got prompt %q, want %q", j.Prompt, "summarize my tasks")
	}
	if j.CronExpr != "0 9 * * 1-5" {
		t.Errorf("got cron_expr %q, want %q", j.CronExpr, "0 9 * * 1-5")
	}
	if !j.Enabled {
		t.Error("expected enabled=true")
	}
	if !j.NextRunAt.Equal(nextRun) {
		t.Errorf("got next_run_at %v, want %v", j.NextRunAt, nextRun)
	}

	// List
	jobs, err := db.ListCronJobs(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want 1", len(jobs))
	}

	// Update
	newNextRun := time.Date(2026, 2, 14, 9, 0, 0, 0, time.UTC)
	err = db.UpdateCronJob(id, 1, "updated name", "new prompt", "0 10 * * *", true, newNextRun)
	if err != nil {
		t.Fatal(err)
	}
	j, _ = db.GetCronJob(id)
	if j.Name != "updated name" {
		t.Errorf("got name %q, want %q", j.Name, "updated name")
	}
	if j.Prompt != "new prompt" {
		t.Errorf("got prompt %q, want %q", j.Prompt, "new prompt")
	}

	// Update wrong user
	err = db.UpdateCronJob(id, 999, "hack", "hack", "* * * * *", true, newNextRun)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// Disable
	err = db.SetCronJobEnabled(id, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	j, _ = db.GetCronJob(id)
	if j.Enabled {
		t.Error("expected enabled=false")
	}

	// Enable
	err = db.SetCronJobEnabled(id, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	j, _ = db.GetCronJob(id)
	if !j.Enabled {
		t.Error("expected enabled=true")
	}

	// Delete
	err = db.DeleteCronJob(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.GetCronJob(id)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestGetDueCronJobs(t *testing.T) {
	db := newTestDB(t)

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	db.CreateCronJob(1, 0, "past job", "prompt1", "0 9 * * *", past)
	db.CreateCronJob(1, 0, "future job", "prompt2", "0 9 * * *", future)

	// Create a disabled job in the past
	id3, _ := db.CreateCronJob(1, 0, "disabled past", "prompt3", "0 9 * * *", past)
	db.SetCronJobEnabled(id3, 1, false)

	due, err := db.GetDueCronJobs(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("got %d due jobs, want 1", len(due))
	}
	if due[0].Name != "past job" {
		t.Errorf("got name %q, want %q", due[0].Name, "past job")
	}
}

func TestCronJobByTopic(t *testing.T) {
	db := newTestDB(t)

	nextRun := time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC)
	db.CreateCronJob(1, 5, "topic job", "prompt", "0 9 * * *", nextRun)
	db.CreateCronJob(1, 0, "private job", "prompt2", "0 9 * * *", nextRun)

	jobs, err := db.ListCronJobsByTopic(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Name != "topic job" {
		t.Errorf("got name %q, want %q", jobs[0].Name, "topic job")
	}
}

func TestExecutionCRUD(t *testing.T) {
	db := newTestDB(t)

	nextRun := time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC)
	jobID, _ := db.CreateCronJob(1, 0, "job", "prompt", "0 9 * * *", nextRun)

	scheduledAt := time.Date(2026, 2, 13, 9, 0, 0, 0, time.UTC)
	startedAt := time.Date(2026, 2, 13, 9, 0, 1, 0, time.UTC)

	// Create
	execID, err := db.CreateExecution(jobID, scheduledAt, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	if execID == 0 {
		t.Fatal("expected non-zero exec id")
	}

	// Complete
	completedAt := time.Date(2026, 2, 13, 9, 0, 30, 0, time.UTC)
	err = db.CompleteExecution(execID, completedAt)
	if err != nil {
		t.Fatal(err)
	}

	// Create another and fail it
	execID2, _ := db.CreateExecution(jobID, scheduledAt, startedAt)
	err = db.FailExecution(execID2, completedAt, "timeout")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewScheduleDBCreatesDir(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "nested", "test.db")

	db, err := NewScheduleDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Verify the directory was created
	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
