# bobot

A self-hosted personal AI assistant with multi-user support, topic-based conversations, task management, scheduling, and push notifications — delivered as a PWA.

## Features

- **LLM-powered chat** — conversations backed by Anthropic Claude (or any compatible API)
- **Topics** — organize conversations into named topics with member management
- **Tasks** — built-in task/project management via natural language
- **Skills** — custom per-chat skills that shape assistant behavior
- **Scheduling** — one-shot reminders and recurring cron-based prompts
- **Push notifications** — Web Push (VAPID) for real-time alerts
- **Smart home** — optional LG ThinQ device control
- **Web search** — optional Brave Search integration
- **User profiles** — auto-generated user profiles from conversation history
- **Admin dashboard** — inspect full LLM context for any user or topic chat
- **PWA** — installable progressive web app with WebSocket live updates
- **Minimal dependencies** — only 6 direct dependencies to reduce supply chain attack surface

## Requirements

- Go 1.23+
- An LLM API key (Anthropic or compatible provider)

## Getting started

```bash
# Install
go install github.com/esnunes/bobot@latest

# Create the first admin user
bobot create-admin <username>

# Configure (see Configuration below)
export BOBOT_LLM_BASE_URL=https://api.anthropic.com
export BOBOT_LLM_API_KEY=sk-ant-...
export BOBOT_LLM_MODEL=claude-sonnet-4-5-20250929
export BOBOT_JWT_SECRET=$(openssl rand -hex 32)

# Run
bobot
```

The server starts on `http://localhost:8080` by default.

## Configuration

All configuration is done through environment variables.

### Required

| Variable | Description |
|---|---|
| `BOBOT_LLM_BASE_URL` | LLM API base URL (e.g. `https://api.anthropic.com`) |
| `BOBOT_LLM_API_KEY` | LLM API key |
| `BOBOT_LLM_MODEL` | LLM model identifier |
| `BOBOT_JWT_SECRET` | Secret for signing JWT tokens |

### Optional

| Variable | Default | Description |
|---|---|---|
| `BOBOT_HOST` | `0.0.0.0` | Server bind address |
| `BOBOT_PORT` | `8080` | Server port |
| `BOBOT_BASE_URL` | `http://localhost:8080` | Public-facing base URL |
| `BOBOT_DATA_DIR` | `./data` | Directory for SQLite databases |
| `BOBOT_SESSION_DURATION` | `30m` | Session token lifetime |
| `BOBOT_SESSION_MAX_AGE` | `168h` | Maximum session age |
| `BOBOT_SESSION_REFRESH_THRESHOLD` | `5m` | Session refresh window |
| `BOBOT_CONTEXT_TOKENS_START` | `30000` | Context window start size (tokens) |
| `BOBOT_CONTEXT_TOKENS_MAX` | `80000` | Maximum context window (tokens) |
| `BOBOT_HISTORY_DEFAULT_LIMIT` | `50` | Default message history limit |
| `BOBOT_HISTORY_MAX_LIMIT` | `100` | Maximum message history limit |
| `BOBOT_SYNC_MAX_LOOKBACK` | `24h` | Max lookback for message sync |
| `BOBOT_SCHEDULE_TIMEOUT` | `5m` | Timeout for scheduled prompt execution |
| `LOG_LEVEL` | `info` | Log level (`debug`, `info`, `warn`, `error`) |

### Push notifications (optional)

Generate keys with `bobot generate-vapid-keys`.

| Variable | Description |
|---|---|
| `BOBOT_VAPID_PUBLIC_KEY` | VAPID public key (base64url) |
| `BOBOT_VAPID_PRIVATE_KEY` | VAPID private key (base64url) |
| `BOBOT_VAPID_SUBJECT` | VAPID subject (`mailto:` or `https:` URL) |

### Integrations (optional)

| Variable | Description |
|---|---|
| `BRAVE_SEARCH_API_KEY` | Brave Search API key for web search tool |
| `BOBOT_GOOGLE_CLIENT_ID` | Google OAuth2 client ID for Calendar integration |
| `BOBOT_GOOGLE_CLIENT_SECRET` | Google OAuth2 client secret for Calendar integration |
| `THINQ_TOKEN` | LG ThinQ API token |
| `THINQ_COUNTRY` | ThinQ country code |
| `THINQ_CLIENT_ID` | ThinQ client ID |

#### Google Calendar setup

To enable the Google Calendar tool, create OAuth2 credentials in the Google Cloud Console:

1. Go to [Google Cloud Console](https://console.cloud.google.com/) and create a project (or select an existing one).
2. Enable the **Google Calendar API** under **APIs & Services > Library**.
3. Configure the **OAuth consent screen** under **APIs & Services > OAuth consent screen**:
   - Choose **External** user type (or **Internal** if using Google Workspace).
   - Fill in the app name and support email.
   - Add the scopes `https://www.googleapis.com/auth/calendar.events` and `https://www.googleapis.com/auth/calendar.calendarlist.readonly`.
4. Create credentials under **APIs & Services > Credentials > Create Credentials > OAuth client ID**:
   - Application type: **Web application**.
   - Add `<your-base-url>/api/calendar/callback` as an **Authorized redirect URI** (e.g. `http://localhost:8080/api/calendar/callback`).
5. Copy the **Client ID** and **Client Secret** into `BOBOT_GOOGLE_CLIENT_ID` and `BOBOT_GOOGLE_CLIENT_SECRET`.

Once configured, topic owners can connect a Google Calendar from the topic settings page.

## CLI commands

```bash
# Start the server
bobot

# Create an admin user
bobot create-admin <username>

# Generate VAPID keys for push notifications
bobot generate-vapid-keys

# Update user profiles from conversation history
bobot update-profiles
```

### Makefile targets

| Target | Description |
|---|---|
| `make run` | Run with `go run .` |
| `make test` | Run all tests |
| `make build` | Build for current OS + linux/arm64 |
| `make deploy` | Build and deploy to remote server via SSH |
| `make logs` | Tail remote server logs |

## Built-in tools

The assistant has access to these tools during conversations:

| Tool | Description |
|---|---|
| `task` | Manage tasks within projects |
| `topic` | Create, delete, and manage conversation topics and members |
| `user` | Invite, block/unblock, and list users (admin only) |
| `skill` | Create, update, delete, and list custom per-chat skills |
| `remind` | Create one-shot reminders that fire at a specific time |
| `cron` | Manage recurring scheduled prompts on a cron schedule |
| `web_search` | Search the web via Brave Search (when configured) |
| `calendar` | Manage Google Calendar events (when configured) |
| `thinq` | Control LG ThinQ smart home devices (when configured) |

## Ideas

- add support for tuya devices
- add a weather tool
- add a web_fetch tool
- create a alexa skill to send messages to bobot
- improve overall logging
- cover solution with web tests
- use QMD as bobot's memory
