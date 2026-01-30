# Admin and User Roles Design

## Overview

Bobot users can be regular users or admin users. Admin users can invite users, block and unblock users. All admin functionality is accessible via a `user` tool that works through natural language (LLM) or slash commands.

## Roles

- **admin** — can use the user tool (invite, block, unblock, list, invites, revoke)
- **user** — regular user, no admin capabilities
- First user created via `BOBOT_INIT_USER` environment variable is always admin
- Admins can only invite regular users; promoting to admin requires direct database access

## Data Model

### User table changes

Add three new columns to the existing `users` table:

```sql
users (
  id INTEGER PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  display_name TEXT NOT NULL,           -- NEW
  role TEXT NOT NULL DEFAULT 'user',    -- NEW: 'admin' or 'user'
  blocked INTEGER NOT NULL DEFAULT 0,   -- NEW: 0=active, 1=blocked
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)
```

### New invites table

```sql
invites (
  id INTEGER PRIMARY KEY,
  code TEXT UNIQUE NOT NULL,            -- random token for signup URL
  created_by INTEGER NOT NULL REFERENCES users(id),
  used_by INTEGER REFERENCES users(id), -- NULL if pending, user_id if used
  used_at DATETIME,                     -- NULL if pending
  revoked INTEGER NOT NULL DEFAULT 0,   -- 0=active, 1=revoked
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)
```

Invite states:
- **Pending**: `used_by IS NULL AND revoked = 0`
- **Used**: `used_by IS NOT NULL`
- **Revoked**: `revoked = 1`

Invites do not expire. They remain valid until used or revoked by an admin.

## Authentication & Authorization

### JWT Claims

Include role in JWT claims:

```go
type Claims struct {
    UserID int64  `json:"user_id"`
    Role   string `json:"role"`    // "admin" or "user"
    jwt.RegisteredClaims
}
```

### Blocking (soft approach)

When an admin blocks a user:

1. Set `blocked = 1` on the user record
2. Delete all refresh tokens for that user
3. Access token continues to work until expiry (max 15 minutes)
4. When token expires and user tries to refresh, it fails
5. Login attempts also check `blocked` status and reject

Blocked status is checked on:
- Login (`/api/auth/login`)
- Token refresh (`/api/auth/refresh`)

No middleware changes needed. This preserves JWT's stateless design.

### User data when blocked

User data (messages, tasks) is preserved when blocked. If unblocked, everything is restored.

## User Tool

### Tool input

```go
type UserToolInput struct {
    Command  string `json:"command"`            // invite, block, unblock, list, invites, revoke
    Username string `json:"username,omitempty"` // for block, unblock
    Code     string `json:"code,omitempty"`     // for revoke
}
```

### Subcommands

| Command | Input | Output |
|---------|-------|--------|
| `invite` | — | Signup URL with code, e.g. `https://host/signup?code=abc123` |
| `block <username>` | `username` | Confirmation message |
| `unblock <username>` | `username` | Confirmation message |
| `list` | — | Table of users: username, display name, role, status, created |
| `invites` | — | Table of pending invites: code, created by, created at |
| `revoke <code>` | `code` | Confirmation message |

### Error handling

- Non-admin tries to use tool → "This command requires admin privileges"
- Block/unblock non-existent user → "User not found"
- Block yourself → "Cannot block yourself"
- Revoke already-used or non-existent invite → "Invite not found or already used"

### Slash commands

- `/user invite` → `{command: "invite"}`
- `/user block alice` → `{command: "block", username: "alice"}`
- `/user unblock alice` → `{command: "unblock", username: "alice"}`
- `/user list` → `{command: "list"}`
- `/user invites` → `{command: "invites"}`
- `/user revoke abc123` → `{command: "revoke", code: "abc123"}`

### LLM integration

The tool is registered with the assistant engine for natural language use:

- "Invite a new user" → LLM calls `{command: "invite"}`
- "Block the user alice" → LLM calls `{command: "block", username: "alice"}`
- "Show me all users" → LLM calls `{command: "list"}`
- "Who hasn't accepted their invite yet?" → LLM calls `{command: "invites"}`

The tool schema is only included in the system prompt when `role = "admin"`. Regular users don't see it, reducing their context size.

## Signup Flow

### Endpoints

- `GET /signup?code=<code>` — signup page
- `POST /api/auth/signup` — create account

### Page behavior

1. Validate invite code
   - Invalid, used, or revoked → show error "Invalid or expired invite"
   - Valid → show signup form

2. Form fields:
   - Username
   - Display name
   - Password
   - Confirm password

### Signup request

```json
{
  "code": "abc123",
  "username": "alice",
  "display_name": "Alice Smith",
  "password": "secret"
}
```

### Signup logic

1. Validate invite code (exists, not used, not revoked)
2. Validate username is unique
3. Create user with `role = "user"`, `blocked = 0`
4. Mark invite as used (`used_by = user.id`, `used_at = now`)
5. Return JWT tokens (user is logged in immediately)
6. Redirect to chat page

## Validation Rules

### Username
- Minimum 3 characters
- Alphanumeric and underscores only (`^[a-zA-Z0-9_]+$`)
- Case-insensitive uniqueness (store lowercase, compare lowercase)

### Password
- Minimum 8 characters
- No other complexity requirements

### Display name
- Minimum 1 character
- Any characters allowed

## Files to Modify

- `db/core.go` — schema migrations, user queries, invite queries
- `auth/jwt.go` — add role to claims
- `server/auth.go` — signup endpoint, blocked checks on login/refresh
- `tools/` — new user tool
- `web/templates/` — new signup page
- `assistant/` — conditional tool loading based on role
