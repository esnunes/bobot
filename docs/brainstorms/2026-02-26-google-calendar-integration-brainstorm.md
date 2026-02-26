# Google Calendar Integration - Brainstorm

**Date:** 2026-02-26
**Issue:** [esnunes/bobot#34](https://github.com/esnunes/bobot/issues/34)
**Status:** Draft

## What We're Building

A Google Calendar integration that lets topic members interact with a topic-owner's Google Calendar through bobot. Any topic owner can connect their Google Calendar to their topic via the settings page. Once connected, all topic members can add, update, delete, and list calendar events -- both through natural language conversation with bobot and via `/calendar` slash commands.

**Core user flow:**
1. Topic owner goes to topic settings, clicks "Connect Google Calendar"
2. OAuth consent screen opens, owner authorizes bobot
3. Owner picks which calendar to associate with the topic (people have multiple -- primary, work, holidays, etc.)
4. All topic members can now interact with the calendar via chat

## Why This Approach

**Hybrid architecture (Approach C)** was chosen because:

- Follows existing tool patterns exactly (`tools/calendar/` self-contained, like `tools/task/`, `tools/quickaction/`)
- Avoids premature abstraction -- no separate `google/` package until another Google integration is needed
- OAuth and Google API client code organized in separate files within the tool package (`oauth.go`, `google_client.go`) for easy future extraction
- Separate `tool_calendar.db` follows the established pattern of tool-specific databases

Rejected alternatives:
- *Shared Google package*: YAGNI -- no other Google integrations planned. Can extract later if needed.
- *Monolithic single file*: Would make the tool too large given OAuth, token management, and calendar API logic.

## Key Decisions

### 1. Any topic owner can connect their calendar
Each topic owner can connect their own Google Calendar to their topic. This is not admin-only. The Google OAuth app credentials (client ID/secret) are provided by the admin via environment variables (`BOBOT_GOOGLE_CLIENT_ID`, `BOBOT_GOOGLE_CLIENT_SECRET`), but individual topic owners authorize their own Google accounts.

### 2. One calendar per topic, user picks which one
After OAuth authorization, the settings page fetches the user's calendar list and lets them pick which specific calendar to associate with the topic. Only one calendar per topic -- keeps things simple.

### 3. Full access for all topic members
All topic members can add, update, delete, and list events. Same permission model as other topic features -- the topic is a shared space.

### 4. Settings page for OAuth connection
The "Connect Google Calendar" flow lives in topic settings (alongside skills, schedules, quick actions). No chat-based OAuth -- the settings page handles the redirect flow cleanly.

### 5. LLM tool + slash command
A `/calendar` tool with subcommands: `list`, `add`, `update`, `delete`. Available both as an LLM tool (bobot uses it during conversations) and as a slash command users can invoke directly.

### 6. Context-dependent display
Event lists are plain text (clean, scannable). Individual event details include interactive bobot tags for quick edit/delete actions.

### 7. Separate tool database
OAuth tokens and calendar associations stored in `tool_calendar.db`, following the `tool_task.db` / `tool_schedule.db` pattern.

### 8. Well-organized files for future extraction
Code within `tools/calendar/` is split across files:
- `calendar.go` -- Tool interface implementation, subcommand routing
- `oauth.go` -- Google OAuth flow (token exchange, refresh)
- `google_client.go` -- Google Calendar API client
- `db.go` -- Database schema, token/calendar storage queries

## Data Model

```sql
-- OAuth tokens per user (a user might connect calendars to multiple topics)
CREATE TABLE IF NOT EXISTS google_tokens (
    user_id INTEGER NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id)
);

-- Calendar association per topic
CREATE TABLE IF NOT EXISTS topic_calendars (
    topic_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,        -- who connected it
    calendar_id TEXT NOT NULL,        -- Google Calendar ID
    calendar_name TEXT NOT NULL,      -- display name
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (topic_id)
);
```

## Settings UI Flow

1. Topic settings page shows a "Google Calendar" section
2. If not connected: "Connect Google Calendar" button
3. Button redirects to Google OAuth consent screen
4. Google redirects back to `/api/google/callback` with auth code
5. Server exchanges code for tokens, stores in `google_tokens`
6. Server fetches user's calendar list, redirects to a calendar picker page
7. User selects a calendar, association saved in `topic_calendars`
8. Settings page now shows connected calendar name with a "Disconnect" option

## Tool Subcommands

- `list [date_or_range]` -- List events for today, a specific date, or a date range (e.g., "this week")
- `add <title> <start> <end> [description]` -- Create a new event
- `update <event_id> [title] [start] [end] [description]` -- Update an existing event
- `delete <event_id>` -- Delete an event

## Resolved Questions

1. **Token refresh strategy:** Reactive -- refresh on 401 response. Simpler, no background jobs. Minor latency only on the first expired call.
2. **Timezone handling:** Use the topic owner's Google Calendar timezone setting. The calendar API returns this, so we use it for displaying and creating events.
3. **Event ID visibility:** Title-based references. The LLM references events by title and date in conversation, which is more natural. The tool handles Google event ID resolution internally (e.g., list events, find matching title, operate on it).
4. **Disconnect behavior:** Leave old messages as-is. Action buttons become non-functional but no cleanup needed. Simple and non-destructive.
5. **Google API quotas:** Handle 429 responses gracefully -- catch rate limit errors and return a friendly message. No client-side throttling.
