# bobot-web Design

A self-hosted AI assistant with a mobile-first chat interface for managing daily family tasks.

## Overview

bobot-web is a personal AI assistant accessed through a mobile-friendly web app. Users interact via natural language chat, and the assistant helps with daily tasks like managing grocery lists. The system is designed for a single user initially, with architecture supporting future multi-user sharing through groups.

## High-Level Architecture

```
┌─────────────────────────────────────────┐
│           Mobile Web UI                 │
│     (Go templates + vanilla JS/CSS)     │
└─────────────────┬───────────────────────┘
                  │ WebSocket
┌─────────────────▼───────────────────────┐
│            Go HTTP Server               │
│  - JWT Auth (access + refresh tokens)   │
│  - WebSocket chat handler               │
│  - Static assets                        │
└─────────────────┬───────────────────────┘
                  │
┌─────────────────▼───────────────────────┐
│          Assistant Engine               │
│  - Loads skills (markdown files)        │
│  - Builds system prompt                 │
│  - Manages conversation                 │
│  - Executes tool calls                  │
└─────────────────┬───────────────────────┘
                  │
        ┌─────────┴─────────┐
        ▼                   ▼
┌───────────────┐   ┌───────────────┐
│  LLM Provider │   │    SQLite     │
│   (z.ai)      │   │   Databases   │
└───────────────┘   └───────────────┘
```

## Technology Stack

- **Backend:** Go (no frontend build step)
- **Database:** SQLite (separate DBs for core and each tool)
- **Frontend:** Go templates + vanilla HTML/CSS/JS
- **Real-time:** WebSocket for chat
- **Auth:** JWT (short-lived access token + refresh token)
- **LLM:** Pluggable providers, starting with z.ai GLM-4.7 (Anthropic-compatible API)
- **Hosting:** Self-hosted
- **Config:** Environment variables

## Data Model

### Core Database (core.db)

```sql
-- Users
users (
  id INTEGER PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)

-- Refresh tokens (access tokens are stateless JWTs)
refresh_tokens (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  token TEXT UNIQUE NOT NULL,
  expires_at DATETIME NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)

-- Groups (for future multi-user sharing)
groups (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)

group_members (
  group_id INTEGER NOT NULL REFERENCES groups(id),
  user_id INTEGER NOT NULL REFERENCES users(id),
  role TEXT NOT NULL, -- 'owner', 'member'
  joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (group_id, user_id)
)

-- Messages (continuous streams)
messages (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  group_id INTEGER REFERENCES groups(id), -- NULL = private stream
  role TEXT NOT NULL, -- 'user', 'assistant', 'tool_result'
  content TEXT NOT NULL, -- JSON for structured content
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)
```

### Tool Database (tool_task.db)

```sql
projects (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  group_id INTEGER, -- NULL = private, set = shared
  name TEXT NOT NULL,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(user_id, name) -- name unique per user
)

tasks (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id),
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'done'
  metadata TEXT, -- JSON for flexible extra data
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
)
```

## Assistant Engine

### Skills

Skills are markdown files with YAML frontmatter that teach the LLM how to use tools for specific domains.

**Location:** `skills/*.md`

**Format:**

```yaml
---
name: groceries
description: Manage grocery shopping lists
---
When the user wants to manage their grocery list, use the `task` tool
with project name "groceries".

- "Add milk" → task(command="create", project="groceries", title="milk")
- "What do I need to buy?" → task(command="list", project="groceries", status="pending")
- "Got the eggs" → task(command="update", project="groceries", title="eggs", status="done")
```

### Tools

Tools are generic capabilities with a command-based interface. Each tool defines its schema using Go structs, auto-generated to JSON Schema via `github.com/invopop/jsonschema`.

**Task tool schema:**

```json
{
  "name": "task",
  "description": "Manage tasks within projects",
  "input_schema": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "enum": ["create", "list", "update", "delete"]
      },
      "project": {
        "type": "string",
        "description": "Project name (e.g., 'groceries')"
      },
      "title": {
        "type": "string",
        "description": "Task title"
      },
      "status": {
        "type": "string",
        "enum": ["pending", "done"]
      }
    },
    "required": ["command", "project"]
  }
}
```

### System Prompt Assembly

1. Load all skill markdown files from `skills/`
2. Build system prompt: base instructions + all skills + available tools schema
3. Send to LLM with conversation history (from messages table)

### Tool Execution Flow

1. LLM responds with tool call (e.g., `task` with command="create")
2. Engine validates and routes to appropriate tool
3. Tool executes against its database
4. Tool result returned to LLM
5. LLM generates final response to user

## LLM Provider Integration

### Configuration (Environment Variables)

```
BOBOT_LLM_BASE_URL=https://api.z.ai
BOBOT_LLM_API_KEY=your-api-key
BOBOT_LLM_MODEL=glm-4.7
```

### Provider Interface

```go
type LLMProvider interface {
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

type ChatRequest struct {
    SystemPrompt string
    Messages     []Message
    Tools        []ToolSchema
}

type ChatResponse struct {
    Content   string
    ToolCalls []ToolCall
    StepType  string // text, tool_use, etc.
}
```

The initial implementation targets Anthropic-compatible APIs (z.ai). Future providers implement the same interface.

## Web UI

### Layout

```
┌─────────────────────────────┐
│  bobot              [menu]  │  ← Header (sticky)
├─────────────────────────────┤
│                             │
│  ┌─────────────────────┐    │
│  │ User message        │    │  ← Right-aligned
│  └─────────────────────┘    │
│                             │
│  ┌─────────────────────┐    │
│  │ Assistant response  │    │  ← Left-aligned
│  └─────────────────────┘    │
│                             │
├─────────────────────────────┤
│  [Type a message...   ][➤]  │  ← Input (sticky bottom)
└─────────────────────────────┘
```

### WebSocket Protocol

Single persistent connection per session for bidirectional communication:
- Client sends user messages
- Server streams assistant responses
- Handles reconnection on network changes

### Rich Components

Messages can contain structured content rendered as components:

```json
{
  "role": "assistant",
  "content": [
    {"type": "text", "text": "Here's the weather:"},
    {"type": "component", "name": "weather", "data": {"temp": 72, "condition": "sunny"}}
  ]
}
```

Components are Go templates in `web/templates/components/`.

## Authentication

### JWT Token Pair

- **Access token:** Short-lived (15 minutes), stateless, contains user ID
- **Refresh token:** Longer-lived (7 days), stored in DB, used to obtain new access tokens

### Flow

1. `POST /login` - Validates credentials, returns access + refresh tokens
2. Access token used for WebSocket connection and API calls
3. `POST /refresh` - Exchanges valid refresh token for new access token
4. `POST /logout` - Deletes refresh token from DB

### Initial User Setup

Environment variables create first user on startup if none exist:

```
BOBOT_INIT_USER=admin
BOBOT_INIT_PASS=your-password
```

## Project Structure

```
bobot-web/
├── main.go                 # Entry point
├── config.go               # Env var loading
│
├── server/
│   ├── server.go           # HTTP server setup, routes
│   ├── auth.go             # Login, JWT, refresh
│   ├── chat.go             # WebSocket handler
│   └── middleware.go       # Auth middleware
│
├── assistant/
│   ├── engine.go           # Conversation orchestration
│   ├── prompt.go           # System prompt assembly
│   └── skills.go           # Skill file loading
│
├── llm/
│   ├── provider.go         # LLMProvider interface
│   └── anthropic.go        # Anthropic-compatible client
│
├── tools/
│   ├── registry.go         # Tool registration and dispatch
│   └── task/
│       ├── task.go         # Task tool implementation
│       └── db.go           # Task tool database
│
├── db/
│   └── core.go             # Core database operations
│
├── skills/
│   └── groceries.md        # Grocery skill
│
├── web/
│   ├── templates/
│   │   ├── layout.html
│   │   ├── login.html
│   │   ├── chat.html
│   │   └── components/
│   │       └── weather.html
│   └── static/
│       ├── style.css
│       └── chat.js
│
└── data/                   # SQLite databases (gitignored)
    ├── core.db
    └── tool_task.db
```

## Scope for v1

### Included

- Single user authentication (JWT)
- Chat-first interface (mobile-optimized)
- Task tool with CRUD operations
- Groceries skill
- z.ai LLM integration (Anthropic-compatible)
- Continuous message history
- Self-hosted deployment

### Designed but Deferred

- Groups and multi-user sharing
- Voice input/output
- Additional tools and skills
- Alternative LLM providers
- Config file support (YAML/JSON)

## Response Style

The assistant uses adaptive responses:
- **Terse** for simple, clear requests (e.g., "Added milk")
- **Conversational** when clarification is needed (e.g., "Added milk. Whole or skim?")
