class ChatClient {
    constructor() {
        this.ws = null;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.logoutBtn = document.getElementById('logout-btn');
        this.isLoadingHistory = false;
        this.oldestMessageId = null;
        this.hasMoreHistory = true;

        this.init();
    }

    async init() {
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        // Load initial messages
        await this.loadRecentMessages(token);

        // Sync any missed messages
        await this.syncMessages(token);

        // Connect WebSocket
        this.connect(token);
        this.setupEventListeners();
    }

    async loadRecentMessages(token) {
        try {
            const resp = await fetch('/api/messages/recent?limit=50', {
                headers: { 'Authorization': `Bearer ${token}` }
            });

            if (!resp.ok) {
                if (resp.status === 401) {
                    this.logout();
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
                // Remember scroll position
                const scrollHeight = this.messagesEl.scrollHeight;
                const scrollTop = this.messagesEl.scrollTop;

                // Prepend messages (they come in DESC order, so reverse for display)
                messages.reverse().forEach(msg => this.prependMessage(msg.Content, msg.Role, msg.ID));
                this.oldestMessageId = messages[0].ID;

                // Restore scroll position
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
                    // Only add if not already displayed
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

    connect(token) {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat?token=${token}`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('WebSocket connected');
        };

        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            // Handle both user and assistant messages from broadcast
            if (data.role === 'assistant' || data.role === 'system') {
                this.removeTypingIndicator();
            }
            // Show typing indicator after user message is displayed
            if (data.role === 'user') {
                this.addMessage(data.content, data.role);
                this.showTypingIndicator();
            } else {
                this.addMessage(data.content, data.role);
            }
            this.updateLastSeenTimestamp(new Date().toISOString());
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            this.refreshAndReconnect();
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }

    async refreshAndReconnect() {
        const refreshToken = localStorage.getItem('refresh_token');
        if (!refreshToken) {
            this.logout();
            return;
        }

        try {
            const resp = await fetch('/api/refresh', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({refresh_token: refreshToken})
            });

            if (!resp.ok) {
                throw new Error('Refresh failed');
            }

            const data = await resp.json();
            localStorage.setItem('access_token', data.access_token);

            // Sync messages before reconnecting
            await this.syncMessages(data.access_token);

            // Reconnect with new token
            setTimeout(() => this.connect(data.access_token), 1000);
        } catch (err) {
            console.error('Token refresh failed:', err);
            this.logout();
        }
    }

    setupEventListeners() {
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

        this.logoutBtn.addEventListener('click', () => {
            this.logout();
        });

        // Infinite scroll - load more when scrolling near top
        this.messagesEl.addEventListener('scroll', () => {
            if (this.messagesEl.scrollTop < 100) {
                this.loadMoreHistory();
            }
        });
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }

        // Don't add message locally - wait for server broadcast
        // This ensures consistency across all devices
        // Typing indicator is shown after user message is displayed in onmessage
        this.ws.send(JSON.stringify({content: content}));
        this.input.value = '';
    }

    addMessage(content, role, id = null, scroll = true) {
        const msgEl = document.createElement('div');
        msgEl.className = `message ${role}`;
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
        msgEl.className = `message ${role}`;
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
        if (refreshToken) {
            try {
                await fetch('/api/logout', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({refresh_token: refreshToken})
                });
            } catch (err) {
                console.error('Logout error:', err);
            }
        }

        localStorage.removeItem('access_token');
        localStorage.removeItem('refresh_token');
        localStorage.removeItem('lastMessageTimestamp');
        window.location.href = '/';
    }
}

document.addEventListener('DOMContentLoaded', () => {
    new ChatClient();
});
