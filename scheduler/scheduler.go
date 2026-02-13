package scheduler

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/esnunes/bobot/auth"
	"github.com/esnunes/bobot/db"
	"github.com/esnunes/bobot/tools/schedule"
)

// Pipeline is the interface for sending messages through the chat flow.
// This avoids importing the server package directly.
type Pipeline interface {
	SendPrivateMessage(ctx context.Context, userID int64, content string) (string, error)
	SendTopicMessage(ctx context.Context, userID int64, topicID int64, content string, displayName string) (string, error)
}

// Scheduler runs due reminders and cron jobs on a 1-minute tick.
type Scheduler struct {
	scheduleDB *schedule.ScheduleDB
	coreDB     *db.CoreDB
	pipeline   Pipeline
	timeout    time.Duration
	done       chan struct{}
}

// New creates a new Scheduler.
func New(scheduleDB *schedule.ScheduleDB, coreDB *db.CoreDB, pipeline Pipeline, timeout time.Duration) *Scheduler {
	return &Scheduler{
		scheduleDB: scheduleDB,
		coreDB:     coreDB,
		pipeline:   pipeline,
		timeout:    timeout,
		done:       make(chan struct{}),
	}
}

// Start begins the scheduler tick loop. It blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("scheduler: starting")

	// Run initial tick immediately to process any backlog
	s.tick(ctx)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler: stopping")
			close(s.done)
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// Done returns a channel that is closed when the scheduler has stopped.
func (s *Scheduler) Done() <-chan struct{} {
	return s.done
}

// dueItem represents either a reminder or cron job, sorted by scheduled time.
type dueItem struct {
	scheduledAt time.Time
	reminder    *schedule.Reminder
	cronJob     *schedule.CronJob
}

func (s *Scheduler) tick(ctx context.Context) {
	now := time.Now().UTC()

	reminders, err := s.scheduleDB.GetDueReminders(now)
	if err != nil {
		slog.Error("scheduler: failed to get due reminders", "error", err)
		return
	}

	cronJobs, err := s.scheduleDB.GetDueCronJobs(now)
	if err != nil {
		slog.Error("scheduler: failed to get due cron jobs", "error", err)
		return
	}

	if len(reminders) == 0 && len(cronJobs) == 0 {
		return
	}

	total := len(reminders) + len(cronJobs)
	if total > 10 {
		slog.Warn("scheduler: large backlog detected", "count", total)
	}

	// Merge and sort by scheduled time
	items := make([]dueItem, 0, total)
	for i := range reminders {
		items = append(items, dueItem{scheduledAt: reminders[i].RunAt, reminder: &reminders[i]})
	}
	for i := range cronJobs {
		items = append(items, dueItem{scheduledAt: cronJobs[i].NextRunAt, cronJob: &cronJobs[i]})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].scheduledAt.Before(items[j].scheduledAt)
	})

	slog.Info("scheduler: processing due items", "reminders", len(reminders), "cron_jobs", len(cronJobs))

	// Execute serially
	for _, item := range items {
		if ctx.Err() != nil {
			slog.Info("scheduler: context cancelled, stopping tick")
			return
		}
		if item.reminder != nil {
			s.executeReminder(ctx, item.reminder)
		} else {
			s.executeCronJob(ctx, item.cronJob)
		}
	}
}

func (s *Scheduler) executeReminder(ctx context.Context, r *schedule.Reminder) {
	slog.Info("scheduler: executing reminder", "id", r.ID, "user_id", r.UserID)

	// Lifecycle guard: check user is not blocked
	user, err := s.coreDB.GetUserByID(r.UserID)
	if err != nil || user.Blocked {
		slog.Warn("scheduler: skipping reminder for blocked/missing user", "id", r.ID, "user_id", r.UserID)
		s.scheduleDB.MarkReminderFailed(r.ID, "user blocked or not found")
		return
	}

	// Lifecycle guard: check topic still valid
	if r.TopicID != nil {
		if !s.isTopicValid(r.UserID, *r.TopicID) {
			slog.Warn("scheduler: skipping reminder for invalid topic", "id", r.ID, "topic_id", *r.TopicID)
			s.scheduleDB.MarkReminderFailed(r.ID, "topic deleted or user removed")
			return
		}
	}

	// Build execution context
	execCtx := auth.ContextWithUserData(ctx, auth.UserData{
		UserID: r.UserID,
		Role:   user.Role,
	})
	execCtx, cancel := context.WithTimeout(execCtx, s.timeout)
	defer cancel()

	content := "<bobot-remind>" + r.Message + "</bobot-remind>"

	var execErr error
	if r.TopicID != nil {
		execCtx = auth.ContextWithChatData(execCtx, auth.ChatData{TopicID: r.TopicID})
		_, execErr = s.pipeline.SendTopicMessage(execCtx, r.UserID, *r.TopicID, content, user.DisplayName)
	} else {
		_, execErr = s.pipeline.SendPrivateMessage(execCtx, r.UserID, content)
	}

	if execErr != nil {
		slog.Error("scheduler: reminder execution failed", "id", r.ID, "error", execErr)
		s.scheduleDB.MarkReminderFailed(r.ID, execErr.Error())
		return
	}

	s.scheduleDB.MarkReminderExecuted(r.ID, time.Now().UTC())
	slog.Info("scheduler: reminder executed", "id", r.ID)
}

func (s *Scheduler) executeCronJob(ctx context.Context, j *schedule.CronJob) {
	slog.Info("scheduler: executing cron job", "id", j.ID, "name", j.Name, "user_id", j.UserID)

	// Lifecycle guard: check user is not blocked
	user, err := s.coreDB.GetUserByID(j.UserID)
	if err != nil || user.Blocked {
		slog.Warn("scheduler: disabling cron job for blocked/missing user", "id", j.ID, "user_id", j.UserID)
		s.scheduleDB.DisableCronJob(j.ID)
		return
	}

	// Lifecycle guard: check topic still valid
	if j.TopicID != nil {
		if !s.isTopicValid(j.UserID, *j.TopicID) {
			slog.Warn("scheduler: disabling cron job for invalid topic", "id", j.ID, "topic_id", *j.TopicID)
			s.scheduleDB.DisableCronJob(j.ID)
			return
		}
	}

	// Create execution record
	startedAt := time.Now().UTC()
	execID, err := s.scheduleDB.CreateExecution(j.ID, j.NextRunAt, startedAt)
	if err != nil {
		slog.Error("scheduler: failed to create execution record", "id", j.ID, "error", err)
		return
	}

	// Build execution context
	execCtx := auth.ContextWithUserData(ctx, auth.UserData{
		UserID: j.UserID,
		Role:   user.Role,
	})
	execCtx, cancel := context.WithTimeout(execCtx, s.timeout)
	defer cancel()

	content := "<bobot-cron>" + j.Prompt + "</bobot-cron>"

	var execErr error
	if j.TopicID != nil {
		execCtx = auth.ContextWithChatData(execCtx, auth.ChatData{TopicID: j.TopicID})
		_, execErr = s.pipeline.SendTopicMessage(execCtx, j.UserID, *j.TopicID, content, user.DisplayName)
	} else {
		_, execErr = s.pipeline.SendPrivateMessage(execCtx, j.UserID, content)
	}

	completedAt := time.Now().UTC()
	if execErr != nil {
		slog.Error("scheduler: cron job execution failed", "id", j.ID, "error", execErr)
		s.scheduleDB.FailExecution(execID, completedAt, execErr.Error())
	} else {
		s.scheduleDB.CompleteExecution(execID, completedAt)
		slog.Info("scheduler: cron job executed", "id", j.ID)
	}

	// Update next_run_at from now (not from scheduled time) to prevent re-firing missed windows
	s.updateCronJobNextRun(j)
}

func (s *Scheduler) updateCronJobNextRun(j *schedule.CronJob) {
	expr, err := schedule.Parse(j.CronExpr)
	if err != nil {
		slog.Error("scheduler: invalid cron expression, disabling job", "id", j.ID, "expr", j.CronExpr, "error", err)
		s.scheduleDB.DisableCronJob(j.ID)
		return
	}

	// Compute next run from now (not from scheduled time) to skip missed intervals
	nextRun := expr.Next(time.Now().UTC())
	s.scheduleDB.UpdateCronJobNextRun(j.ID, nextRun)
	slog.Debug("scheduler: updated next run", "id", j.ID, "next_run_at", nextRun)
}

func (s *Scheduler) isTopicValid(userID, topicID int64) bool {
	topic, err := s.coreDB.GetTopicByID(topicID)
	if err != nil {
		return false
	}
	// Check soft-delete
	if topic.DeletedAt != nil {
		return false
	}
	// Check membership
	isMember, err := s.coreDB.IsTopicMember(topicID, userID)
	if err != nil || !isMember {
		return false
	}
	return true
}
