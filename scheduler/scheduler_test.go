package scheduler

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools/schedule"
)

// mockPipeline records calls to SendMessage.
type mockPipeline struct {
	mu       sync.Mutex
	calls    []messageCall
	failNext bool
}

type messageCall struct {
	UserID      int64
	TopicID     int64
	Content     string
	DisplayName string
}

func (m *mockPipeline) SendMessage(ctx context.Context, userID int64, topicID int64, content string, displayName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, messageCall{UserID: userID, TopicID: topicID, Content: content, DisplayName: displayName})
	if m.failNext {
		m.failNext = false
		return "", fmt.Errorf("mock error")
	}
	return "ok", nil
}

func setupTest(t *testing.T) (*schedule.ScheduleDB, *db.CoreDB, *mockPipeline) {
	t.Helper()
	dir := t.TempDir()

	schedDB, err := schedule.NewScheduleDB(filepath.Join(dir, "schedule.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { schedDB.Close() })

	coreDB, err := db.NewCoreDB(filepath.Join(dir, "core.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { coreDB.Close() })

	return schedDB, coreDB, &mockPipeline{}
}

func TestExecuteReminder(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	// Create a test user with a bobot topic (needed for nil-topic reminders)
	user, err := coreDB.CreateUser("testuser", "hash")
	if err != nil {
		t.Fatal(err)
	}
	bobotTopic, err := coreDB.CreateBobotTopic(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Create a due reminder with bobot topic
	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := bobotTopic.ID
	schedDB.CreateReminder(user.ID, tid, "call dentist", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pipeline.calls))
	}
	wantContent := "<bobot-remind>call dentist</bobot-remind>"
	if pipeline.calls[0].Content != wantContent {
		t.Errorf("got content %q, want %q", pipeline.calls[0].Content, wantContent)
	}
	if pipeline.calls[0].UserID != user.ID {
		t.Errorf("got user_id %d, want %d", pipeline.calls[0].UserID, user.ID)
	}
	if pipeline.calls[0].TopicID != bobotTopic.ID {
		t.Errorf("got topic_id %d, want %d", pipeline.calls[0].TopicID, bobotTopic.ID)
	}

	// Verify reminder is marked as executed
	r, _ := schedDB.GetReminder(1)
	if r.Status != "executed" {
		t.Errorf("got status %q, want %q", r.Status, "executed")
	}
}

func TestExecuteReminderInTopic(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, err := coreDB.CreateUser("testuser", "hash")
	if err != nil {
		t.Fatal(err)
	}

	topic, err := coreDB.CreateTopic("test-topic", user.ID)
	if err != nil {
		t.Fatal(err)
	}
	coreDB.AddTopicMember(topic.ID, user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := topic.ID
	schedDB.CreateReminder(user.ID, tid, "topic reminder", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pipeline.calls))
	}
	if pipeline.calls[0].Content != "<bobot-remind>topic reminder</bobot-remind>" {
		t.Errorf("got content %q", pipeline.calls[0].Content)
	}
	if pipeline.calls[0].TopicID != topic.ID {
		t.Errorf("got topic_id %d, want %d", pipeline.calls[0].TopicID, topic.ID)
	}
}

func TestExecuteReminderBlockedUser(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("blocked", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)
	coreDB.BlockUser(user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := bobotTopic.ID
	schedDB.CreateReminder(user.ID, tid, "should not execute", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 0 {
		t.Errorf("expected 0 messages for blocked user, got %d", len(pipeline.calls))
	}

	// Should be marked as failed
	r, _ := schedDB.GetReminder(1)
	if r.Status != "failed" {
		t.Errorf("got status %q, want %q", r.Status, "failed")
	}
}

func TestExecuteReminderDeletedTopic(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")
	topic, _ := coreDB.CreateTopic("deleted-topic", user.ID)

	// Soft-delete the topic
	coreDB.SoftDeleteTopic(topic.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := topic.ID
	schedDB.CreateReminder(user.ID, tid, "should not execute", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 0 {
		t.Errorf("expected 0 messages for deleted topic, got %d", len(pipeline.calls))
	}

	r, _ := schedDB.GetReminder(1)
	if r.Status != "failed" {
		t.Errorf("got status %q, want %q", r.Status, "failed")
	}
}

func TestExecuteCronJob(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, err := coreDB.CreateBobotTopic(user.ID)
	if err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := bobotTopic.ID
	schedDB.CreateCronJob(user.ID, tid, "daily tasks", "summarize my tasks", "0 9 * * 1-5", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 1 {
		t.Fatalf("expected 1 message, got %d", len(pipeline.calls))
	}
	if pipeline.calls[0].Content != "<bobot-cron>summarize my tasks</bobot-cron>" {
		t.Errorf("got content %q", pipeline.calls[0].Content)
	}
	if pipeline.calls[0].TopicID != bobotTopic.ID {
		t.Errorf("got topic_id %d, want %d", pipeline.calls[0].TopicID, bobotTopic.ID)
	}
}

func TestExecuteCronJobBlockedUser(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("blocked", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)
	coreDB.BlockUser(user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := bobotTopic.ID
	schedDB.CreateCronJob(user.ID, tid, "job", "prompt", "0 9 * * *", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 0 {
		t.Errorf("expected 0 messages for blocked user, got %d", len(pipeline.calls))
	}

	// Cron job should be disabled
	j, _ := schedDB.GetCronJob(1)
	if j.Enabled {
		t.Error("expected cron job to be disabled for blocked user")
	}
}

func TestExecuteReminderFailed(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	tid := bobotTopic.ID
	schedDB.CreateReminder(user.ID, tid, "will fail", past)

	pipeline.failNext = true

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	r, _ := schedDB.GetReminder(1)
	if r.Status != "failed" {
		t.Errorf("got status %q, want %q", r.Status, "failed")
	}
}

func TestFutureItemsNotExecuted(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)

	future := time.Now().UTC().Add(1 * time.Hour)
	tid := bobotTopic.ID
	schedDB.CreateReminder(user.ID, tid, "future reminder", future)
	schedDB.CreateCronJob(user.ID, tid, "future job", "prompt", "0 9 * * *", future)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.calls) != 0 {
		t.Errorf("expected 0 messages for future items, got %d", len(pipeline.calls))
	}
}

func TestCoalescingMultipleMissedRuns(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")
	bobotTopic, _ := coreDB.CreateBobotTopic(user.ID)

	// Create a cron job that missed many runs (next_run_at far in the past)
	longPast := time.Now().UTC().Add(-24 * time.Hour)
	tid := bobotTopic.ID
	schedDB.CreateCronJob(user.ID, tid, "missed job", "prompt", "0 * * * *", longPast)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	// Should execute exactly once (coalescing)
	if len(pipeline.calls) != 1 {
		t.Errorf("expected 1 message (coalesced), got %d", len(pipeline.calls))
	}
}

func TestSchedulerContextCancellation(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		s.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Good, scheduler stopped
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop after context cancellation")
	}
}
