package schedule

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Reminder struct {
	ID         int64
	UserID     int64
	TopicID    *int64
	Message    string
	RunAt      time.Time
	Status     string // pending, executed, failed, cancelled
	ExecutedAt *time.Time
	Error      *string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type CronJob struct {
	ID        int64
	UserID    int64
	TopicID   *int64
	Name      string
	Prompt    string
	CronExpr  string
	Enabled   bool
	NextRunAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

type CronExecution struct {
	ID          int64
	CronJobID   int64
	ScheduledAt time.Time
	StartedAt   time.Time
	CompletedAt *time.Time
	Status      string // running, completed, failed
	Error       *string
	CreatedAt   time.Time
}

type ScheduleDB struct {
	db *sql.DB
}

func NewScheduleDB(dbPath string) (*ScheduleDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	sdb := &ScheduleDB{db: db}
	if err := sdb.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return sdb, nil
}

func (s *ScheduleDB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS reminders (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		topic_id INTEGER,
		message TEXT NOT NULL,
		run_at TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		executed_at TEXT,
		error TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_reminders_due ON reminders(status, run_at);

	CREATE TABLE IF NOT EXISTS cron_jobs (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL,
		topic_id INTEGER,
		name TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL,
		cron_expr TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		next_run_at TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
		updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	);

	CREATE INDEX IF NOT EXISTS idx_cron_jobs_due ON cron_jobs(enabled, next_run_at);

	CREATE TABLE IF NOT EXISTS cron_executions (
		id INTEGER PRIMARY KEY,
		cron_job_id INTEGER NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
		scheduled_at TEXT NOT NULL,
		started_at TEXT NOT NULL,
		completed_at TEXT,
		status TEXT NOT NULL DEFAULT 'running',
		error TEXT,
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *ScheduleDB) Close() error {
	return s.db.Close()
}

const timeFormat = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(timeFormat, s)
}

func parseNullableTime(s *string) *time.Time {
	if s == nil {
		return nil
	}
	t, err := parseTime(*s)
	if err != nil {
		return nil
	}
	return &t
}

// --- Reminders ---

func (s *ScheduleDB) CreateReminder(userID int64, topicID *int64, message string, runAt time.Time) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO reminders (user_id, topic_id, message, run_at) VALUES (?, ?, ?, ?)",
		userID, topicID, message, formatTime(runAt),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *ScheduleDB) GetReminder(id int64) (*Reminder, error) {
	var r Reminder
	var runAt, createdAt, updatedAt string
	var executedAt, errStr *string
	err := s.db.QueryRow(
		"SELECT id, user_id, topic_id, message, run_at, status, executed_at, error, created_at, updated_at FROM reminders WHERE id = ?",
		id,
	).Scan(&r.ID, &r.UserID, &r.TopicID, &r.Message, &runAt, &r.Status, &executedAt, &errStr, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.RunAt, _ = parseTime(runAt)
	r.CreatedAt, _ = parseTime(createdAt)
	r.UpdatedAt, _ = parseTime(updatedAt)
	r.ExecutedAt = parseNullableTime(executedAt)
	r.Error = errStr
	return &r, nil
}

func (s *ScheduleDB) ListPendingReminders(userID int64) ([]Reminder, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, message, run_at, status, executed_at, error, created_at, updated_at FROM reminders WHERE user_id = ? AND status = 'pending' ORDER BY run_at",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *ScheduleDB) ListPendingRemindersByTopic(userID, topicID int64) ([]Reminder, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, message, run_at, status, executed_at, error, created_at, updated_at FROM reminders WHERE user_id = ? AND topic_id = ? AND status = 'pending' ORDER BY run_at",
		userID, topicID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *ScheduleDB) CancelReminder(id, userID int64) error {
	result, err := s.db.Exec(
		"UPDATE reminders SET status = 'cancelled', updated_at = ? WHERE id = ? AND user_id = ? AND status = 'pending'",
		formatTime(time.Now()), id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ScheduleDB) GetDueReminders(now time.Time) ([]Reminder, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, message, run_at, status, executed_at, error, created_at, updated_at FROM reminders WHERE status = 'pending' AND run_at <= ? ORDER BY run_at",
		formatTime(now),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanReminders(rows)
}

func (s *ScheduleDB) MarkReminderExecuted(id int64, executedAt time.Time) error {
	_, err := s.db.Exec(
		"UPDATE reminders SET status = 'executed', executed_at = ?, updated_at = ? WHERE id = ? AND status = 'pending'",
		formatTime(executedAt), formatTime(executedAt), id,
	)
	return err
}

func (s *ScheduleDB) MarkReminderFailed(id int64, errMsg string) error {
	now := formatTime(time.Now())
	_, err := s.db.Exec(
		"UPDATE reminders SET status = 'failed', error = ?, updated_at = ? WHERE id = ? AND status = 'pending'",
		errMsg, now, id,
	)
	return err
}

// --- Cron Jobs ---

func (s *ScheduleDB) CreateCronJob(userID int64, topicID *int64, name, prompt, cronExpr string, nextRunAt time.Time) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO cron_jobs (user_id, topic_id, name, prompt, cron_expr, next_run_at) VALUES (?, ?, ?, ?, ?, ?)",
		userID, topicID, name, prompt, cronExpr, formatTime(nextRunAt),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *ScheduleDB) GetCronJob(id int64) (*CronJob, error) {
	var j CronJob
	var nextRunAt, createdAt, updatedAt string
	var enabled int
	err := s.db.QueryRow(
		"SELECT id, user_id, topic_id, name, prompt, cron_expr, enabled, next_run_at, created_at, updated_at FROM cron_jobs WHERE id = ?",
		id,
	).Scan(&j.ID, &j.UserID, &j.TopicID, &j.Name, &j.Prompt, &j.CronExpr, &enabled, &nextRunAt, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.Enabled = enabled == 1
	j.NextRunAt, _ = parseTime(nextRunAt)
	j.CreatedAt, _ = parseTime(createdAt)
	j.UpdatedAt, _ = parseTime(updatedAt)
	return &j, nil
}

func (s *ScheduleDB) ListCronJobs(userID int64) ([]CronJob, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, name, prompt, cron_expr, enabled, next_run_at, created_at, updated_at FROM cron_jobs WHERE user_id = ? ORDER BY created_at",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCronJobs(rows)
}

func (s *ScheduleDB) ListCronJobsByTopic(topicID int64) ([]CronJob, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, name, prompt, cron_expr, enabled, next_run_at, created_at, updated_at FROM cron_jobs WHERE topic_id = ? ORDER BY created_at",
		topicID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCronJobs(rows)
}

func (s *ScheduleDB) UpdateCronJob(id, userID int64, name, prompt, cronExpr string, enabled bool, nextRunAt time.Time) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	result, err := s.db.Exec(
		"UPDATE cron_jobs SET name = ?, prompt = ?, cron_expr = ?, enabled = ?, next_run_at = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		name, prompt, cronExpr, enabledInt, formatTime(nextRunAt), formatTime(time.Now()), id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ScheduleDB) DeleteCronJob(id, userID int64) error {
	result, err := s.db.Exec(
		"DELETE FROM cron_jobs WHERE id = ? AND user_id = ?",
		id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ScheduleDB) GetDueCronJobs(now time.Time) ([]CronJob, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, topic_id, name, prompt, cron_expr, enabled, next_run_at, created_at, updated_at FROM cron_jobs WHERE enabled = 1 AND next_run_at <= ? ORDER BY next_run_at",
		formatTime(now),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCronJobs(rows)
}

func (s *ScheduleDB) UpdateCronJobNextRun(id int64, nextRunAt time.Time) error {
	_, err := s.db.Exec(
		"UPDATE cron_jobs SET next_run_at = ?, updated_at = ? WHERE id = ?",
		formatTime(nextRunAt), formatTime(time.Now()), id,
	)
	return err
}

func (s *ScheduleDB) SetCronJobEnabled(id, userID int64, enabled bool) error {
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}
	result, err := s.db.Exec(
		"UPDATE cron_jobs SET enabled = ?, updated_at = ? WHERE id = ? AND user_id = ?",
		enabledInt, formatTime(time.Now()), id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *ScheduleDB) DisableCronJob(id int64) error {
	_, err := s.db.Exec(
		"UPDATE cron_jobs SET enabled = 0, updated_at = ? WHERE id = ?",
		formatTime(time.Now()), id,
	)
	return err
}


// --- Executions ---

func (s *ScheduleDB) CreateExecution(cronJobID int64, scheduledAt, startedAt time.Time) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO cron_executions (cron_job_id, scheduled_at, started_at) VALUES (?, ?, ?)",
		cronJobID, formatTime(scheduledAt), formatTime(startedAt),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *ScheduleDB) CompleteExecution(id int64, completedAt time.Time) error {
	_, err := s.db.Exec(
		"UPDATE cron_executions SET status = 'completed', completed_at = ? WHERE id = ?",
		formatTime(completedAt), id,
	)
	return err
}

func (s *ScheduleDB) FailExecution(id int64, completedAt time.Time, errMsg string) error {
	_, err := s.db.Exec(
		"UPDATE cron_executions SET status = 'failed', completed_at = ?, error = ? WHERE id = ?",
		formatTime(completedAt), errMsg, id,
	)
	return err
}

// --- Scan helpers ---

func scanReminders(rows *sql.Rows) ([]Reminder, error) {
	var reminders []Reminder
	for rows.Next() {
		var r Reminder
		var runAt, createdAt, updatedAt string
		var executedAt, errStr *string
		if err := rows.Scan(&r.ID, &r.UserID, &r.TopicID, &r.Message, &runAt, &r.Status, &executedAt, &errStr, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.RunAt, _ = parseTime(runAt)
		r.CreatedAt, _ = parseTime(createdAt)
		r.UpdatedAt, _ = parseTime(updatedAt)
		r.ExecutedAt = parseNullableTime(executedAt)
		r.Error = errStr
		reminders = append(reminders, r)
	}
	return reminders, rows.Err()
}

func scanCronJobs(rows *sql.Rows) ([]CronJob, error) {
	var jobs []CronJob
	for rows.Next() {
		var j CronJob
		var nextRunAt, createdAt, updatedAt string
		var enabled int
		if err := rows.Scan(&j.ID, &j.UserID, &j.TopicID, &j.Name, &j.Prompt, &j.CronExpr, &enabled, &nextRunAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		j.Enabled = enabled == 1
		j.NextRunAt, _ = parseTime(nextRunAt)
		j.CreatedAt, _ = parseTime(createdAt)
		j.UpdatedAt, _ = parseTime(updatedAt)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
