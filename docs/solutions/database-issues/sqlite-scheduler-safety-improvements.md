---
title: "SQLite Scheduler Safety Improvements"
date: 2026-02-13
category: database-issues
tags:
  - sqlite
  - concurrency
  - scheduler
  - wal-mode
  - status-guards
  - dead-code
  - structured-logging
severity: high
components:
  - tools/schedule/db.go
  - tools/schedule/cron.go
  - tools/schedule/cron_parser.go
  - server/pipeline.go
  - server/schedules.go
  - config/config.go
related_issues:
  - pr: "#16"
    description: "feat: reminders and scheduled prompts"
---

# SQLite Scheduler Safety Improvements

## Problem

A code review of the reminders/scheduled prompts feature (PR #16) identified five safety issues in the SQLite-backed scheduler:

1. **No WAL mode** -- The `ScheduleDB` was opened with only `foreign_keys(1)`. SQLite's default journal mode (DELETE) serializes writes, risking `SQLITE_BUSY` errors when the scheduler tick loop and web UI handlers access the database concurrently.

2. **No status guards on state transitions** -- `MarkReminderExecuted` and `MarkReminderFailed` updated rows by ID alone. If two scheduler ticks overlapped, the same reminder could be processed twice.

3. **Dead code** -- `MinInterval()` in `cron_parser.go` was never called anywhere in the codebase.

4. **Inconsistent logging** -- `server/pipeline.go` used `log.Printf` while the rest of the codebase used `log/slog` with structured fields.

5. **TOCTOU race** -- `CountEnabledCronJobs` followed by `CreateCronJob` was a check-then-act race. A concurrent request could insert between the count and the create.

## Root Cause

The ScheduleDB connection string lacked WAL mode and busy timeout pragmas. The UPDATE queries in mark methods had no WHERE clause on status, making them technically idempotent but not safe against the scheduler processing the same record twice within a tick cycle. The max-jobs limit used a count-then-act pattern that cannot be made atomic without database constraints.

## Solution

### 1. WAL mode + busy_timeout

```go
// Before
db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")

// After
db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
```

WAL mode allows concurrent reads while writes happen. `busy_timeout(5000)` makes transactions wait up to 5 seconds instead of failing immediately on contention.

### 2. Status guards on state transitions

```go
// Before
"UPDATE reminders SET status = 'executed', executed_at = ?, updated_at = ? WHERE id = ?"

// After
"UPDATE reminders SET status = 'executed', executed_at = ?, updated_at = ? WHERE id = ? AND status = 'pending'"
```

Same pattern applied to `MarkReminderFailed`. This ensures a reminder can only transition from `pending` once.

### 3. Remove dead code

Deleted `MinInterval()` from `cron_parser.go` and both `TestMinInterval` and `TestMinInterval_TooFrequent` from tests.

### 4. Structured logging

```go
// Before
log.Printf("assistant error: %v", err)

// After
slog.Error("pipeline: assistant error", "user_id", userID, "error", err)
```

All three `log.Printf` calls in `pipeline.go` replaced with `slog.Error` including contextual fields (`user_id`, `topic_id`, `error`).

### 5. Remove TOCTOU-prone max-jobs limit

Removed entirely: `CountEnabledCronJobs()`, `maxJobs` field on `CronTool`, `MaxCronJobs` config, and the check in `server/schedules.go`. Simpler and correct -- if limits are needed later, they should use database constraints.

## Prevention

### SQLite connections

- Always enable WAL mode and busy_timeout for any SQLite database accessed by multiple goroutines (scheduler + web server).
- Consider a shared helper for SQLite connection setup to enforce consistent pragmas.

### State transition guards

- Every UPDATE that changes status must include `AND status = '<expected>'` in the WHERE clause.
- Check `RowsAffected` after state transitions -- 0 rows means the record was not in the expected state.
- Write idempotency tests: call the same mark operation twice and verify the second is a no-op.

### Dead code

- Run `go vet ./...` and consider `golangci-lint` with `unused`/`deadcode` linters in CI.
- In code review, verify every new exported function has at least one call site.

### Logging consistency

- Use `log/slog` exclusively. Flag `log.Printf` in code reviews.
- Always include contextual fields (`user_id`, `topic_id`, `error`) for searchability.

### TOCTOU races

- Flag `SELECT COUNT(*)` before `INSERT` patterns in code review.
- Prefer database constraints (UNIQUE indexes, CHECK constraints) over application-level count checks.

## Related Documentation

- [Implementation plan](../../plans/2026-02-12-feat-reminders-and-scheduled-prompts-plan.md)
- [Brainstorm](../../brainstorms/2026-02-12-reminders-and-scheduled-prompts-brainstorm.md)
