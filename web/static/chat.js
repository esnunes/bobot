// Guard against re-declaration on HTMX swaps
if (typeof ChatClient === 'undefined') {
    window.ChatClient = class ChatClient {
        constructor() {
            // Clean up any previous page client
            if (window.currentPageClient && window.currentPageClient.cleanup) {
                window.currentPageClient.cleanup();
            }
            window.currentPageClient = this;

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
            this.handleChatMessage = null;

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
            // WebSocket message events - bind to instance for cleanup
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
        }

        cleanup() {
            if (this.handleChatMessage) {
                document.removeEventListener('bobot:chat-message', this.handleChatMessage);
                this.handleChatMessage = null;
            }
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
    };
}
