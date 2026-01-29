class ChatClient {
    constructor() {
        this.ws = null;
        this.messagesEl = document.getElementById('messages');
        this.form = document.getElementById('chat-form');
        this.input = document.getElementById('message-input');
        this.menuBtn = document.getElementById('menu-btn');
        this.menuOverlay = document.getElementById('menu-overlay');
        this.logoutBtn = document.getElementById('logout-btn');

        this.init();
    }

    init() {
        // Check auth
        const token = localStorage.getItem('access_token');
        if (!token) {
            window.location.href = '/';
            return;
        }

        this.connect(token);
        this.setupEventListeners();
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
            this.removeTypingIndicator();
            this.addMessage(data.content, 'assistant');
        };

        this.ws.onclose = () => {
            console.log('WebSocket disconnected');
            // Try to refresh token and reconnect
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
    }

    sendMessage() {
        const content = this.input.value.trim();
        if (!content || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
            return;
        }

        // Add user message to UI
        this.addMessage(content, 'user');

        // Show typing indicator
        this.showTypingIndicator();

        // Send to server
        this.ws.send(JSON.stringify({content: content}));

        // Clear input
        this.input.value = '';
    }

    addMessage(content, role) {
        const msgEl = document.createElement('div');
        msgEl.className = `message ${role}`;
        msgEl.textContent = content;
        this.messagesEl.appendChild(msgEl);
        this.scrollToBottom();
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
        window.location.href = '/';
    }
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
    new ChatClient();
});
