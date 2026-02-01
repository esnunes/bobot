# HTMX PWA Navigation Design

## Problem

iOS Safari in standalone PWA mode has issues with HTTP redirects and `window.location.href` calls. They can open in the browser instead of staying in the PWA, or fail silently. All current navigation in bobot-web uses `window.location.href`, making the PWA unreliable on iOS.

## Solution

Use HTMX to handle navigation with a hybrid approach:
- **In-app navigation** (`/chat` ↔ `/groups` ↔ `/groups/{id}`): SPA-like content swapping via `hx-boost`
- **Auth flows** (login → chat, logout → home): Server-driven redirects via `HX-Redirect` header

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Auth redirect mechanism | Server-driven `HX-Redirect` header | Clean, server controls flow, HTMX handles PWA context |
| In-app navigation | `hx-boost` on body | Minimal refactoring, automatic link conversion |
| Content swapping | Swap `<body>` with `hx-preserve` | Less template refactoring than `<main>` swap |
| WebSocket handling | Preserve connection, event-based messaging | Single WebSocket already handles all message types |
| Form handling | Hybrid - boost auth forms, keep fetch for messages | Messages use WebSocket feedback, no need to change |

## Architecture

### HTMX Setup

Add to `layout.html`:
```html
<head>
  <script src="https://unpkg.com/htmx.org@2.0.0"></script>
</head>
<body hx-boost="true">
  <div id="ws-connection" hx-preserve="true"></div>
  <script src="/static/js/ws-manager.js"></script>
  <!-- page content -->
</body>
```

### WebSocket Manager (`ws-manager.js`)

Global script that:
1. Initializes WebSocket once, stores reference on `#ws-connection`
2. Dispatches custom DOM events on message receipt:
   - `bobot:chat-message` - direct chat messages
   - `bobot:group-message` - group messages (includes `group_id` in detail)
   - `bobot:connection-status` - open/close/error states
3. Handles reconnection with exponential backoff
4. Handles token refresh

Pages become listeners:
- `chat.js` listens for `bobot:chat-message`
- `group-chat.js` listens for `bobot:group-message`, filters by current group ID
- Pages clean up listeners on `htmx:beforeSwap`

### Auth Flows

**Login/Signup forms:**
```html
<form hx-post="/api/login" hx-swap="none"
      hx-on::after-request="handleAuthResponse(event)">
```

**Server responses:**
- Success: Return `HX-Redirect: /chat` header + tokens in body
- Failure: Return HTML error fragment

**Token storage:**
- `handleAuthResponse()` extracts tokens from response, stores in `localStorage`
- Redirect triggers after storage complete

**Logout:**
- `hx-post="/api/logout"` or `<a href="/api/logout">`
- Clear `localStorage` via `htmx:beforeRequest` event
- Server returns `HX-Redirect: /`

### Unauthorized Access

**Server-side:**
- Protected pages check token validity
- Invalid token: return `HX-Redirect: /` with 401 status

**Client-side:**
- Global listener for `htmx:responseError`
- On 401: clear `localStorage`, redirect to `/`
- On `bobot:auth-expired` event (from ws-manager): same handling

## File Changes

### New Files

| File | Purpose |
|------|---------|
| `web/static/js/ws-manager.js` | Global WebSocket manager with event dispatching |

### Modified Files

| File | Changes |
|------|---------|
| `web/templates/layout.html` | Add HTMX script, `hx-boost` on body, preserved container, ws-manager script |
| `server/server.go` | Auth endpoints return `HX-Redirect` when `HX-Request` header present |
| `web/static/js/chat.js` | Remove redirects, add event listeners, remove WebSocket init |
| `web/static/js/groups.js` | Remove redirects, add event listeners |
| `web/static/js/group-chat.js` | Remove redirects, add event listeners, remove WebSocket init |
| `web/static/js/login.js` | Convert to HTMX form handling |
| `web/static/js/signup.js` | Convert to HTMX form handling |

### Unchanged Files

- `web/static/manifest.json` - PWA config unchanged
- Individual page templates - content structure unchanged

## Server Response Examples

### Successful Login (HTMX request)
```http
HTTP/1.1 200 OK
HX-Redirect: /chat
Content-Type: application/json

{"access_token": "...", "refresh_token": "..."}
```

### Failed Login (HTMX request)
```http
HTTP/1.1 401 Unauthorized
Content-Type: text/html

<div class="error">Invalid credentials</div>
```

### Unauthorized Page Access (HTMX request)
```http
HTTP/1.1 401 Unauthorized
HX-Redirect: /
```

### Group Creation Success
```http
HTTP/1.1 201 Created
HX-Redirect: /groups/abc123
```

## Event Flow Diagrams

### Navigation Flow
```
User clicks <a href="/groups">
  → HTMX intercepts (hx-boost)
  → GET /groups via AJAX
  → Server returns full page HTML
  → HTMX swaps <body>, preserves #ws-connection
  → URL updates via pushState
  → groups.js initializes, listens for events
```

### Message Flow
```
WebSocket receives message
  → ws-manager parses message
  → Dispatches bobot:chat-message or bobot:group-message
  → Current page's listener receives event
  → Page renders message to UI
```

### Login Flow
```
User submits login form
  → HTMX POST /api/login
  → Server validates, returns HX-Redirect + tokens
  → hx-on::after-request fires
  → handleAuthResponse() stores tokens
  → HTMX follows HX-Redirect to /chat
  → chat.js initializes
  → ws-manager connects WebSocket with token
```
