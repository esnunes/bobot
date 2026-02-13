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

// mockPipeline records calls to SendPrivateMessage and SendTopicMessage.
type mockPipeline struct {
	mu       sync.Mutex
	private  []privateCall
	topic    []topicCall
	failNext bool
}

type privateCall struct {
	UserID  int64
	Content string
}

type topicCall struct {
	UserID      int64
	TopicID     int64
	Content     string
	DisplayName string
}

func (m *mockPipeline) SendPrivateMessage(ctx context.Context, userID int64, content string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.private = append(m.private, privateCall{UserID: userID, Content: content})
	if m.failNext {
		m.failNext = false
		return "", fmt.Errorf("mock error")
	}
	return "ok", nil
}

func (m *mockPipeline) SendTopicMessage(ctx context.Context, userID int64, topicID int64, content string, displayName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.topic = append(m.topic, topicCall{UserID: userID, TopicID: topicID, Content: content, DisplayName: displayName})
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

	// Create a test user
	user, err := coreDB.CreateUser("testuser", "hash")
	if err != nil {
		t.Fatal(err)
	}

	// Create a due reminder
	past := time.Now().UTC().Add(-5 * time.Minute)
	schedDB.CreateReminder(user.ID, nil, "call dentist", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.private) != 1 {
		t.Fatalf("expected 1 private message, got %d", len(pipeline.private))
	}
	if pipeline.private[0].Content != "[Reminder] call dentist" {
		t.Errorf("got content %q, want %q", pipeline.private[0].Content, "[Reminder] call dentist")
	}
	if pipeline.private[0].UserID != user.ID {
		t.Errorf("got user_id %d, want %d", pipeline.private[0].UserID, user.ID)
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
	schedDB.CreateReminder(user.ID, &tid, "topic reminder", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.topic) != 1 {
		t.Fatalf("expected 1 topic message, got %d", len(pipeline.topic))
	}
	if pipeline.topic[0].Content != "[Reminder] topic reminder" {
		t.Errorf("got content %q", pipeline.topic[0].Content)
	}
	if pipeline.topic[0].TopicID != topic.ID {
		t.Errorf("got topic_id %d, want %d", pipeline.topic[0].TopicID, topic.ID)
	}
}

func TestExecuteReminderBlockedUser(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("blocked", "hash")
	coreDB.BlockUser(user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	schedDB.CreateReminder(user.ID, nil, "should not execute", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.private) != 0 {
		t.Errorf("expected 0 messages for blocked user, got %d", len(pipeline.private))
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
	schedDB.CreateReminder(user.ID, &tid, "should not execute", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.topic) != 0 {
		t.Errorf("expected 0 messages for deleted topic, got %d", len(pipeline.topic))
	}

	r, _ := schedDB.GetReminder(1)
	if r.Status != "failed" {
		t.Errorf("got status %q, want %q", r.Status, "failed")
	}
}

func TestExecuteCronJob(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")

	past := time.Now().UTC().Add(-5 * time.Minute)
	schedDB.CreateCronJob(user.ID, nil, "daily tasks", "summarize my tasks", "0 9 * * 1-5", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.private) != 1 {
		t.Fatalf("expected 1 private message, got %d", len(pipeline.private))
	}
	if pipeline.private[0].Content != "[Scheduled] summarize my tasks" {
		t.Errorf("got content %q", pipeline.private[0].Content)
	}
}

func TestExecuteCronJobBlockedUser(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("blocked", "hash")
	coreDB.BlockUser(user.ID)

	past := time.Now().UTC().Add(-5 * time.Minute)
	schedDB.CreateCronJob(user.ID, nil, "job", "prompt", "0 9 * * *", past)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.private) != 0 {
		t.Errorf("expected 0 messages for blocked user, got %d", len(pipeline.private))
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

	past := time.Now().UTC().Add(-5 * time.Minute)
	schedDB.CreateReminder(user.ID, nil, "will fail", past)

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

	future := time.Now().UTC().Add(1 * time.Hour)
	schedDB.CreateReminder(user.ID, nil, "future reminder", future)
	schedDB.CreateCronJob(user.ID, nil, "future job", "prompt", "0 9 * * *", future)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	if len(pipeline.private) != 0 {
		t.Errorf("expected 0 messages for future items, got %d", len(pipeline.private))
	}
}

func TestCoalescingMultipleMissedRuns(t *testing.T) {
	schedDB, coreDB, pipeline := setupTest(t)

	user, _ := coreDB.CreateUser("testuser", "hash")

	// Create a cron job that missed many runs (next_run_at far in the past)
	longPast := time.Now().UTC().Add(-24 * time.Hour)
	schedDB.CreateCronJob(user.ID, nil, "missed job", "prompt", "0 * * * *", longPast)

	s := New(schedDB, coreDB, pipeline, 5*time.Minute)
	s.tick(context.Background())

	// Should execute exactly once (coalescing)
	if len(pipeline.private) != 1 {
		t.Errorf("expected 1 message (coalesced), got %d", len(pipeline.private))
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
