# Reminders & Scheduled Prompts

**Date:** 2026-02-12
**Status:** Brainstorm

## What We're Building

Two new tools for bobot: **remind** (one-shot reminders) and **cron** (recurring scheduled prompts), backed by an in-process coalescing scheduler and a web UI for managing recurring prompts.

### Remind Tool (one-shot)
- User says "remind me to call the dentist at 3pm" or uses `/remind` slash command
- At the scheduled time, bobot sends the reminder as a chat message + push notification
- The message is injected into the chat as if the user typed it, so the LLM responds naturally
- Works in both private chat and topics

### Cron Tool (recurring)
- User says "every weekday at 9am, summarize my open tasks" or uses `/cron` slash command
- Uses cron expressions for scheduling (e.g., `0 9 * * 1-5`)
- At each trigger, the prompt is sent through the full LLM pipeline (with tools, context, skills)
- The LLM responds as if the user had typed and submitted the prompt at that time
- Works in both private chat and topics
- Web UI for listing, editing, and deleting recurring prompts

## Why This Approach

### Two separate tools (remind + cron) instead of one
- Clearer mental model for users and the LLM
- Different schemas: remind needs a time, cron needs a cron expression + prompt
- Different lifecycle: reminders are fire-and-forget, cron jobs are persistent

### In-process scheduler (goroutine + ticker)
- The server is already a long-running process — no need for external cron
- Simpler deployment (no extra systemd unit or crontab entry)
- Direct access to the chat engine, DB, and push sender

### Coalescing execution model
- Each execution creates a DB record for auditability
- **Catch-up**: if the system was down at scheduled time, the job runs on recovery
- **No accumulation**: multiple missed runs collapse into one catch-up execution
- **Long-running protection**: if execution takes longer than the interval, skip intermediate runs — compute `next_run_at` from actual completion time, not from the originally scheduled time
- Example: 5-min interval, execution takes 15 min → runs at T=0, then once at T=15 (not 3 times)

## Key Decisions

1. **Two tools**: `remind` for one-shot, `cron` for recurring
2. **Delivery**: chat message + push notification (same as regular bot responses)
3. **Execution model**: prompts run through the full LLM pipeline as if the user typed them
4. **Schedule format**: cron expressions (e.g., `0 9 * * 1-5` for weekdays at 9am)
5. **Scheduler**: in-process goroutine with ~1 minute tick interval
6. **Coalescing**: missed runs collapse, `next_run_at` computed from actual execution time
7. **Execution records**: every execution logged in DB with status, timestamps
8. **Scope**: works in both private chat and topics
9. **Creation UX**: LLM natural language + slash commands + web UI (web UI for cron only)
10. **Web UI**: list, edit (schedule + prompt text), enable/disable, delete recurring prompts at top-level `/schedules`
11. **Timezone**: per-user timezone in profile, default to server TZ
12. **Concurrency**: global serial — one scheduled execution at a time across all users
13. **Cron parsing**: minimal custom parser (no external library)
14. **Timeout**: 5 min default, configurable via `BOBOT_SCHEDULE_TIMEOUT` env var

## Data Model (Conceptual)

### reminders table
- `id`, `user_id`, `topic_id` (nullable), `message`, `run_at`, `status` (pending/executed/missed_then_executed), `executed_at`, `created_at`

### cron_jobs table
- `id`, `user_id`, `topic_id` (nullable), `prompt`, `cron_expr`, `enabled`, `next_run_at`, `created_at`, `updated_at`

### cron_executions table
- `id`, `cron_job_id`, `scheduled_at`, `started_at`, `completed_at`, `status` (running/completed/failed), `created_at`

## Resolved Questions

1. **Timezone handling**: user timezone stored in profile, default to server TZ if unset
2. **Concurrency**: global serial execution — only one scheduled prompt runs at a time across all users. Simple, prevents LLM overload.
3. **Cron expression parsing**: minimal custom parser — parse a subset of cron syntax (minute, hour, day-of-month, month, day-of-week). No external dependency.
4. **Execution timeout**: 5 minute default, configurable via `BOBOT_SCHEDULE_TIMEOUT` env var
5. **Web UI placement**: top-level `/schedules` page, accessible from main navigation

## Open Questions

None — ready for planning.
