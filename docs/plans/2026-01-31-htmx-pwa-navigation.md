# HTMX PWA Navigation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix iOS PWA redirect issues by using HTMX for navigation with content swapping and server-driven auth redirects.

**Architecture:** HTMX `hx-boost` on body for SPA-like navigation, `hx-preserve` for WebSocket connection, server returns `HX-Redirect` headers for auth flows, event-based WebSocket message dispatching.

**Tech Stack:** HTMX 2.x, Go net/http, vanilla JavaScript

---

## Task 1: Add HTMX to Layout and Configure hx-boost

**Files:**
- Modify: `web/templates/layout.html`

**Step 1: Add HTMX script and configure body**

Edit `web/templates/layout.html`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="apple-mobile-web-app-title" content="Bobot">
    <link rel="apple-touch-icon" href="/static/icon-512x512.png" sizes="512x512">
    <meta name="format-detection" content="telephone=no">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no, viewport-fit=cover">
    <title>{{.Title}} - bobot</title>
    <link rel="stylesheet" href="/static/style.css">
    <link ref="manifest" href="/static/manifest.json">
    <script src="https://unpkg.com/htmx.org@2.0.4"></script>
</head>
<body hx-boost="true">
    <div id="ws-connection" hx-preserve="true"></div>
    <script src="/static/ws-manager.js"></script>
    {{template "content" .}}
</body>
</html>
```

**Step 2: Verify page loads without errors**

Run: `go run . &` and open http://localhost:8080 in browser
Expected: Page loads, HTMX script present in DevTools Network tab

**Step 3: Commit**

```bash
git add web/templates/layout.html
git commit -m "feat: add HTMX to layout with hx-boost and ws-connection container"
```

---

## Task 2: Create WebSocket Manager with Event-Based Messaging

**Files:**
- Create: `web/static/ws-manager.js`

**Step 1: Create ws-manager.js**

Create `web/static/ws-manager.js`:

```javascript
// WebSocket manager - handles connection lifecycle and event dispatching
(function() {
    const container = document.getElementById('ws-connection');
    if (!container) return;

    // Prevent re-initialization on HTMX swaps
    if (container.dataset.initialized === 'true') return;
    container.dataset.initialized = 'true';

    let ws = null;
    let reconnectAttempts = 0;
    const MAX_RECONNECT_DELAY = 30000;

    function getToken() {
        return localStorage.getItem('access_token');
    }

    function connect() {
        const token = getToken();
        if (!token) {
            dispatchStatus('disconnected', 'no token');
            return;
        }

        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        ws = new WebSocket(wsUrl);
        container._ws = ws;

        ws.onopen = () => {
            console.log('WebSocket connected');
            reconnectAttempts = 0;
            dispatchStatus('connected');
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            dispatchMessage(data);
        };

        ws.onclose = () => {
            console.log('WebSocket disconnected');
            dispatchStatus('disconnected');
            scheduleReconnect();
        };

        ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            dispatchStatus('error', error);
        };
    }

    function dispatchStatus(status, detail = null) {
        document.dispatchEvent(new CustomEvent('bobot:connection-status', {
            detail: { status, detail }
        }));
    }

    function dispatchMessage(data) {
        if (data.group_id) {
            document.dispatchEvent(new CustomEvent('bobot:group-message', {
                detail: data
            }));
        } else {
            document.dispatchEvent(new CustomEvent('bobot:chat-message', {
                detail: data
            }));
        }
    }

    function scheduleReconnect() {
        const delay = Math.min(1000 * Math.pow(2, reconnectAttempts), MAX_RECONNECT_DELAY);
        reconnectAttempts++;
        console.log(`Reconnecting in ${delay}ms...`);
        setTimeout(() => refreshAndReconnect(), delay);
    }

    async function refreshAndReconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            dispatchStatus('auth-expired');
            document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });

            if (!resp.ok) {
                throw new Error('Refresh failed');
            }

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);
            connect();
        } catch (err) {
            console.error('Token refresh failed:', err);
            dispatchStatus('auth-expired');
            document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
        }
    }

    // Public API via container element
    container.send = function(message) {
        if (ws && ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify(message));
            return true;
        }
        return false;
    };

    container.close = function() {
        if (ws) {
            ws.close();
            ws = null;
        }
    };

    container.reconnect = function() {
        if (ws) ws.close();
        reconnectAttempts = 0;
        connect();
    };

    // Handle auth expiration globally
    document.addEventListener('bobot:auth-expired', () => {
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');
        window.location.href = '/';
    });

    // Initialize connection if we have a token
    if (getToken()) {
        connect();
    }

    // Reconnect when token changes (e.g., after login)
    document.addEventListener('bobot:token-updated', () => {
        container.reconnect();
    });
})();
```

**Step 2: Verify file is served**

Run server, open browser DevTools, check Network tab for ws-manager.js loading
Expected: File loads with 200 status

**Step 3: Commit**

```bash
git add web/static/ws-manager.js
git commit -m "feat: add WebSocket manager with event-based messaging"
```

---

## Task 3: Update Server Auth Endpoints for HTMX Redirects

**Files:**
- Modify: `server/auth.go`

**Step 1: Add helper function to detect HTMX requests**

Add at top of auth.go after imports:

```go
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
```

**Step 2: Update handleLogin for HTMX redirect**

Replace handleLogin function:

```go
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := s.db.GetUserByUsername(req.Username)
	if err == db.ErrNotFound {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	// Check if user is blocked
	if user.Blocked {
		http.Error(w, "account blocked", http.StatusForbidden)
		return
	}

	accessToken, err := s.jwt.GenerateAccessTokenWithRole(user.ID, user.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	refreshToken := s.jwt.GenerateRefreshToken()
	expiresAt := time.Now().Add(s.jwt.RefreshTTL())

	_, err = s.db.CreateRefreshToken(user.ID, refreshToken, expiresAt)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/chat")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}
```

**Step 3: Update handleSignup for HTMX redirect**

In handleSignup, add before the final json.NewEncoder line:

```go
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/chat")
	}
```

**Step 4: Update handleLogout for HTMX redirect**

Replace handleLogout function:

```go
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	s.db.DeleteRefreshToken(req.RefreshToken)

	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/")
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

**Step 5: Run tests**

Run: `go test ./server/...`
Expected: All tests pass

**Step 6: Commit**

```bash
git add server/auth.go
git commit -m "feat: add HX-Redirect headers to auth endpoints"
```

---

## Task 4: Update Group Creation for HTMX Redirect

**Files:**
- Modify: `server/groups.go`

**Step 1: Add isHTMXRequest helper if not already present**

Add at top after imports (if not importing from auth.go):

```go
func isHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
```

**Step 2: Update handleCreateGroup for HTMX redirect**

In handleCreateGroup, replace the success response section:

```go
	if isHTMXRequest(r) {
		w.Header().Set("HX-Redirect", "/groups/"+group.ID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       group.ID,
		"name":     group.Name,
		"owner_id": group.OwnerID,
	})
```

**Step 3: Run tests**

Run: `go test ./server/...`
Expected: All tests pass

**Step 4: Commit**

```bash
git add server/groups.go
git commit -m "feat: add HX-Redirect to group creation endpoint"
```

---

## Task 5: Refactor Login Page for HTMX

**Files:**
- Modify: `web/templates/login.html`

**Step 1: Update login form for HTMX**

Replace `web/templates/login.html`:

```html
{{define "content"}}
<div class="login-container">
    <h1>bobot</h1>
    <form id="login-form" class="login-form"
          hx-post="/api/login"
          hx-swap="none"
          hx-on::after-request="handleLoginResponse(event)">
        <input type="text" name="username" placeholder="Username" required autofocus>
        <input type="password" name="password" placeholder="Password" required>
        <button type="submit">Login</button>
        <p id="error-message" class="error hidden"></p>
    </form>
</div>
<script>
// Check if already logged in - redirect via HTMX content swap
const existingToken = localStorage.getItem('access_token');
if (existingToken) {
    htmx.ajax('GET', '/chat', {target: 'body', swap: 'innerHTML'}).then(() => {
        history.pushState({}, '', '/chat');
    });
}

function handleLoginResponse(event) {
    const xhr = event.detail.xhr;
    const errorEl = document.getElementById('error-message');

    if (xhr.status === 200) {
        // Store tokens before HTMX redirect happens
        try {
            const data = JSON.parse(xhr.responseText);
            localStorage.setItem('access_token', data.access_token);
            localStorage.setItem('refresh_token', data.refresh_token);
            // Trigger WebSocket connection
            document.dispatchEvent(new CustomEvent('bobot:token-updated'));
        } catch (e) {
            console.error('Failed to parse login response:', e);
        }
    } else {
        errorEl.textContent = xhr.status === 401 ? 'Invalid credentials' : 'Login failed';
        errorEl.classList.remove('hidden');
    }
}
</script>
{{end}}
```

**Step 2: Verify login works**

Run server, test login flow
Expected: Login succeeds, redirects to /chat via HTMX

**Step 3: Commit**

```bash
git add web/templates/login.html
git commit -m "feat: convert login form to HTMX with HX-Redirect"
```

---

## Task 6: Refactor Signup Page for HTMX

**Files:**
- Modify: `web/templates/signup.html`

**Step 1: Update signup form for HTMX**

Replace `web/templates/signup.html`:

```html
{{define "content"}}
<div class="login-container">
    <h1>bobot</h1>
    {{if .Error}}
    <p class="error">{{.Error}}</p>
    {{else}}
    <form id="signup-form" class="login-form"
          hx-post="/api/signup"
          hx-swap="none"
          hx-on::before-request="prepareSignupRequest(event)"
          hx-on::after-request="handleSignupResponse(event)">
        <input type="hidden" name="code" value="{{.Code}}">
        <input type="text" name="username" placeholder="Username" required autofocus
               pattern="[a-zA-Z0-9_]+" minlength="3" title="Letters, numbers, and underscores only">
        <input type="text" name="display_name" placeholder="Display Name" required>
        <input type="password" name="password" placeholder="Password" required minlength="8">
        <input type="password" name="confirm_password" placeholder="Confirm Password" required>
        <button type="submit">Sign Up</button>
        <p id="error-message" class="error hidden"></p>
    </form>
    {{end}}
</div>
<script>
function prepareSignupRequest(event) {
    const form = event.target;
    const errorEl = document.getElementById('error-message');

    if (form.password.value !== form.confirm_password.value) {
        event.preventDefault();
        errorEl.textContent = 'Passwords do not match';
        errorEl.classList.remove('hidden');
        return;
    }

    // HTMX sends form data, but we need JSON
    event.detail.headers['Content-Type'] = 'application/json';
    event.detail.xhr.send(JSON.stringify({
        code: form.code.value,
        username: form.username.value,
        display_name: form.display_name.value,
        password: form.password.value
    }));
    // Cancel default send since we did our own
    return false;
}

function handleSignupResponse(event) {
    const xhr = event.detail.xhr;
    const errorEl = document.getElementById('error-message');

    if (xhr.status === 200) {
        try {
            const data = JSON.parse(xhr.responseText);
            localStorage.setItem('access_token', data.access_token);
            localStorage.setItem('refresh_token', data.refresh_token);
            document.dispatchEvent(new CustomEvent('bobot:token-updated'));
        } catch (e) {
            console.error('Failed to parse signup response:', e);
        }
    } else {
        errorEl.textContent = xhr.responseText || 'Signup failed';
        errorEl.classList.remove('hidden');
    }
}
</script>
{{end}}
```

**Step 2: Verify signup works**

Test signup flow with invite code
Expected: Signup succeeds, redirects to /chat

**Step 3: Commit**

```bash
git add web/templates/signup.html
git commit -m "feat: convert signup form to HTMX with HX-Redirect"
```

---

## Task 7: Refactor Chat Page to Use Event-Based WebSocket

**Files:**
- Modify: `web/static/chat.js`
- Modify: `web/templates/chat.html`

**Step 1: Update chat.html to not auto-initialize**

Replace script tag at end of `web/templates/chat.html`:

```html
<script src="/static/chat.js"></script>
<script>
document.addEventListener('DOMContentLoaded', () => {
    window.chatClient = new ChatClient();
});
// Re-initialize after HTMX swap
document.body.addEventListener('htmx:afterSwap', (event) => {
    if (event.detail.target === document.body && document.getElementById('messages')) {
        window.chatClient = new ChatClient();
    }
});
</script>
```

**Step 2: Refactor chat.js for event-based messaging**

Replace `web/static/chat.js`:

```javascript
class ChatClient {
    constructor() {
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.logoutBtn = document.getElementById('logout-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;
        this.wsContainer = document.getElementById('ws-connection');

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
                history.pushState({}, '', '/');
            });
            return;
        }

        await this.loadRecentMessages(token);
        await this.syncMessages(token);
        this.setupEventListeners();
    }

    async loadRecentMessages(token) {
        try {
            const resp = await fetch('/api/messages/recent?limit=50', {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 401) {
                    document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
                    return;
                }
                throw new Error('Failed to load messages');
            }

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => this.addMessage(msg.Content, msg.Role, msg.ID, false));
                this.oldestMessageId = messages[0].ID;
                this.updateLastSeenTimestamp(messages[messages.length - 1].CreatedAt);
            }
            this.scrollToBottom();
        } catch (err) {
            console.error('Failed to load messages:', err);
        }
    }

    async loadMoreHistory() {
        if (this.isLoadingHistory || !this.hasMoreHistory || !this.oldestMessageId) {
            return;
        }

        this.isLoadingHistory = true;
        const token = localStorage.getItem('access_token');

        try {
            const resp = await fetch(`/api/messages/history?before=${this.oldestMessageId}&limit=50`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to load history');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                const scrollHeight = this.messagesEl.scrollHeight;
                const scrollTop = this.messagesEl.scrollTop;

                messages.reverse().forEach(msg => this.prependMessage(msg.Content, msg.Role, msg.ID));
                this.oldestMessageId = messages[0].ID;

                this.messagesEl.scrollTop = this.messagesEl.scrollHeight - scrollHeight + scrollTop;
            } else {
                this.hasMoreHistory = false;
            }
        } catch (err) {
            console.error('Failed to load history:', err);
        } finally {
            this.isLoadingHistory = false;
        }
    }

    async syncMessages(token) {
        const lastSeen = localStorage.getItem('lastMessageTimestamp');
        if (!lastSeen) return;

        try {
            const resp = await fetch(`/api/messages/sync?since=${encodeURIComponent(lastSeen)}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) return;

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => {
                    if (!document.querySelector(`[data-message-id="${msg.ID}"]`)) {
                        this.addMessage(msg.Content, msg.Role, msg.ID, false);
                    }
                });
                this.updateLastSeenTimestamp(messages[messages.length - 1].CreatedAt);
                this.scrollToBottom();
            }
        } catch (err) {
            console.error('Sync failed:', err);
        }
    }

    updateLastSeenTimestamp(timestamp) {
        localStorage.setItem('lastMessageTimestamp', timestamp);
    }

    setupEventListeners() {
        // WebSocket message events
        this.handleChatMessage = (event) => {
            const data = event.detail;
            if (data.role === 'assistant' || data.role === 'system') {
                this.removeTypingIndicator();
            }
            if (data.role === 'user') {
                this.addMessage(data.content, data.role);
                this.showTypingIndicator();
            } else {
                this.addMessage(data.content, data.role);
            }
            this.updateLastSeenTimestamp(new Date().toISOString());
        };
        document.addEventListener('bobot:chat-message', this.handleChatMessage);

        // Form submission
        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
        });

        // Menu
        this.menuBtn.addEventListener('click', () => {
            this.menuOverlay.classList.remove('hidden');
        });

        this.menuOverlay.addEventListener('click', (e) => {
            if (e.target === this.menuOverlay) {
                this.menuOverlay.classList.add('hidden');
            }
        });

        // Logout
        this.logoutBtn.addEventListener('click', () => {
            this.logout();
        });

        // Infinite scroll
        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });

        // Cleanup on page swap
        document.body.addEventListener('htmx:beforeSwap', this.cleanup.bind(this), { once: true });
    }

    cleanup() {
        document.removeEventListener('bobot:chat-message', this.handleChatMessage);
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content) return;

        if (this.wsContainer.send({ content: content })) {
            this.input.value = '';
        }
    }

    addMessage(content, role, id = null, scroll = true) {
        const msgEl = document.createElement('div');
        const displayRole = role === 'command' ? 'user' : role;
        msgEl.className = `message ${displayRole}`;
        msgEl.textContent = content;
        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }
        this.messagesEl.appendChild(msgEl);
        if (scroll) {
            this.scrollToBottom();
        }
    }

    prependMessage(content, role, id = null) {
        const msgEl = document.createElement('div');
        const displayRole = role === 'command' ? 'user' : role;
        msgEl.className = `message ${displayRole}`;
        msgEl.textContent = content;
        if (id) {
            msgEl.setAttribute('data-message-id', id);
        }
        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
    }

    showTypingIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'message assistant typing-indicator';
        indicator.id = 'typing-indicator';
        indicator.innerHTML = '<span></span><span></span><span></span>';
        this.messagesEl.appendChild(indicator);
        this.scrollToBottom();
    }

    removeTypingIndicator() {
        const indicator = document.getElementById('typing-indicator');
        if (indicator) {
            indicator.remove();
        }
    }

    scrollToBottom() {
        this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    async logout() {
        const refreshToken = localStorage.getItem('refresh_token');

        // Close WebSocket before logout
        this.wsContainer.close();

        if (refreshToken) {
            try {
                await fetch('/api/logout', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'HX-Request': 'true'
                    },
                    body: JSON.stringify({ refresh_token: refreshToken })
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
        }

        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');

        htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
            history.pushState({}, '', '/');
        });
    }
}
```

**Step 3: Verify chat works**

Login and test chat functionality
Expected: Messages send/receive, logout works

**Step 4: Commit**

```bash
git add web/static/chat.js web/templates/chat.html
git commit -m "feat: refactor chat to use event-based WebSocket messaging"
```

---

## Task 8: Refactor Groups Page for HTMX Navigation

**Files:**
- Modify: `web/templates/groups.html`

**Step 1: Update groups.html for HTMX**

Replace `web/templates/groups.html`:

```html
{{define "content"}}
<div class="groups-container">
    <header class="groups-header">
        <a href="/chat" class="back-link">&larr; Chat</a>
        <h1>Groups</h1>
        <button id="create-group-btn" class="create-btn">+</button>
    </header>

    <main class="groups-list" id="groups-list">
        <div class="loading">Loading groups...</div>
    </main>
</div>

<div id="create-group-modal" class="modal hidden">
    <div class="modal-content">
        <h2>Create Group</h2>
        <form id="create-group-form"
              hx-post="/api/groups"
              hx-swap="none"
              hx-on::before-request="prepareCreateGroupRequest(event)"
              hx-on::after-request="handleCreateGroupResponse(event)">
            <input type="text" id="group-name" name="name" placeholder="Group name" required maxlength="100">
            <div class="modal-actions">
                <button type="button" id="cancel-create">Cancel</button>
                <button type="submit">Create</button>
            </div>
        </form>
    </div>
</div>

<script>
class GroupsPage {
    constructor() {
        this.listEl = document.getElementById('groups-list');
        this.modal = document.getElementById('create-group-modal');
        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
                history.pushState({}, '', '/');
            });
            return;
        }

        await this.loadGroups(token);
        this.setupEventListeners();
    }

    async loadGroups(token) {
        try {
            const resp = await fetch('/api/groups', {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 401) {
                    document.dispatchEvent(new CustomEvent('bobot:auth-expired'));
                    return;
                }
                throw new Error('Failed to load groups');
            }

            const groups = await resp.json();
            this.renderGroups(groups);
        } catch (err) {
            console.error('Failed to load groups:', err);
            this.listEl.innerHTML = '<div class="error">Failed to load groups</div>';
        }
    }

    renderGroups(groups) {
        if (!groups || groups.length === 0) {
            this.listEl.innerHTML = '<div class="empty">No groups yet. Create one!</div>';
            return;
        }

        this.listEl.innerHTML = groups.map(g => `
            <a href="/groups/${g.id}" class="group-item">
                <span class="group-name">${this.escapeHtml(g.name)}</span>
                <span class="group-members">${g.member_count} member${g.member_count !== 1 ? 's' : ''}</span>
            </a>
        `).join('');
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    setupEventListeners() {
        document.getElementById('create-group-btn').addEventListener('click', () => {
            this.modal.classList.remove('hidden');
        });

        document.getElementById('cancel-create').addEventListener('click', () => {
            this.modal.classList.add('hidden');
        });

        this.modal.addEventListener('click', (e) => {
            if (e.target === this.modal) {
                this.modal.classList.add('hidden');
            }
        });
    }
}

function prepareCreateGroupRequest(event) {
    const form = event.target;
    const token = localStorage.getItem('access_token');

    event.detail.headers['Content-Type'] = 'application/json';
    event.detail.headers['Authorization'] = `Bearer ${token}`;
}

function handleCreateGroupResponse(event) {
    const xhr = event.detail.xhr;
    if (xhr.status !== 201) {
        alert('Failed to create group');
    }
    // HX-Redirect will handle navigation on success
}

document.addEventListener('DOMContentLoaded', () => new GroupsPage());

// Re-initialize after HTMX swap
document.body.addEventListener('htmx:afterSwap', (event) => {
    if (event.detail.target === document.body && document.getElementById('groups-list')) {
        new GroupsPage();
    }
});
</script>
{{end}}
```

**Step 2: Verify groups page works**

Navigate to /groups, create a group
Expected: Group list loads, creation redirects to new group

**Step 3: Commit**

```bash
git add web/templates/groups.html
git commit -m "feat: refactor groups page for HTMX navigation"
```

---

## Task 9: Refactor Group Chat Page for Event-Based WebSocket

**Files:**
- Modify: `web/static/group_chat.js`
- Modify: `web/templates/group_chat.html`

**Step 1: Update group_chat.html**

Replace script section at end of `web/templates/group_chat.html`:

```html
<script>
const GROUP_ID = {{.GroupID}};
</script>
<script src="/static/group_chat.js"></script>
<script>
document.addEventListener('DOMContentLoaded', () => {
    window.groupChatClient = new GroupChatClient(GROUP_ID);
});
document.body.addEventListener('htmx:afterSwap', (event) => {
    if (event.detail.target === document.body && document.getElementById('messages') && typeof GROUP_ID !== 'undefined') {
        window.groupChatClient = new GroupChatClient(GROUP_ID);
    }
});
</script>
```

**Step 2: Refactor group_chat.js for event-based messaging**

Replace `web/static/group_chat.js`:

```javascript
class GroupChatClient {
    constructor(groupId) {
        this.groupId = groupId;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.leaveBtn = document.getElementById('leave-btn');
        this.deleteBtn = document.getElementById('delete-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;
        this.currentUserId = null;
        this.wsContainer = document.getElementById('ws-connection');

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
                history.pushState({}, '', '/');
            });
            return;
        }

        await this.loadGroupInfo(token);
        await this.loadRecentMessages(token);
        this.setupEventListeners();
    }

    async loadGroupInfo(token) {
        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 403 || resp.status === 404) {
                    htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                        history.pushState({}, '', '/groups');
                    });
                    return;
                }
                throw new Error('Failed to load group');
            }

            const group = await resp.json();
            document.getElementById('group-name').textContent = group.name;

            const payload = JSON.parse(atob(token.split('.')[1]));
            this.currentUserId = payload.user_id;

            if (group.owner_id === this.currentUserId) {
                this.deleteBtn.classList.remove('hidden');
                this.leaveBtn.classList.add('hidden');
            }

            const membersList = document.getElementById('members-list');
            membersList.innerHTML = '<strong>Members:</strong>' + group.members.map(m =>
                `<div class="member">${this.escapeHtml(m.display_name || m.username)}</div>`
            ).join('');

        } catch (err) {
            console.error('Failed to load group:', err);
        }
    }

    async loadRecentMessages(token) {
        try {
            const resp = await fetch(`/api/groups/${this.groupId}/messages/recent?limit=50`, {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to load messages');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                messages.forEach(msg => this.addMessage(msg, false));
                this.oldestMessageId = messages[0].ID;
            }
            this.scrollToBottom();
        } catch (err) {
            console.error('Failed to load messages:', err);
        }
    }

    setupEventListeners() {
        // WebSocket message events - filter by group_id
        this.handleGroupMessage = (event) => {
            const data = event.detail;
            if (data.group_id === this.groupId) {
                if (data.role === 'assistant') {
                    this.removeTypingIndicator();
                }
                this.addMessage(data, true);
            }
        };
        document.addEventListener('bobot:group-message', this.handleGroupMessage);

        this.form.addEventListener('submit', (e) => {
            e.preventDefault();
            this.sendMessage();
        });

        this.menuBtn.addEventListener('click', () => {
            this.menuOverlay.classList.remove('hidden');
        });

        this.menuOverlay.addEventListener('click', (e) => {
            if (e.target === this.menuOverlay) {
                this.menuOverlay.classList.add('hidden');
            }
        });

        this.leaveBtn.addEventListener('click', () => this.leaveGroup());
        this.deleteBtn.addEventListener('click', () => this.deleteGroup());

        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });

        // Cleanup on page swap
        document.body.addEventListener('htmx:beforeSwap', this.cleanup.bind(this), { once: true });
    }

    cleanup() {
        document.removeEventListener('bobot:group-message', this.handleGroupMessage);
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content) return;

        if (this.wsContainer.send({ content: content, group_id: this.groupId })) {
            this.input.value = '';
            if (content.toLowerCase().includes('@assistant')) {
                this.showTypingIndicator();
            }
        }
    }

    addMessage(msg, scroll = true) {
        const msgEl = document.createElement('div');
        const role = msg.role || msg.Role;
        const content = msg.content || msg.Content;
        const displayName = msg.display_name || msg.DisplayName;

        msgEl.className = `message ${role}`;

        if (role === 'user' && displayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = displayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';
        contentEl.textContent = content;
        msgEl.appendChild(contentEl);

        if (msg.ID) {
            msgEl.setAttribute('data-message-id', msg.ID);
        }

        this.messagesEl.appendChild(msgEl);
        if (scroll) this.scrollToBottom();
    }

    showTypingIndicator() {
        const indicator = document.createElement('div');
        indicator.className = 'message assistant typing-indicator';
        indicator.id = 'typing-indicator';
        indicator.innerHTML = '<span></span><span></span><span></span>';
        this.messagesEl.appendChild(indicator);
        this.scrollToBottom();
    }

    removeTypingIndicator() {
        const indicator = document.getElementById('typing-indicator');
        if (indicator) indicator.remove();
    }

    scrollToBottom() {
        this.messagesEl.scrollTop = this.messagesEl.scrollHeight;
    }

    async loadMoreHistory() {
        if (this.isLoadingHistory || !this.hasMoreHistory || !this.oldestMessageId) return;

        this.isLoadingHistory = true;
        const token = localStorage.getItem('access_token');

        try {
            const resp = await fetch(
                `/api/groups/${this.groupId}/messages/history?before=${this.oldestMessageId}&limit=50`,
                { headers: { 'Authorization': `Bearer ${token}` } }
            );

            if (!resp.ok) throw new Error('Failed to load history');

            const messages = await resp.json();
            if (messages && messages.length > 0) {
                const scrollHeight = this.messagesEl.scrollHeight;
                const scrollTop = this.messagesEl.scrollTop;

                messages.reverse().forEach(msg => this.prependMessage(msg));
                this.oldestMessageId = messages[0].ID;

                this.messagesEl.scrollTop = this.messagesEl.scrollHeight - scrollHeight + scrollTop;
            } else {
                this.hasMoreHistory = false;
            }
        } catch (err) {
            console.error('Failed to load history:', err);
        } finally {
            this.isLoadingHistory = false;
        }
    }

    prependMessage(msg) {
        const msgEl = document.createElement('div');
        const role = msg.Role;
        msgEl.className = `message ${role}`;

        if (role === 'user' && msg.DisplayName) {
            const nameEl = document.createElement('div');
            nameEl.className = 'message-sender';
            nameEl.textContent = msg.DisplayName;
            msgEl.appendChild(nameEl);
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';
        contentEl.textContent = msg.Content;
        msgEl.appendChild(contentEl);

        if (msg.ID) {
            msgEl.setAttribute('data-message-id', msg.ID);
        }

        this.messagesEl.insertBefore(msgEl, this.messagesEl.firstChild);
    }

    async leaveGroup() {
        if (!confirm('Are you sure you want to leave this group?')) return;

        const token = localStorage.getItem('access_token');
        try {
            const resp = await fetch(`/api/groups/${this.groupId}/members/${this.currentUserId}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to leave group');

            htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                history.pushState({}, '', '/groups');
            });
        } catch (err) {
            console.error('Failed to leave group:', err);
            alert('Failed to leave group');
        }
    }

    async deleteGroup() {
        if (!confirm('Are you sure you want to delete this group? This cannot be undone.')) return;

        const token = localStorage.getItem('access_token');
        try {
            const resp = await fetch(`/api/groups/${this.groupId}`, {
                method: 'DELETE',
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) throw new Error('Failed to delete group');

            htmx.ajax('GET', '/groups', {target: 'body', swap: 'innerHTML'}).then(() => {
                history.pushState({}, '', '/groups');
            });
        } catch (err) {
            console.error('Failed to delete group:', err);
            alert('Failed to delete group');
        }
    }

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
}
```

**Step 3: Verify group chat works**

Navigate to a group, send messages
Expected: Messages send/receive, leave/delete work with HTMX navigation

**Step 4: Commit**

```bash
git add web/static/group_chat.js web/templates/group_chat.html
git commit -m "feat: refactor group chat for event-based WebSocket messaging"
```

---

## Task 10: Add Global HTMX Error Handling

**Files:**
- Modify: `web/templates/layout.html`

**Step 1: Add global error handler after ws-manager.js**

In `web/templates/layout.html`, add after the ws-manager.js script tag:

```html
<script>
// Global HTMX error handling
document.body.addEventListener('htmx:responseError', function(event) {
    const xhr = event.detail.xhr;
    if (xhr.status === 401) {
        // Unauthorized - clear tokens and redirect to login
        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');
        htmx.ajax('GET', '/', {target: 'body', swap: 'innerHTML'}).then(() => {
            history.pushState({}, '', '/');
        });
    }
});

// Handle HX-Redirect on all responses (including errors)
document.body.addEventListener('htmx:beforeSwap', function(event) {
    const xhr = event.detail.xhr;
    const redirectUrl = xhr.getResponseHeader('HX-Redirect');
    if (redirectUrl) {
        event.detail.shouldSwap = false;
        htmx.ajax('GET', redirectUrl, {target: 'body', swap: 'innerHTML'}).then(() => {
            history.pushState({}, '', redirectUrl);
        });
    }
});
</script>
```

**Step 2: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 3: Commit**

```bash
git add web/templates/layout.html
git commit -m "feat: add global HTMX error and redirect handling"
```

---

## Task 11: Manual Testing on iOS PWA

**Step 1: Deploy to test environment or use ngrok**

Run: `ngrok http 8080` or deploy to test server

**Step 2: Add to iOS home screen**

Open in Safari, tap Share > Add to Home Screen

**Step 3: Test all navigation flows**

- [ ] Login redirects to /chat (stays in PWA)
- [ ] Navigate to /groups via link (content swap, no browser open)
- [ ] Create group redirects to /groups/{id} (stays in PWA)
- [ ] Navigate back to /groups (back button or link)
- [ ] Navigate to /chat (link)
- [ ] Logout redirects to / (stays in PWA)
- [ ] WebSocket messages work across all pages
- [ ] Token refresh works when access token expires

**Step 4: Document any issues found**

Create issues for any bugs discovered during testing.

---

## Task 12: Final Cleanup and Documentation

**Step 1: Remove any unused code**

Check for dead code in JS files that was replaced.

**Step 2: Update design doc with any changes**

If implementation deviated from design, update `docs/plans/2026-01-31-htmx-pwa-navigation-design.md`.

**Step 3: Final commit**

```bash
git add -A
git commit -m "chore: cleanup and documentation updates"
```
